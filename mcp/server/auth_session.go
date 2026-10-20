package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/auth"
)

// sessionHandler serves GET /auth/session (contract routes/session.json). It
// reports what a verified session token resolves to, so the Web UI can choose
// session mode. It never returns the token, the `sub` or any claim besides
// `email` and `exp`.
type sessionHandler struct {
	// log receives the encoder error at debug level; nil discards it.
	log *slog.Logger
}

// sessionPrincipalBody and sessionBody are the response of the contract schema
// routes/session-response.schema.json.
type sessionPrincipalBody struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type sessionBody struct {
	Principals []sessionPrincipalBody `json:"principals"`
	Email      string                 `json:"email,omitempty"`
	Exp        int64                  `json:"exp"`
	Admin      string                 `json:"admin"`
}

// Bounds of the schema's email property.
const (
	minEmailLen = 3
	maxEmailLen = 320
)

func (h sessionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var id Identity
	if info := auth.TokenInfoFromContext(r.Context()); info != nil {
		id, _ = info.Extra[identityExtraKey].(Identity)
	}
	// Only a session identity has a report: a static bearer is authenticated
	// but is not a console session.
	if id.Session == nil {
		w.Header().Add("WWW-Authenticate", "Bearer")
		http.Error(w, "no session", http.StatusUnauthorized)
		return
	}
	s := id.Session
	body := sessionBody{Principals: make([]sessionPrincipalBody, 0, len(s.Principals)), Exp: s.Expiry.Unix(), Admin: s.Admin}
	for _, p := range s.Principals {
		body.Principals = append(body.Principals, sessionPrincipalBody{Name: p.ID, Kind: p.Kind})
	}
	if n := utf8.RuneCountInString(s.Email); n >= minEmailLen && n <= maxEmailLen {
		body.Email = s.Email
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(body); err != nil && h.log != nil {
		// The client usually hung up; the response is already committed.
		h.log.Debug("session report not written", "error", err.Error())
	}
}
