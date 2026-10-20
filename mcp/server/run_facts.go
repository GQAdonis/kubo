//go:build !noagent

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"slices"
	"unicode/utf8"
)

const runFactsEventName = "ipfs.run.facts"

// RunFacts contains only actual, bounded facts. Pointer fields preserve an
// explicit empty offer/count separately from unreported backend knowledge.
type RunFacts struct {
	OfferedTools       *[]string           `json:"offered_tools,omitempty"`
	CatalogSize        *int                `json:"catalog_size,omitempty"`
	ToolScope          string              `json:"tool_scope,omitempty"`
	SystemPromptDigest string              `json:"system_prompt_digest,omitempty"`
	Backend            *RunBackendFacts    `json:"backend,omitempty"`
	Memory             *RunMemoryFacts     `json:"memory,omitempty"`
	Egress             *RunEgressFacts     `json:"egress,omitempty"`
	ContextSnapshot    *[]ContextReference `json:"context_snapshot,omitempty"`
	Provenance         map[string]string   `json:"provenance,omitempty"`
}

// RunBackendFacts names the selected backend without exposing its URL.
type RunBackendFacts struct {
	Kind  string `json:"kind"`
	Model string `json:"model,omitempty"`
	Host  string `json:"host,omitempty"`
}

// RunMemoryFacts reports an established scope, never an assumed absence.
type RunMemoryFacts struct {
	Scope string `json:"scope"`
}

// RunEgressFacts reports existing acknowledgements without granting any.
type RunEgressFacts struct {
	AcknowledgedHosts []string `json:"acknowledged_hosts"`
}

var runFactHostPattern = regexp.MustCompile(`^[A-Za-z0-9.\[\]:-]{1,256}$`)

func (s *Server) runFactsEnabled() bool {
	cfg, _ := s.aguiExt.(*aguiConfig) // immutable before listener construction
	return cfg != nil && cfg.runFacts
}

func selectedBackendFacts(kind, base, model string) *RunBackendFacts {
	backend := &RunBackendFacts{Kind: kind}
	if utf8.ValidString(model) && utf8.RuneCountInString(model) > 0 && utf8.RuneCountInString(model) <= 256 {
		backend.Model = model
	}
	host := HostFromURL(base)
	if runFactHostPattern.MatchString(host) {
		backend.Host = host
	}
	// A configured URL does not establish where inference actually runs.
	// No placement is inferred from loopback, DNS names or a proxy endpoint.
	return backend
}

func acknowledgedRunEgress(id Identity) *RunEgressFacts {
	if len(id.EgressAck) > 16 {
		return nil
	}
	hosts := slices.Clone(id.EgressAck)
	for _, host := range hosts {
		if !runFactHostPattern.MatchString(host) {
			return nil
		}
	}
	slices.Sort(hosts)
	hosts = slices.Compact(hosts)
	if hosts == nil {
		hosts = []string{}
	}
	return &RunEgressFacts{AcknowledgedHosts: hosts}
}

func (a *Agent) runFacts(run *runCtx) *RunFacts {
	names := make([]string, 0, len(run.offered))
	for name := range run.offered {
		names = append(names, name)
	}
	slices.Sort(names)
	count := len(a.srv.cat.Tools)
	sum := sha256.Sum256([]byte(run.snap.systemPrompt))
	model := ""
	if run.snap.llm != nil {
		model = run.snap.llm.model
	}
	facts := &RunFacts{
		OfferedTools: &names, CatalogSize: &count, ToolScope: "kubo_contract", SystemPromptDigest: "sha256:" + hex.EncodeToString(sum[:]),
		Backend: selectedBackendFacts(BackendKindOpenAI, run.snap.baseURL, model), Memory: &RunMemoryFacts{Scope: "process_conversation"},
		Provenance: map[string]string{"offered_tools": "effective_policy_offer", "catalog_size": "loaded_contract", "system_prompt_digest": "effective_run_snapshot", "backend": "effective_run_snapshot", "memory": "effective_run_snapshot"},
	}
	if egress := acknowledgedRunEgress(run.id); egress != nil {
		facts.Egress = egress
		facts.Provenance["egress"] = "effective_run_snapshot"
	}
	facts.setContext(run.sess.context)
	return facts
}

func (f *RunFacts) setContext(refs []ContextReference) {
	cloned := cloneContextReferences(refs)
	f.ContextSnapshot = &cloned
	f.Provenance["context_snapshot"] = "accepted_turn"
}

func (b *uarBackend) runFacts(id Identity) *RunFacts {
	facts := &RunFacts{Backend: selectedBackendFacts(BackendKindUAR, b.set.BaseURL, ""), Provenance: map[string]string{"backend": "effective_run_snapshot"}}
	if egress := acknowledgedRunEgress(id); egress != nil {
		facts.Egress = egress
		facts.Provenance["egress"] = "effective_run_snapshot"
	}
	// The provisioned default model/prompt and local Kubo catalog do not prove
	// UAR's actual final model, tool offer, system prompt or memory behavior.
	return facts
}

func (s *aguiStream) sendRunFacts(facts *RunFacts) {
	if s.factsSent || facts == nil || len(facts.Provenance) == 0 {
		return
	}
	s.factsSent = true
	value := map[string]any{"thread_id": s.threadID, "run_id": s.runID, "facts": facts}
	if s.activityID != "" {
		value["activity_id"] = s.activityID
	}
	s.send(aguiEvent{"type": "CUSTOM", "name": runFactsEventName, "value": value})
}
