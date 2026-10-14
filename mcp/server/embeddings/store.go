package embeddings

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Table and index names. The table holds one row per chunk of a CID.
const (
	chunkTable  = "ipfs_chunk"
	vectorIndex = "ipfs_chunk_vector"
	cidIndex    = "ipfs_chunk_cid"
)

// ErrDimensionMismatch is returned when the store's vector index was built for
// another dimension than the configured one.
var ErrDimensionMismatch = errors.New("vector index dimension differs from embeddings.embedder.dimensions")

// Row is one stored chunk of a document: its CID and where it came from, plus
// its embedding. The chunk text of a document indexed natively (Source "text"
// or empty) is not stored: a search re-reads it from the node. The text of a
// derived chunk (extracted, ocr, transcript) is stored in Text, because it is
// not a slice of the file's bytes.
type Row struct {
	CID        string
	Path       string
	MIME       string
	ChunkIndex int
	Model      string
	Vector     []float32
	// Kind is the content kind (text, document, code, data, image, video).
	Kind string
	// Source says how the chunk text was produced (text, extracted, ocr,
	// transcript, description).
	Source string
	// Text is the chunk text of a derived chunk; empty for Source "text".
	Text string
}

// Hit is one search result. Distance is the cosine distance (0 is identical).
type Hit struct {
	CID        string  `json:"cid"`
	Path       string  `json:"path"`
	MIME       string  `json:"mime"`
	ChunkIndex int     `json:"chunk_index"`
	Model      string  `json:"model"`
	Distance   float64 `json:"distance"`
	// Kind, Source and Text are empty for rows written before derived content
	// existed, which are natively indexed text.
	Kind   string `json:"kind"`
	Source string `json:"source"`
	Text   string `json:"text"`
}

// Derived reports whether the hit's text came from a derivation and is stored
// with the row, rather than being cut from the file's bytes on demand.
func (h Hit) Derived() bool { return h.Source != "" && h.Source != SourceText }

// Chunk sources, the contract's source values.
const (
	SourceText        = "text"
	SourceExtracted   = "extracted"
	SourceOCR         = "ocr"
	SourceTranscript  = "transcript"
	SourceDescription = "description"
)

// Store is the vector index. Implementations are safe for concurrent use.
type Store interface {
	// Bootstrap creates the schema and the HNSW index if they are missing and
	// fails if an existing index has another dimension.
	Bootstrap(ctx context.Context) error
	// Replace atomically swaps all rows of a CID for the given rows.
	Replace(ctx context.Context, cid string, rows []Row) error
	// Search returns the k rows nearest to the vector, nearest first.
	Search(ctx context.Context, vector []float32, k int) ([]Hit, error)
	// DeleteCID removes every row of a CID and returns how many it removed.
	DeleteCID(ctx context.Context, cid string) (int, error)
	// Count returns the number of stored rows.
	Count(ctx context.Context) (int, error)
	// SearchCID returns the k rows of one CID nearest to the vector, nearest
	// first. The CID is small, so the distance is computed per row and the
	// HNSW index is not used.
	SearchCID(ctx context.Context, vector []float32, cid string, k int) ([]Hit, error)
	// CountCID returns the number of rows of one CID.
	CountCID(ctx context.Context, cid string) (int, error)
	// CountCIDs returns the number of distinct CIDs that have rows.
	CountCIDs(ctx context.Context) (int, error)
	// ListCIDs returns up to limit distinct CIDs that sort after the given
	// one ("" starts at the beginning), in ascending order.
	ListCIDs(ctx context.Context, after string, limit int) ([]string, error)
}

// surrealStore is the Store over SurrealDB's HTTP API.
type surrealStore struct {
	c    *surrealClient
	dims int
}

func newSurrealStore(c *surrealClient, dims int) *surrealStore {
	return &surrealStore{c: c, dims: dims}
}

