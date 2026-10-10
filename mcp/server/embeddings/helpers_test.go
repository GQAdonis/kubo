package embeddings

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	require.NoError(t, err)
	return string(b)
}

// recorded is a request the fake saw.
type recorded struct {
	Method, Path, Auth, NS, DB, Body string
}

// replayServer answers each request with the first reply whose match appears
// in the request body, and records every request.
type replayServer struct {
	*httptest.Server
	mu   sync.Mutex
	seen []recorded
}

type reply struct {
	match  string
	status int
	body   string
}

func newReplayServer(t *testing.T, replies ...reply) *replayServer {
	t.Helper()
	rs := &replayServer{}
	rs.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := new(strings.Builder)
		b := make([]byte, 4096)
		for {
			n, err := r.Body.Read(b)
			buf.Write(b[:n])
			if err != nil {
				break
			}
		}
		rs.mu.Lock()
		rs.seen = append(rs.seen, recorded{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Surreal-NS"), r.Header.Get("Surreal-DB"), buf.String()})
		rs.mu.Unlock()
		for _, rp := range replies {
			if strings.Contains(buf.String(), rp.match) {
				status := rp.status
				if status == 0 {
					status = 200
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(rp.body))
				return
			}
		}
		http.Error(w, "no recorded reply", http.StatusTeapot)
	}))
	t.Cleanup(rs.Close)
	return rs
}

func (rs *replayServer) requests() []recorded {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return append([]recorded(nil), rs.seen...)
}
