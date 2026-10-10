package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// tokenVersion is the ConfirmTokenClaims version.
const tokenVersion = 1

// macLen is the length of an unpadded base64url HMAC-SHA256.
const macLen = 43

// Handle uses (the use claim). The two handles carry the same binding but mean
// different things, so each is signed with its own key and names its use: a
// confirm token is never a request state, and a request state is never a
// confirm token.
const (
	useConfirmToken = "confirm_token"
	useRequestState = "request_state"
)

// tokenClaims are the fields bound by a confirm token's HMAC
// (ConfirmTokenClaims in the confirmation schema).
type tokenClaims struct {
	V          int    `json:"v"`
	Use        string `json:"use"`
	CID        string `json:"cid"`
	SID        string `json:"sid"`
	PID        string `json:"pid"`
	Tool       string `json:"tool"`
	ArgsSHA256 string `json:"args_sha256"`
	Exp        int64  `json:"exp"`
	Nonce      string `json:"nonce"`
}

var errBadToken = errors.New("confirm token is malformed or was tampered with")

// tokenSigner mints and verifies the signed handles with per-process keys. A
// process restart invalidates every outstanding handle. Each use has its own
// key, derived from one random master key, as defense in depth next to the
// use claim.
type tokenSigner struct {
	keys map[string][]byte
}

func newTokenSigner() (*tokenSigner, error) {
	master := make([]byte, 32)
	if _, err := rand.Read(master); err != nil {
		return nil, fmt.Errorf("generate confirm token key: %w", err)
	}
	keys := make(map[string][]byte, 2)
	for _, use := range []string{useConfirmToken, useRequestState} {
		m := hmac.New(sha256.New, master)
		m.Write([]byte("kubo-mcp/handle-key/" + use))
		keys[use] = m.Sum(nil)
	}
	return &tokenSigner{keys: keys}, nil
}

// mac signs payload with the key of use; an unknown use has no key and signs
// with an empty one that verify never accepts.
func (s *tokenSigner) mac(use, payload string) string {
	m := hmac.New(sha256.New, s.keys[use])
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// mint encodes the claims, with use set, as base64url(JSON) followed by
// base64url(HMAC). The result stays inside the contract's ConfirmToken
// alphabet.
func (s *tokenSigner) mint(use string, c tokenClaims) (string, error) {
	if _, ok := s.keys[use]; !ok {
		return "", fmt.Errorf("unknown handle use %q", use)
	}
	c.V = tokenVersion
	c.Use = use
	raw, err := json.Marshal(c)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", use, err)
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + s.mac(use, payload), nil
}

// verify checks the HMAC under the key of use in constant time, decodes the
// claims and requires their use claim to match. It says nothing about expiry
// or binding; the gate checks those.
func (s *tokenSigner) verify(use, tok string) (tokenClaims, error) {
	var c tokenClaims
	if _, ok := s.keys[use]; !ok || len(tok) <= macLen {
		return c, errBadToken
	}
	payload, sig := tok[:len(tok)-macLen], tok[len(tok)-macLen:]
	if !hmac.Equal([]byte(sig), []byte(s.mac(use, payload))) {
		return c, errBadToken
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return c, errBadToken
	}
	if err := json.Unmarshal(raw, &c); err != nil || c.V != tokenVersion || c.Use != use {
		return tokenClaims{}, errBadToken
	}
	return c, nil
}

// randomID returns n random bytes as unpadded base64url.
func randomID(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand does not fail on supported platforms; a server that
		// cannot get randomness must not hand out guessable IDs.
		panic(fmt.Sprintf("crypto/rand: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