// Bootstrap implements Store.
func (s *surrealStore) Bootstrap(ctx context.Context) error {
	// Defining the namespace and database needs a root or namespace user. A
	// user scoped to an existing database cannot, and does not need to; if the
	// database is really missing the table definition below fails and this
	// error is reported with it.
	_, defErr := s.c.query(ctx, "DEFINE NAMESPACE IF NOT EXISTS `"+s.c.namespace+"`; "+
		"USE NS `"+s.c.namespace+"`; DEFINE DATABASE IF NOT EXISTS `"+s.c.database+"`;", nil)
	if errors.Is(defErr, ErrAuth) {
		return defErr
	}
	_, err := s.c.query(ctx, ddl(s.dims), nil)
	if err != nil {
		if defErr != nil {
			return fmt.Errorf("define schema: %w (defining the namespace and database also failed: %v)", err, defErr)
		}
		return fmt.Errorf("define schema: %w", err)
	}
	return s.checkDimension(ctx)
}

// ddl is the schema. IF NOT EXISTS keeps a bootstrap idempotent; a changed
// dimension is caught by checkDimension.
func ddl(dims int) string {
	n := strconv.Itoa(dims)
	return strings.Join([]string{
		"DEFINE TABLE IF NOT EXISTS " + chunkTable + " SCHEMAFULL;",
		"DEFINE FIELD IF NOT EXISTS cid ON " + chunkTable + " TYPE string;",
		"DEFINE FIELD IF NOT EXISTS path ON " + chunkTable + " TYPE string;",
		"DEFINE FIELD IF NOT EXISTS mime ON " + chunkTable + " TYPE string;",
		"DEFINE FIELD IF NOT EXISTS chunk_index ON " + chunkTable + " TYPE int;",
		"DEFINE FIELD IF NOT EXISTS model ON " + chunkTable + " TYPE string;",
		"DEFINE FIELD IF NOT EXISTS dims ON " + chunkTable + " TYPE int;",
		"DEFINE FIELD IF NOT EXISTS kind ON " + chunkTable + " TYPE option<string>;",
		"DEFINE FIELD IF NOT EXISTS source ON " + chunkTable + " TYPE option<string>;",
		"DEFINE FIELD IF NOT EXISTS text ON " + chunkTable + " TYPE option<string>;",
		"DEFINE FIELD IF NOT EXISTS vector ON " + chunkTable + " TYPE array<float, " + n + ">;",
		"DEFINE INDEX IF NOT EXISTS " + cidIndex + " ON " + chunkTable + " FIELDS cid;",
		"DEFINE INDEX IF NOT EXISTS " + vectorIndex + " ON " + chunkTable + " FIELDS vector HNSW DIMENSION " + n + " DIST COSINE TYPE F32;",
	}, " ")
}

var indexDimension = regexp.MustCompile(`DIMENSION\s+(\d+)`)

func (s *surrealStore) checkDimension(ctx context.Context) error {
	res, err := s.c.query(ctx, "INFO FOR TABLE "+chunkTable+";", nil)
	if err != nil {
		return fmt.Errorf("read index definition: %w", err)
	}
	if len(res) != 1 {
		return fmt.Errorf("read index definition: %d results, want 1", len(res))
	}
	var info struct {
		Indexes map[string]string `json:"indexes"`
	}
	if err := json.Unmarshal(res[0], &info); err != nil {
		return fmt.Errorf("read index definition: %w", err)
	}
	def, ok := info.Indexes[vectorIndex]
	if !ok {
		return fmt.Errorf("index %s is missing after bootstrap", vectorIndex)
	}
	m := indexDimension.FindStringSubmatch(def)
	if m == nil {
		return fmt.Errorf("index %s has no DIMENSION in its definition", vectorIndex)
	}
	if got, _ := strconv.Atoi(m[1]); got != s.dims {
		return fmt.Errorf("%w: the index in this database holds %d-dimensional vectors, the embedder is configured for %d; use another database or data_dir, or remove table %s", ErrDimensionMismatch, got, s.dims, chunkTable)
	}
	return nil
}

// rowID is the record id of a chunk: stable for a CID, chunk position and
// model, so indexing the same content again replaces its rows.
func rowID(cid string, chunk int, model string) string {
	sum := sha256.Sum256([]byte(cid + "\x00" + strconv.Itoa(chunk) + "\x00" + model))
	return hex.EncodeToString(sum[:])
}

