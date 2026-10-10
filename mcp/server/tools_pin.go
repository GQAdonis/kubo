package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
)

func init() { registerGroup(pinTools) }

func pinTools(s *Server) []toolImpl {
	return []toolImpl{
		{Name: "pin_add", Call: s.pinAdd},
		{Name: "pin_ls", Call: s.pinLs},
		{Name: "pin_verify", Call: s.pinVerify},
		{Name: "pin_update", Call: s.pinUpdate},
		{Name: "pin_rm", Call: s.pinRm},
	}
}

// pinsResponse is the pin/add, pin/rm and pin/update response.
type pinsResponse struct {
	Pins []string
}

type pinAddInput struct {
	Path      string `json:"path"`
	Recursive *bool  `json:"recursive"`
	Name      string `json:"name"`
}

type pinAddOutput struct {
	Pins []string `json:"pins"`
	Name string   `json:"name,omitempty"`
}

// The confirmation gate has already run when these handlers are called.

func (s *Server) pinAdd(ctx context.Context, raw json.RawMessage) (any, error) {
	var in pinAddInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	opts := map[string]any{"recursive": boolOr(in.Recursive, true)}
	if in.Name != "" {
		opts["name"] = in.Name
	}
	var resp pinsResponse
	if err := s.daemon.Call(ctx, "pin/add", []string{in.Path}, opts, &resp); err != nil {
		return nil, err
	}
	return pinAddOutput{Pins: nonNil(resp.Pins), Name: in.Name}, nil
}

type pinRmInput struct {
	Path      string `json:"path"`
	Recursive *bool  `json:"recursive"`
}

type pinRmOutput struct {
	Unpinned []string `json:"unpinned"`
	Warnings []string `json:"warnings,omitempty"`
}

func (s *Server) pinRm(ctx context.Context, raw json.RawMessage) (any, error) {
	var in pinRmInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	var resp pinsResponse
	opts := map[string]any{"recursive": boolOr(in.Recursive, true)}
	if err := s.daemon.Call(ctx, "pin/rm", []string{in.Path}, opts, &resp); err != nil {
		return nil, err
	}
	return pinRmOutput{Unpinned: nonNil(resp.Pins), Warnings: s.afterUnpin(ctx, resp.Pins)}, nil
}

type pinUpdateInput struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Unpin *bool  `json:"unpin"`
}

type pinUpdateOutput struct {
	From        string   `json:"from"`
	To          string   `json:"to"`
	UnpinnedOld bool     `json:"unpinned_old"`
	Warnings    []string `json:"warnings,omitempty"`
}

// pinUpdate is class destructive: with the default unpin=true it removes the
// old pin, so it needs the destructive selector like pin_rm.
func (s *Server) pinUpdate(ctx context.Context, raw json.RawMessage) (any, error) {
	var in pinUpdateInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	unpin := boolOr(in.Unpin, true)
	var resp pinsResponse
	if err := s.daemon.Call(ctx, "pin/update", []string{in.From, in.To}, map[string]any{"unpin": unpin}, &resp); err != nil {
		return nil, err
	}
	if len(resp.Pins) != 2 {
		return nil, fmt.Errorf("pin/update returned %d pins, want the old and the new", len(resp.Pins))
	}
	out := pinUpdateOutput{From: resp.Pins[0], To: resp.Pins[1], UnpinnedOld: unpin}
	if unpin {
		out.Warnings = s.afterUnpin(ctx, resp.Pins[:1])
	}
	return out, nil
}

