package embeddings

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbedderKeyNeedsHTTPSOffLoopback(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		url    string
		key    Secret
		wantOK bool
	}{
		{"https remote with key", "https://api.example.com/v1", "KEY-CANARY-77", true},
		{"http remote with key refused", "http://api.example.com/v1", "KEY-CANARY-77", false},
		{"http remote without key allowed", "http://api.example.com/v1", "", true},
		{"http loopback ip with key", "http://127.0.0.1:11434/v1", "KEY-CANARY-77", true},
		{"http localhost with key", "http://localhost:11434/v1", "KEY-CANARY-77", true},
		{"http ipv6 loopback with key", "http://[::1]:11434/v1", "KEY-CANARY-77", true},
		{"http private lan address with key refused", "http://192.168.1.5:11434/v1", "KEY-CANARY-77", false},
		{"http lookalike host refused", "http://localhost.example.com/v1", "KEY-CANARY-77", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			e := EmbedderSettings{BaseURL: tt.url, Model: "bge-m3", Dimensions: 1024, APIKey: tt.key}
			errs := e.validate()
			if tt.wantOK {
				assert.Empty(t, errs)
				return
			}
			require.Len(t, errs, 1)
			assert.Contains(t, errs[0].Error(), "plain http")
			assert.NotContains(t, errs[0].Error(), string(tt.key), "the error never echoes the key")
		})
	}
}