type rowDoc struct {
	ID         string    `json:"id"`
	CID        string    `json:"cid"`
	Path       string    `json:"path"`
	MIME       string    `json:"mime"`
	ChunkIndex int       `json:"chunk_index"`
	Model      string    `json:"model"`
	Dims       int       `json:"dims"`
	Vector     []float32 `json:"vector"`
	// Kind, Source and Text are left out when empty so SurrealDB stores NONE
	// for an option<string> field, never NULL.
	Kind   string `json:"kind,omitempty"`
	Source string `json:"source,omitempty"`
	Text   string `json:"text,omitempty"`
}

// Replace implements Store.
func (s *surrealStore) Replace(ctx context.Context, cid string, rows []Row) error {
	if cid == "" {
		return errors.New("replace rows: empty CID")
	}
	docs := make([]rowDoc, 0, len(rows))
	for _, r := range rows {
		if r.CID != cid {
			return fmt.Errorf("replace rows: row for %q in a replacement of %q", r.CID, cid)
		}
		if len(r.Vector) != s.dims {
			return fmt.Errorf("replace rows: vector has %d dimensions, want %d", len(r.Vector), s.dims)
		}
		docs = append(docs, rowDoc{
			ID: rowID(r.CID, r.ChunkIndex, r.Model), CID: r.CID, Path: r.Path, MIME: r.MIME,
			ChunkIndex: r.ChunkIndex, Model: r.Model, Dims: len(r.Vector), Vector: r.Vector,
			Kind: r.Kind, Source: r.Source, Text: r.Text,
		})
	}
	_, err := s.c.query(ctx, "BEGIN TRANSACTION; "+
		"DELETE "+chunkTable+" WHERE cid = $cid; "+
		"FOR $r IN $rows { UPSERT type::record('"+chunkTable+"', $r.id) CONTENT { cid: $r.cid, path: $r.path, mime: $r.mime, "+
		"chunk_index: $r.chunk_index, model: $r.model, dims: $r.dims, vector: $r.vector, "+
		"kind: $r.kind, source: $r.source, text: $r.text }; }; "+
		"COMMIT TRANSACTION;", map[string]any{"cid": cid, "rows": docs})
	if err != nil {
		return fmt.Errorf("store rows of %s: %w", cid, err)
	}
	return nil
}

// efFor picks the HNSW exploration factor for k results.
func efFor(k int) int { return max(40, 4*k) }

// Search implements Store.
func (s *surrealStore) Search(ctx context.Context, vector []float32, k int) ([]Hit, error) {
	if k < 1 {
		return nil, errors.New("search: k must be at least 1")
	}
	if len(vector) != s.dims {
		return nil, fmt.Errorf("search: vector has %d dimensions, want %d", len(vector), s.dims)
	}
	// k and ef are integers formatted here, never operator or model text.
	surql := "SELECT cid, path, mime, chunk_index, model, kind, source, text, vector::distance::knn() AS distance FROM " + chunkTable +
		" WHERE vector <|" + strconv.Itoa(k) + "," + strconv.Itoa(efFor(k)) + "|> $v ORDER BY distance;"
	res, err := s.c.query(ctx, surql, map[string]any{"v": vector})
	if err != nil {
		return nil, fmt.Errorf("vector search: %w", err)
	}
	if len(res) != 1 {
		return nil, fmt.Errorf("vector search: %d results, want 1", len(res))
	}
	var hits []Hit
	if err := json.Unmarshal(res[0], &hits); err != nil {
		return nil, fmt.Errorf("vector search: decode hits: %w", err)
	}
	return hits, nil
}

// DeleteCID implements Store.
func (s *surrealStore) DeleteCID(ctx context.Context, cid string) (int, error) {
	if cid == "" {
		return 0, errors.New("delete rows: empty CID")
	}
	res, err := s.c.query(ctx, "DELETE "+chunkTable+" WHERE cid = $cid RETURN cid;", map[string]any{"cid": cid})
	if err != nil {
		return 0, fmt.Errorf("delete rows of %s: %w", cid, err)
	}
	var removed []json.RawMessage
	if len(res) == 1 {
		if err := json.Unmarshal(res[0], &removed); err != nil {
			return 0, fmt.Errorf("delete rows of %s: decode result: %w", cid, err)
		}
	}
	return len(removed), nil
}

