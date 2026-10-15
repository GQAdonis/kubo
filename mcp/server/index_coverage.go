package server

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
)

// This file is built with and without the noagent tag, like index.go.
//
// An indexed CID is covered when a pin reaches it or the MFS holds it. Both
// checks read the node only: ls and stat carry offline: true, so a missing
// block is an error and never a fetch.

const (
	// coverWalkNodes bounds how many nodes (files and directories) one walk
	// visits. A walk that reaches the bound is incomplete, and what it did not
	// reach is left for ipfs_index_reconcile.
	coverWalkNodes = 10000
	// coverWalkDepth bounds how deep a walk descends.
	coverWalkDepth = 64
	// coverCallTimeout bounds one stat or ls of a walk.
	coverCallTimeout = 20 * time.Second
)

// statLocal is files/stat of a CID, from blocks the node already holds.
func (s *Server) statLocal(ctx context.Context, c string) (statResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, coverCallTimeout)
	defer cancel()
	var st statResponse
	err := s.daemon.Call(ctx, "files/stat", []string{"/ipfs/" + c}, map[string]any{"offline": true}, &st)
	return st, err
}

// childrenLocal lists the entries of a directory, offline. resolve-type and
// size are off so that one missing block does not hide the siblings.
func (s *Server) childrenLocal(ctx context.Context, c string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, coverCallTimeout)
	defer cancel()
	var resp lsResponse
	err := s.daemon.Call(ctx, "ls", []string{"/ipfs/" + c}, map[string]any{"offline": true, "resolve-type": false, "size": false}, &resp)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, o := range resp.Objects {
		for _, l := range o.Links {
			if l.Hash != "" {
				out = append(out, l.Hash)
			}
		}
	}
	return out, nil
}

// walkFiles calls visit with the CID of every file under root (root included
// when it is a file), reading the node offline. complete is false when the
// walk stopped short: it reached coverWalkNodes or coverWalkDepth, or a block
// was not on the node, in which case err is the first such failure.
func (s *Server) walkFiles(ctx context.Context, root string, visit func(c string)) (complete bool, err error) {
	type item struct {
		cid   string
		depth int
	}
	seen := map[string]struct{}{}
	stack := []item{{root, 0}}
	complete = true
	for len(stack) > 0 {
		if cerr := ctx.Err(); cerr != nil {
			return false, cerr
		}
		it := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, dup := seen[it.cid]; dup {
			continue
		}
		seen[it.cid] = struct{}{}
		if len(seen) > coverWalkNodes {
			return false, err
		}
		st, serr := s.statLocal(ctx, it.cid)
		if serr != nil {
			complete = false
			if err == nil {
				err = serr
			}
			continue
		}
		switch st.Type {
		case "file":
			visit(it.cid)
		case "directory":
			if it.depth >= coverWalkDepth {
				complete = false
				continue
			}
			kids, lerr := s.childrenLocal(ctx, it.cid)
			if lerr != nil {
				complete = false
				if err == nil {
					err = lerr
				}
				continue
			}
			for _, k := range kids {
				stack = append(stack, item{k, it.depth + 1})
			}
		}
	}
	return complete, err
}

// mfsCoverage answers whether the MFS holds a file, by walking the MFS once on
// first use. The walk starts from the CID of the MFS root, which is a node the
// daemon holds in memory, and reads everything below it offline (files/ls and
// files/stat on an MFS path would load directories through the online DAG
// service).
type mfsCoverage struct {
	s        *Server
	loaded   bool
	hashes   map[string]struct{} // multihashes of the files in the MFS
	complete bool
	err      error
}

func (m *mfsCoverage) load(ctx context.Context) {
	m.loaded = true
	m.hashes = map[string]struct{}{}
	var root statResponse
	cctx, cancel := context.WithTimeout(ctx, coverCallTimeout)
	defer cancel()
	if err := m.s.daemon.Call(cctx, "files/stat", []string{"/"}, map[string]any{"hash": true, "offline": true}, &root); err != nil || root.Hash == "" {
		if err == nil {
			err = fmt.Errorf("the MFS root has no CID")
		}
		m.err = err
		return
	}
	m.complete, m.err = m.s.walkFiles(ctx, root.Hash, func(c string) {
		if parsed, err := cid.Decode(c); err == nil {
			m.hashes[string(parsed.Hash())] = struct{}{}
		}
	})
}

// covers reports whether the MFS holds the file c, and whether that answer is
// known: a file not found in a walk that did not finish may still be in the
// part that was not read, so it is reported as unknown and kept.
func (m *mfsCoverage) covers(ctx context.Context, c string) (in, known bool) {
	if !m.loaded {
		m.load(ctx)
	}
	parsed, err := cid.Decode(c)
	if err != nil {
		return false, false
	}
	if _, ok := m.hashes[string(parsed.Hash())]; ok {
		return true, true
	}
	return false, m.complete
}

// reason says why the answer was unknown, for a warning.
func (m *mfsCoverage) reason() string {
	if m.err != nil {
		return clipMessage(m.err.Error())
	}
	return fmt.Sprintf("the MFS has more than %d entries", coverWalkNodes)
}