type pinLsInput struct {
	Type   string `json:"type"`
	Path   string `json:"path"`
	Name   string `json:"name"`
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

type pinEntry struct {
	CID  string `json:"cid"`
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type pinLsOutput struct {
	Pins []pinEntry `json:"pins"`
	Page pageInfo   `json:"page"`
}

// pinLsResponse is the non-streaming pin/ls response.
type pinLsResponse struct {
	Keys map[string]struct {
		Type string
		Name string
	}
}

func (s *Server) pinLs(ctx context.Context, raw json.RawMessage) (any, error) {
	var in pinLsInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	limit, err := pageLimit(in.Limit)
	if err != nil {
		return nil, err
	}
	cur, err := decodeCursor(in.Cursor)
	if err != nil {
		return nil, err
	}
	typ := in.Type
	if typ == "" {
		typ = "all"
	}
	opts := map[string]any{"type": typ, "names": true}
	var args []string
	if in.Path != "" {
		args = []string{in.Path}
	}
	if in.Name != "" {
		opts["name"] = in.Name
	}

	var resp pinLsResponse
	if err := s.daemon.Call(ctx, "pin/ls", args, opts, &resp); err != nil {
		var rpcErr *RPCError
		if in.Path != "" && errors.As(err, &rpcErr) && strings.Contains(rpcErr.Message, "is not pinned") {
			return pinLsOutput{Pins: []pinEntry{}, Page: pageInfo{}}, nil
		}
		return nil, err
	}

	entries := make([]pinEntry, 0, len(resp.Keys))
	for c, k := range resp.Keys {
		if in.Name != "" && !strings.Contains(k.Name, in.Name) {
			continue
		}
		entries = append(entries, pinEntry{CID: c, Type: normalizePinType(k.Type), Name: k.Name})
	}
	slices.SortFunc(entries, func(a, b pinEntry) int { return strings.Compare(a.CID, b.CID) })
	page, info := pageByKey(entries, func(e pinEntry) string { return e.CID }, cur, limit)
	return pinLsOutput{Pins: nonNilEntries(page), Page: info}, nil
}

// normalizePinType maps the daemon's pin type to the contract enum. The
// daemon reports indirect pins as "indirect" optionally followed by the
// pinned root.
func normalizePinType(t string) string {
	if strings.HasPrefix(t, "indirect") {
		return "indirect"
	}
	return t
}

type pinVerifyInput struct {
	OnlyFailures bool   `json:"only_failures"`
	Cursor       string `json:"cursor"`
	Limit        int    `json:"limit"`
}

type badNode struct {
	CID   string `json:"cid"`
	Error string `json:"error"`
}

type verifyEntry struct {
	CID      string    `json:"cid"`
	OK       bool      `json:"ok"`
	BadNodes []badNode `json:"bad_nodes,omitempty"`
}

type pinVerifyOutput struct {
	Results []verifyEntry `json:"results"`
	Page    pageInfo      `json:"page"`
}

// pinVerifyLine is one line of the pin/verify stream.
type pinVerifyLine struct {
	Cid      string
	Err      string
	Ok       bool
	BadNodes []struct {
		Cid string
		Err string
	}
}

// pinVerify pages over the daemon's verification stream. The stream has no
// sort key, so the cursor is an offset into it, and reading stops as soon as
// the page and one more item were seen: a page costs the verification of the
// pins up to it, not of every pin. Offsets stay valid while the pin set does
// not change.
func (s *Server) pinVerify(ctx context.Context, raw json.RawMessage) (any, error) {
	var in pinVerifyInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	limit, err := pageLimit(in.Limit)
	if err != nil {
		return nil, err
	}
	cur, err := decodeCursor(in.Cursor)
	if err != nil {
		return nil, err
	}

	results := make([]verifyEntry, 0, min(limit, defaultPageLimit))
	seen, hasMore := 0, false
	opts := map[string]any{"verbose": !in.OnlyFailures}
	err = s.daemon.Stream(ctx, "pin/verify", nil, opts, func(dec *json.Decoder) error {
		for {
			var line pinVerifyLine
			if err := dec.Decode(&line); err != nil {
				if errors.Is(err, io.EOF) {
					return nil
				}
				return fmt.Errorf("decode pin/verify stream: %w", err)
			}
			if line.Cid == "" && line.Err != "" {
				return &RPCError{Command: "pin/verify", Message: line.Err}
			}
			seen++
			if seen <= cur.Offset {
				continue
			}
			if len(results) == limit {
				hasMore = true
				return ErrStopStream
			}
			results = append(results, toVerifyEntry(line))
		}
	})
	if err != nil {
		return nil, err
	}
	info := pageInfo{HasMore: hasMore}
	if hasMore {
		info.NextCursor = encodeCursor(cursor{Offset: cur.Offset + len(results)})
	}
	return pinVerifyOutput{Results: results, Page: info}, nil
}

func toVerifyEntry(l pinVerifyLine) verifyEntry {
	e := verifyEntry{CID: l.Cid, OK: l.Ok}
	for _, n := range l.BadNodes {
		e.BadNodes = append(e.BadNodes, badNode{CID: n.Cid, Error: n.Err})
	}
	if l.Err != "" {
		e.OK = false
		e.BadNodes = append(e.BadNodes, badNode{CID: l.Cid, Error: l.Err})
	}
	return e
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilEntries(s []pinEntry) []pinEntry {
	if s == nil {
		return []pinEntry{}
	}
	return s
}
