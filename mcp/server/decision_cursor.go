//go:build !noagent

package server

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"time"
)

var opaqueCursorPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

type decisionCursor struct {
	Version   int       `json:"v"`
	Owner     string    `json:"owner"`
	Epoch     string    `json:"epoch"`
	Revision  string    `json:"revision"`
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Limit     int       `json:"limit"`
	Expires   int64     `json:"expires"`
	Order     string    `json:"order"`
}

func decisionOwnerTag(secret []byte, owner string) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte("decisions-owner\x00" + owner)) // hash writes cannot fail
	return hex.EncodeToString(mac.Sum(nil))
}

func (b *Broker) makeDecisionCursor(owner string, snap decisionSnapshot, last Pending, limit int) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	key := decisionCursor{Version: 1, Owner: decisionOwnerTag(b.historySecret, owner), Epoch: snap.Epoch,
		Revision: snap.Revision, ID: last.ID, CreatedAt: last.CreatedAt, Limit: limit, Expires: b.now().Add(time.Minute).Unix(), Order: "created_at_desc,id_desc"}
	data, _ := json.Marshal(key) // fixed typed fields only
	mac := hmac.New(sha256.New, b.historySecret)
	_, _ = mac.Write(data)
	return base64.RawURLEncoding.EncodeToString(append(data, mac.Sum(nil)...))
}

// readDecisionCursor verifies integrity and owner before revealing staleness.
// A restart loses this process-only key: old bytes are generic invalid input,
// never a trusted old epoch. An authenticated own stale cursor returns refresh.
func (b *Broker) readDecisionCursor(raw, owner string, snap decisionSnapshot, limit int) (decisionCursor, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	invalid := errors.New("invalid decision cursor")
	if len(raw) == 0 || len(raw) > 2048 || !opaqueCursorPattern.MatchString(raw) {
		return decisionCursor{}, false, invalid
	}
	data, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(data) <= sha256.Size {
		return decisionCursor{}, false, invalid
	}
	data, signature := data[:len(data)-sha256.Size], data[len(data)-sha256.Size:]
	mac := hmac.New(sha256.New, b.historySecret)
	_, _ = mac.Write(data)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return decisionCursor{}, false, invalid
	}
	var key decisionCursor
	if err := json.Unmarshal(data, &key); err != nil || key.Version != 1 || !hmac.Equal([]byte(key.Owner), []byte(decisionOwnerTag(b.historySecret, owner))) ||
		key.Limit != limit || key.Order != "created_at_desc,id_desc" || !decisionIDPattern.MatchString(key.ID) || key.CreatedAt.IsZero() {
		return decisionCursor{}, false, invalid
	}
	stale := key.Epoch != snap.Epoch || key.Revision != snap.Revision || !b.now().Before(time.Unix(key.Expires, 0))
	return key, stale, nil
}