// Count implements Store.
func (s *surrealStore) Count(ctx context.Context) (int, error) {
	res, err := s.c.query(ctx, "SELECT count() AS n FROM "+chunkTable+" GROUP ALL;", nil)
	if err != nil {
		return 0, fmt.Errorf("count rows: %w", err)
	}
	if len(res) != 1 {
		return 0, fmt.Errorf("count rows: %d results, want 1", len(res))
	}
	var rows []struct {
		N int `json:"n"`
	}
	if err := json.Unmarshal(res[0], &rows); err != nil {
		return 0, fmt.Errorf("count rows: %w", err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].N, nil
}

// SearchCID implements Store.
func (s *surrealStore) SearchCID(ctx context.Context, vector []float32, cid string, k int) ([]Hit, error) {
	if k < 1 {
		return nil, errors.New("search: k must be at least 1")
	}
	if cid == "" {
		return nil, errors.New("search: empty CID")
	}
	if len(vector) != s.dims {
		return nil, fmt.Errorf("search: vector has %d dimensions, want %d", len(vector), s.dims)
	}
	surql := "SELECT cid, path, mime, chunk_index, model, kind, source, text, 1 - vector::similarity::cosine(vector, $v) AS distance FROM " + chunkTable +
		" WHERE cid = $cid ORDER BY distance LIMIT " + strconv.Itoa(k) + ";"
	res, err := s.c.query(ctx, surql, map[string]any{"v": vector, "cid": cid})
	if err != nil {
		return nil, fmt.Errorf("vector search in %s: %w", cid, err)
	}
	if len(res) != 1 {
		return nil, fmt.Errorf("vector search: %d results, want 1", len(res))
	}
	var hits []Hit
	if err := json.Unmarshal(res[0], &hits); err != nil {
		return nil, fmt.Errorf("vector search: decode hits: %w", err)
	}
	return hits, nil
}

// countOf runs a statement that selects one `count() AS n` row (none when the
// table or the match is empty).
func (s *surrealStore) countOf(ctx context.Context, what, surql string, vars map[string]any) (int, error) {
	res, err := s.c.query(ctx, surql, vars)
	if err != nil {
		return 0, fmt.Errorf("count %s: %w", what, err)
	}
	if len(res) != 1 {
		return 0, fmt.Errorf("count %s: %d results, want 1", what, len(res))
	}
	var rows []struct {
		N int `json:"n"`
	}
	if err := json.Unmarshal(res[0], &rows); err != nil {
		return 0, fmt.Errorf("count %s: %w", what, err)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	return rows[0].N, nil
}

// CountCID implements Store.
func (s *surrealStore) CountCID(ctx context.Context, cid string) (int, error) {
	if cid == "" {
		return 0, errors.New("count rows: empty CID")
	}
	return s.countOf(ctx, "rows of "+cid, "SELECT count() AS n FROM "+chunkTable+" WHERE cid = $cid GROUP ALL;", map[string]any{"cid": cid})
}

// CountCIDs implements Store.
func (s *surrealStore) CountCIDs(ctx context.Context) (int, error) {
	return s.countOf(ctx, "indexed CIDs", "SELECT count() AS n FROM (SELECT cid FROM "+chunkTable+" GROUP BY cid) GROUP ALL;", nil)
}

// ListCIDs implements Store.
func (s *surrealStore) ListCIDs(ctx context.Context, after string, limit int) ([]string, error) {
	if limit < 1 {
		return nil, errors.New("list CIDs: limit must be at least 1")
	}
	surql := "SELECT cid FROM " + chunkTable + " WHERE cid > $after GROUP BY cid ORDER BY cid LIMIT " + strconv.Itoa(limit) + ";"
	res, err := s.c.query(ctx, surql, map[string]any{"after": after})
	if err != nil {
		return nil, fmt.Errorf("list indexed CIDs: %w", err)
	}
	if len(res) != 1 {
		return nil, fmt.Errorf("list indexed CIDs: %d results, want 1", len(res))
	}
	var rows []struct {
		CID string `json:"cid"`
	}
	if err := json.Unmarshal(res[0], &rows); err != nil {
		return nil, fmt.Errorf("list indexed CIDs: %w", err)
	}
	cids := make([]string, 0, len(rows))
	for _, r := range rows {
		cids = append(cids, r.CID)
	}
	return cids, nil
}
