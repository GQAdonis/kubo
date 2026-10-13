package contract

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
)

const (
	contractBase = "file:///contract/"
	metaPrefix   = "https://json-schema.org/draft/2020-12/"
)

// loader resolves contract-relative references from the embedded files and
// the 2020-12 meta-schema from testdata. Nothing is fetched from the network.
func loader(uri *url.URL) (*jsonschema.Schema, error) {
	u := *uri
	u.Fragment = ""
	s := u.String()
	var data []byte
	var err error
	switch {
	case strings.HasPrefix(s, contractBase):
		data, err = fs.ReadFile(files, strings.TrimPrefix(s, contractBase))
	case strings.HasPrefix(s, metaPrefix):
		data, err = os.ReadFile(filepath.Join("testdata", "metaschema", "draft2020-12", filepath.FromSlash(strings.TrimPrefix(s, metaPrefix))+".json"))
	default:
		return nil, fmt.Errorf("refusing to load %s", s)
	}
	if err != nil {
		return nil, err
	}
	var sch jsonschema.Schema
	if err := json.Unmarshal(data, &sch); err != nil {
		return nil, fmt.Errorf("unmarshal %s: %w", s, err)
	}
	return &sch, nil
}

// resolveRef resolves a schema reference such as "schemas/common.json#/$defs/Cid"
// or "catalog.schema.json".
func resolveRef(t *testing.T, ref string) *jsonschema.Resolved {
	t.Helper()
	wrapper := &jsonschema.Schema{Ref: contractBase + ref}
	rs, err := wrapper.Resolve(&jsonschema.ResolveOptions{BaseURI: contractBase + "_wrapper.json", Loader: loader})
	if err != nil {
		t.Fatalf("resolve %s: %v", ref, err)
	}
	return rs
}

var metaResolved *jsonschema.Resolved

func metaSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	if metaResolved != nil {
		return metaResolved
	}
	wrapper := &jsonschema.Schema{Ref: metaPrefix + "schema"}
	rs, err := wrapper.Resolve(&jsonschema.ResolveOptions{BaseURI: contractBase + "_meta.json", Loader: loader})
	if err != nil {
		t.Fatalf("resolve 2020-12 meta-schema: %v", err)
	}
	metaResolved = rs
	return rs
}

// checkAgainstMeta validates a schema document against the 2020-12
// meta-schema and checks that jsonschema-go accepts it.
func checkAgainstMeta(t *testing.T, name string, data []byte) error {
	t.Helper()
	var inst any
	if err := json.Unmarshal(data, &inst); err != nil {
		return fmt.Errorf("%s: invalid JSON: %w", name, err)
	}
	if err := metaSchema(t).Validate(inst); err != nil {
		return fmt.Errorf("%s: not valid JSON Schema 2020-12: %w", name, err)
	}
	var sch jsonschema.Schema
	if err := json.Unmarshal(data, &sch); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if _, err := sch.Resolve(&jsonschema.ResolveOptions{BaseURI: contractBase + name, Loader: loader}); err != nil {
		return fmt.Errorf("%s: does not resolve: %w", name, err)
	}
	return nil
}

func readJSON(t *testing.T, name string) any {
	t.Helper()
	data, err := fs.ReadFile(files, name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return v
}

func readInto(t *testing.T, name string, v any) {
	t.Helper()
	data, err := fs.ReadFile(files, name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
}

// jsonFiles lists embedded .json files under dir.
func jsonFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := fs.WalkDir(files, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && path.Ext(p) == ".json" {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}

func mustLoad(t *testing.T) *Catalog {
	t.Helper()
	c, err := Load()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return c
}
