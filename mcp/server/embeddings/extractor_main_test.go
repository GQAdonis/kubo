package embeddings

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"
)

// TestMain lets the test binary stand in for `xberg`: the fake xberg scripts
// of the supervisor tests exec it with FAKE_XBERG=1.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_XBERG") == "1" {
		os.Exit(runFakeXberg(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// fakeXbergRecord is what the fake writes into its working directory (the
// extractor cache_dir) so a test can read how it was started.
type fakeXbergRecord struct {
	Args []string `json:"args"`
	Env  []string `json:"env"`
	Cwd  string   `json:"cwd"`
}

func runFakeXberg(args []string) int {
	if len(args) == 1 && args[0] == "--version" {
		v := os.Getenv("FAKE_XBERG_VERSION")
		if v == "" {
			v = "1.3.10"
		}
		fmt.Println("xberg " + v)
		return 0
	}
	if len(args) == 0 || args[0] != "serve" {
		return 2
	}
	healthVersion := os.Getenv("FAKE_XBERG_HEALTH_VERSION")
	if healthVersion == "" {
		healthVersion = os.Getenv("FAKE_XBERG_VERSION")
	}
	if healthVersion == "" {
		healthVersion = "1.3.10"
	}
	var host, port string
	for i := 1; i+1 < len(args); i++ {
		switch args[i] {
		case "--host":
			host = args[i+1]
		case "--port":
			port = args[i+1]
		}
	}
	cwd, _ := os.Getwd()
	rec, _ := json.Marshal(fakeXbergRecord{Args: args, Env: os.Environ(), Cwd: cwd})
	if err := os.WriteFile("fake-xberg.json", rec, 0o600); err != nil {
		return 3
	}
	l, err := net.Listen("tcp", net.JoinHostPort(host, port))
	if err != nil {
		return 4
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"status":"healthy","version":"`+healthVersion+`"}`)
	})
	mux.HandleFunc("/extract", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"results":[{"content":"from the fake xberg","mime_type":"application/pdf","metadata":{}}]}`)
	})
	go func() { _ = http.Serve(l, mux) }()
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	<-sig
	return 0
}
