package server

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
)

func init() { registerGroup(mfsTools) }

func mfsTools(s *Server) []toolImpl {
	return []toolImpl{
		{Name: "mfs_ls", Call: s.mfsLs},
		{Name: "mfs_stat", Call: s.mfsStat},
		{Name: "mfs_read", Call: s.mfsRead},
		{Name: "mfs_write", Call: s.mfsWrite},
		{Name: "mfs_mkdir", Call: s.mfsMkdir},
		{Name: "mfs_cp", Call: s.mfsCp},
		{Name: "mfs_mv", Call: s.mfsMv},
		{Name: "mfs_rm", Call: s.mfsRm},
	}
}

// requireMfsPath rejects the /ipfs/, /ipns/ and /ipld/ prefixes. The daemon
// resolves such paths as content paths, which would let a closed-world MFS
// tool read from the network.
func requireMfsPath(field, p string) error {
	for _, prefix := range []string{"/ipfs/", "/ipns/", "/ipld/"} {
		if strings.HasPrefix(p, prefix) {
			return invalidArgument(field + " must be an MFS path, not a content path; use mfs_cp to copy content into MFS")
		}
	}
	return nil
}

// requireMfsPaths validates the named fields; values come in pairs.
func requireMfsPaths(fieldAndPath ...string) error {
	for i := 0; i+1 < len(fieldAndPath); i += 2 {
		if err := requireMfsPath(fieldAndPath[i], fieldAndPath[i+1]); err != nil {
			return err
		}
	}
	return nil
}

func mfsType(t string) string {
	if t == "directory" {
		return "directory"
	}
	return "file"
}

// --- mfs_ls ---------------------------------------------------------------

type mfsLsInput struct {
	Path   string `json:"path"`
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

type mfsEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Size *int64 `json:"size,omitempty"`
	CID  string `json:"cid,omitempty"`
}

type mfsLsOutput struct {
	Path    string     `json:"path"`
	Entries []mfsEntry `json:"entries"`
	Page    pageInfo   `json:"page"`
}

// filesLsResponse is the files/ls response with long=true. Type is the MFS
// node type: 0 for a file, 1 for a directory.
type filesLsResponse struct {
	Entries []struct {
		Name string
		Type int
		Size int64
		Hash string
	}
}

func (s *Server) mfsLs(ctx context.Context, raw json.RawMessage) (any, error) {
	var in mfsLsInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	if in.Path == "" {
		in.Path = "/"
	}
	if err := requireMfsPath("path", in.Path); err != nil {
		return nil, err
	}
	limit, err := pageLimit(in.Limit)
	if err != nil {
		return nil, err
	}
	cur, err := decodeCursor(in.Cursor)
	if err != nil {
		return nil, err
	}
	var resp filesLsResponse
	if err := s.daemon.Call(ctx, "files/ls", []string{in.Path}, map[string]any{"long": true}, &resp); err != nil {
		return nil, err
	}
	entries := make([]mfsEntry, 0, len(resp.Entries))
	for _, e := range resp.Entries {
		typ, size := "file", e.Size
		if e.Type == 1 {
			typ = "directory"
		}
		entries = append(entries, mfsEntry{Name: e.Name, Type: typ, Size: &size, CID: e.Hash})
	}
	slices.SortFunc(entries, func(a, b mfsEntry) int { return strings.Compare(a.Name, b.Name) })
	page, info := pageByKey(entries, func(e mfsEntry) string { return e.Name }, cur, limit)
	if page == nil {
		page = []mfsEntry{}
	}
	return mfsLsOutput{Path: in.Path, Entries: page, Page: info}, nil
}

// --- mfs_stat -------------------------------------------------------------

type mfsPathInput struct {
	Path string `json:"path"`
}

type mfsStatOutput struct {
	Path           string `json:"path"`
	CID            string `json:"cid"`
	Type           string `json:"type"`
	Size           int64  `json:"size"`
	CumulativeSize int64  `json:"cumulative_size"`
	Blocks         int    `json:"blocks"`
}

func (s *Server) mfsStat(ctx context.Context, raw json.RawMessage) (any, error) {
	var in mfsPathInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	if err := requireMfsPath("path", in.Path); err != nil {
		return nil, err
	}
	st, err := s.stat(ctx, in.Path)
	if err != nil {
		return nil, err
	}
	return mfsStatOutput{
		Path: in.Path, CID: st.Hash, Type: mfsType(st.Type), Size: st.Size,
		CumulativeSize: st.CumulativeSize, Blocks: st.Blocks,
	}, nil
}

// --- mfs_read -------------------------------------------------------------

type mfsReadInput struct {
	Path     string `json:"path"`
	Offset   int64  `json:"offset"`
	Length   int64  `json:"length"`
	Encoding string `json:"encoding"`
}

type mfsReadOutput struct {
	Path      string `json:"path"`
	Offset    int64  `json:"offset"`
	Length    int64  `json:"length"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated"`
	Encoding  string `json:"encoding"`
	Content   string `json:"content"`
}

