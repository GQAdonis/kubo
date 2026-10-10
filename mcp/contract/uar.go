package contract

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"strconv"
	"time"
)

// UARPreflightFile is agentic/uar-preflight.json; UARSettingsSchemaFile is the
// schema a UAR settings response must validate against (ADR-0007).
const (
	UARPreflightFile      = "agentic/uar-preflight.json"
	UARSettingsSchemaFile = "agentic/uar-effective-settings.schema.json"
)

// UARPin is the evidence for a pinned UAR release.
type UARPin struct {
	Tag      string `json:"tag"`
	Artifact string `json:"artifact"`
	Report   string `json:"report"`
}

// UARPreflight holds the check-uar values the contract owns. MinVersion and
// SettingsPath are nil until a published UAR release passes the topology-B
// gate (agentic-uar-integration task 1.2).
type UARPreflight struct {
	MinVersion      *string `json:"minVersion"`
	SettingsPath    *string `json:"settingsPath"`
	SettingsSchema  string  `json:"settingsSchema"`
	RecheckInterval string  `json:"recheckInterval"`
	PinnedFrom      *UARPin `json:"pinnedFrom"`
}

// Maximum recheck interval: a stale verdict must not outlive a minute.
const maxUARRecheck = 60 * time.Second

var uarSeconds = regexp.MustCompile(`^([0-9]+)s$`)

// Recheck returns RecheckInterval as a duration.
func (p UARPreflight) Recheck() (time.Duration, error) {
	m := uarSeconds.FindStringSubmatch(p.RecheckInterval)
	if m == nil {
		return 0, fmt.Errorf("recheckInterval %q is not whole seconds", p.RecheckInterval)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, fmt.Errorf("recheckInterval %q: %w", p.RecheckInterval, err)
	}
	return time.Duration(n) * time.Second, nil
}

// check enforces the rules JSON Schema cannot express: the version and the
// settings path are pinned together and only with evidence, and the recheck
// interval is a positive bound of at most a minute.
func (p UARPreflight) check() error {
	if (p.MinVersion == nil) != (p.SettingsPath == nil) {
		return fmt.Errorf("%s: minVersion and settingsPath are pinned together from one release", UARPreflightFile)
	}
	if (p.MinVersion == nil) != (p.PinnedFrom == nil) {
		return fmt.Errorf("%s: pinnedFrom is required exactly when minVersion is set", UARPreflightFile)
	}
	d, err := p.Recheck()
	if err != nil {
		return fmt.Errorf("%s: %w", UARPreflightFile, err)
	}
	if d <= 0 || d > maxUARRecheck {
		return fmt.Errorf("%s: recheckInterval %s must be in (0, %s]", UARPreflightFile, d, maxUARRecheck)
	}
	return nil
}

// LoadUARPreflight reads and checks agentic/uar-preflight.json.
func LoadUARPreflight() (*UARPreflight, error) {
	data, err := fs.ReadFile(files, UARPreflightFile)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", UARPreflightFile, err)
	}
	var p UARPreflight
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", UARPreflightFile, err)
	}
	if err := p.check(); err != nil {
		return nil, err
	}
	return &p, nil
}
