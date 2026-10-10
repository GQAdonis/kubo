package embeddings

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func validEmbedder() EmbedderSettings {
	return EmbedderSettings{BaseURL: "http://127.0.0.1:9/v1", Model: "bge-m3", Dimensions: 1024}
}

func TestSettingsValidate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		s    Settings
		want string // substring of the error, "" for valid
	}{
		{"embedded ok", Settings{Mode: ModeEmbedded, Embedded: &EmbeddedSettings{Binary: "/opt/surreal", DataDir: "/var/idx"}, Embedder: validEmbedder()}, ""},
		{"remote ok", Settings{Mode: ModeRemote, Remote: &RemoteSettings{Endpoint: "https://db.example:8000", Namespace: "ipfs", Database: "idx"}, Embedder: validEmbedder()}, ""},
		{"relative binary refused", Settings{Mode: ModeEmbedded, Embedded: &EmbeddedSettings{Binary: "surreal", DataDir: "/var/idx"}, Embedder: validEmbedder()}, "surreal_binary"},
		{"relative data dir refused", Settings{Mode: ModeEmbedded, Embedded: &EmbeddedSettings{Binary: "/opt/surreal", DataDir: "idx"}, Embedder: validEmbedder()}, "data_dir"},
		{"embedded block missing", Settings{Mode: ModeEmbedded, Embedder: validEmbedder()}, "embeddings.embedded is required"},
		{"remote block missing names the field", Settings{Mode: ModeRemote, Embedder: validEmbedder()}, "embeddings.remote.endpoint"},
		{"remote without URL names the field", Settings{Mode: ModeRemote, Remote: &RemoteSettings{Namespace: "n", Database: "d"}, Embedder: validEmbedder()}, "embeddings.remote.endpoint"},
		{"remote namespace with a quote refused", Settings{Mode: ModeRemote, Remote: &RemoteSettings{Endpoint: "http://h:1", Namespace: "a`b", Database: "d"}, Embedder: validEmbedder()}, "embeddings.remote.namespace"},
		{"URL with credentials refused", Settings{Mode: ModeRemote, Remote: &RemoteSettings{Endpoint: "http://u:p@h:1", Namespace: "n", Database: "d"}, Embedder: validEmbedder()}, "must not carry credentials"},
		{"unknown mode", Settings{Mode: "local", Embedder: validEmbedder()}, "embeddings.mode"},
		{"mode embedded with remote block", Settings{Mode: ModeEmbedded, Embedded: &EmbeddedSettings{Binary: "/b", DataDir: "/d"}, Remote: &RemoteSettings{}, Embedder: validEmbedder()}, "embeddings.remote is set"},
		{"no embedder URL", Settings{Mode: ModeEmbedded, Embedded: &EmbeddedSettings{Binary: "/b", DataDir: "/d"}, Embedder: EmbedderSettings{Model: "m", Dimensions: 8}}, "embeddings.embedder.base_url"},
		{"dimensions too large", Settings{Mode: ModeEmbedded, Embedded: &EmbeddedSettings{Binary: "/b", DataDir: "/d"}, Embedder: EmbedderSettings{BaseURL: "http://h:1", Model: "m", Dimensions: 9000}}, "dimensions"},
		{"bad min version", Settings{Mode: ModeEmbedded, Embedded: &EmbeddedSettings{Binary: "/b", DataDir: "/d", MinVersion: "three"}, Embedder: validEmbedder()}, "min_version"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.s.Validate()
			if tt.want == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func TestSecretNeverPrintsItsValue(t *testing.T) {
	t.Parallel()
	const value = "hunter2-very-secret"
	s := Settings{Embedder: EmbedderSettings{APIKey: Secret(value)}, Remote: &RemoteSettings{Credential: Secret("root:" + value)}}
	j, err := json.Marshal(s)
	require.NoError(t, err)
	var logbuf strings.Builder
	slog.New(slog.NewTextHandler(&logbuf, nil)).Info("settings", "key", s.Embedder.APIKey, "all", s)
	for name, got := range map[string]string{
		"%v":   fmt.Sprintf("%v", s),
		"%+v":  fmt.Sprintf("%+v", s),
		"%#v":  fmt.Sprintf("%#v", s),
		"json": string(j),
		"slog": logbuf.String(),
	} {
		require.NotContains(t, got, value, name)
	}
	require.Equal(t, "<redacted>", Secret(value).String())
	require.Equal(t, value, Secret(value).Reveal())
}

func TestParseCredential(t *testing.T) {
	t.Parallel()
	require.Equal(t, auth{}, parseCredential(""))
	require.Equal(t, auth{user: "root", pass: "p:w"}, parseCredential("root:p:w"))
	require.Equal(t, auth{token: "eyJhbGciOi.payload.sig"}, parseCredential("eyJhbGciOi.payload.sig"))
}