// mfsRead stats the file first: the size says how much of the requested range
// the cap cuts off, and a directory is refused with a clear message.
func (s *Server) mfsRead(ctx context.Context, raw json.RawMessage) (any, error) {
	var in mfsReadInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	if err := requireMfsPath("path", in.Path); err != nil {
		return nil, err
	}
	st, err := s.stat(ctx, in.Path)
	if err != nil {
		return nil, err
	}
	if st.Type == "directory" {
		return nil, invalidArgument(in.Path + " is a directory; use mfs_ls")
	}
	remaining := max(st.Size-in.Offset, 0)
	want := in.Length
	truncated := false
	if want == 0 {
		want = min(remaining, maxContentBytes)
		truncated = remaining > maxContentBytes
	}
	out := mfsReadOutput{Path: in.Path, Offset: in.Offset, Size: st.Size, Truncated: truncated, Encoding: utf8Encoding}
	if remaining == 0 {
		return out, nil
	}

	opts := map[string]any{"count": want}
	if in.Offset > 0 {
		opts["offset"] = in.Offset
	}
	var data []byte
	err = s.rawCall(ctx, "files/read", []string{in.Path}, opts, nil, func(r io.Reader) error {
		var rerr error
		data, rerr = readBounded(r, want)
		return rerr
	})
	if err != nil {
		return nil, err
	}
	content, enc, used := encodeContent(data, in.Encoding, truncated)
	out.Content, out.Encoding, out.Length = content, enc, int64(len(used))
	return out, nil
}

// --- mfs_write ------------------------------------------------------------

type mfsWriteInput struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
	Parents  bool   `json:"parents"`
}

type mfsWriteOutput struct {
	Path         string `json:"path"`
	CID          string `json:"cid"`
	BytesWritten int64  `json:"bytes_written"`
}

func (s *Server) mfsWrite(ctx context.Context, raw json.RawMessage) (any, error) {
	var in mfsWriteInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	if err := requireMfsPath("path", in.Path); err != nil {
		return nil, err
	}
	data, err := decodeContent(in.Content, in.Encoding)
	if err != nil {
		return nil, err
	}
	opts := map[string]any{"create": true, "truncate": true, "parents": in.Parents}
	// A zero-length upload slice must still be sent as a file, so it is never nil.
	err = s.rawCall(ctx, "files/write", []string{in.Path}, opts, append([]byte{}, data...), func(r io.Reader) error {
		_, cerr := io.Copy(io.Discard, r)
		return cerr
	})
	if err != nil {
		return nil, err
	}
	st, err := s.stat(ctx, in.Path)
	if err != nil {
		return nil, err
	}
	return mfsWriteOutput{Path: in.Path, CID: st.Hash, BytesWritten: int64(len(data))}, nil
}

// --- mfs_mkdir ------------------------------------------------------------

type mfsMkdirInput struct {
	Path    string `json:"path"`
	Parents bool   `json:"parents"`
}

type mfsMkdirOutput struct {
	Path string `json:"path"`
	CID  string `json:"cid"`
}

func (s *Server) mfsMkdir(ctx context.Context, raw json.RawMessage) (any, error) {
	var in mfsMkdirInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	if err := requireMfsPath("path", in.Path); err != nil {
		return nil, err
	}
	if err := s.daemon.Call(ctx, "files/mkdir", []string{in.Path}, map[string]any{"parents": in.Parents}, nil); err != nil {
		return nil, err
	}
	st, err := s.stat(ctx, in.Path)
	if err != nil {
		return nil, err
	}
	return mfsMkdirOutput{Path: in.Path, CID: st.Hash}, nil
}

// --- mfs_cp ---------------------------------------------------------------

type mfsCopyInput struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Parents     bool   `json:"parents"`
}

type mfsCopyOutput struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	CID         string `json:"cid"`
}

// mfsCp copies an MFS path or an /ipfs/ content path. The daemon refuses an
// existing destination because force is never sent.
func (s *Server) mfsCp(ctx context.Context, raw json.RawMessage) (any, error) {
	var in mfsCopyInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	if err := requireMfsPath("destination", in.Destination); err != nil {
		return nil, err
	}
	if strings.HasPrefix(in.Source, "/ipns/") || strings.HasPrefix(in.Source, "/ipld/") {
		return nil, invalidArgument("source must be an MFS path or an /ipfs/<cid> content path")
	}
	err := s.daemon.Call(ctx, "files/cp", []string{in.Source, in.Destination}, map[string]any{"parents": in.Parents}, nil)
	if err != nil {
		return nil, err
	}
	st, err := s.stat(ctx, in.Destination)
	if err != nil {
		return nil, err
	}
	return mfsCopyOutput{Source: in.Source, Destination: in.Destination, CID: st.Hash}, nil
}

// --- mfs_mv ---------------------------------------------------------------

type mfsMoveInput struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

type mfsMoveOutput struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
}

func (s *Server) mfsMv(ctx context.Context, raw json.RawMessage) (any, error) {
	var in mfsMoveInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	if err := requireMfsPaths("source", in.Source, "destination", in.Destination); err != nil {
		return nil, err
	}
	if err := s.daemon.Call(ctx, "files/mv", []string{in.Source, in.Destination}, nil, nil); err != nil {
		return nil, err
	}
	return mfsMoveOutput(in), nil
}

// --- mfs_rm ---------------------------------------------------------------

type mfsRmInput struct {
	Path      string `json:"path"`
	Recursive bool   `json:"recursive"`
}

type mfsRmOutput struct {
	Removed string `json:"removed"`
}

func (s *Server) mfsRm(ctx context.Context, raw json.RawMessage) (any, error) {
	var in mfsRmInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, invalidArgument(err.Error())
	}
	if err := requireMfsPath("path", in.Path); err != nil {
		return nil, err
	}
	if err := s.daemon.Call(ctx, "files/rm", []string{in.Path}, map[string]any{"recursive": in.Recursive}, nil); err != nil {
		return nil, err
	}
	return mfsRmOutput{Removed: in.Path}, nil
}
