package confirm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"
)

// Errors of the headless CLI confirmation.
var (
	// ErrCLINotEnabled is returned without cli confirmation (--confirm=cli or confirmation.cli).
	ErrCLINotEnabled = errors.New("ipfs mcp confirm needs cli confirmation (--confirm=cli or confirmation.cli, with the same --mcp-config as the server): it is the opt-in for headless servers; a desktop or native prompt is the safer choice")
	// ErrNoTTY is returned when stdin is not a terminal.
	ErrNoTTY = errors.New("ipfs mcp confirm needs a terminal on stdin; it will not read an approval from a pipe or a script")
	// ErrNoServer is returned when no server has a CLI channel.
	ErrNoServer = errors.New("no running ipfs mcp server with --confirm=cli was found")
)

// CLIOptions configure RunCLI.
type CLIOptions struct {
	// Modes is the value of --confirm; it must name cli.
	Modes []string
	// SocketDir overrides the default socket directory.
	SocketDir string
	// ID selects a confirmation; empty means the only one waiting.
	ID string
	// In is the terminal, Out the place the prompt is written.
	In  *os.File
	Out io.Writer
	// IsTerminal reports whether f is a terminal. Nil uses the real check.
	IsTerminal func(f *os.File) bool
}

// RunCLI is `ipfs mcp confirm`. It shows what is waiting, asks the person to
// type "yes", and sends the decision to the server's channel. It refuses
// without --confirm=cli and without a terminal, so a script or an agent that
// merely runs the command resolves nothing.
func RunCLI(ctx context.Context, o CLIOptions) error {
	cfg := Config{Modes: o.Modes, SocketDir: o.SocketDir}
	if !cfg.Has(ModeCLI) {
		return ErrCLINotEnabled
	}
	isTTY := o.IsTerminal
	if isTTY == nil {
		isTTY = func(f *os.File) bool { return f != nil && term.IsTerminal(int(f.Fd())) }
	}
	if !isTTY(o.In) {
		return ErrNoTTY
	}
	socks, _ := filepath.Glob(filepath.Join(cfg.socketDir(), "*.sock"))
	for _, sock := range socks {
		c := newCLIClient(sock)
		pending, err := c.pending(ctx)
		if err != nil {
			continue // stale socket or a server without the cli channel
		}
		chosen, err := choose(pending, o.ID)
		if err != nil {
			return err
		}
		if chosen == nil {
			continue
		}
		return decide(ctx, c, *chosen, o)
	}
	if o.ID != "" {
		return fmt.Errorf("no pending confirmation %s found", o.ID)
	}
	return ErrNoServer
}

// choose picks the confirmation to show.
func choose(pending []PendingView, id string) (*PendingView, error) {
	if id != "" {
		for i := range pending {
			if pending[i].ID == id {
				return &pending[i], nil
			}
		}
		return nil, nil
	}
	switch len(pending) {
	case 0:
		return nil, nil
	case 1:
		return &pending[0], nil
	}
	ids := make([]string, 0, len(pending))
	for _, p := range pending {
		ids = append(ids, sanitizeText(p.ID))
	}
	return nil, fmt.Errorf("several confirmations are waiting; name one: %s", strings.Join(ids, ", "))
}

// renderPending is what the terminal shows for a waiting confirmation. Every
// field came from the server, and the summary can carry text shaped by a tool
// argument, so each is sanitized: an escape sequence or a carriage return must
// not overwrite the operation the person is about to approve.
func renderPending(p PendingView) string {
	return fmt.Sprintf("Confirmation %s (%s, %s)\n  %s\n  expires %s\n",
		sanitizeText(p.ID), sanitizeText(p.Tool), sanitizeText(p.Class), sanitizeText(p.Summary),
		p.ExpiresAt.Local().Format(time.TimeOnly))
}

// ErrAnswerExpired is returned when the confirmation expired before an answer.
var ErrAnswerExpired = errors.New("the confirmation expired before you answered; nothing was approved")

// readAnswer reads one line from in, and gives up when ctx ends or the
// confirmation expires. The read itself cannot be interrupted on a terminal, so
// it runs in a goroutine that is left behind; the command exits right after.
func readAnswer(ctx context.Context, in io.Reader, expires time.Time) (string, error) {
	type result struct {
		line string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		line, err := bufio.NewReader(in).ReadString('\n')
		done <- result{line, err}
	}()
	var expiry <-chan time.Time
	if !expires.IsZero() {
		timer := time.NewTimer(time.Until(expires))
		defer timer.Stop()
		expiry = timer.C
	}
	select {
	case r := <-done:
		if r.err != nil && r.line == "" {
			return "", fmt.Errorf("read answer: %w", r.err)
		}
		return r.line, nil
	case <-expiry:
		return "", ErrAnswerExpired
	case <-ctx.Done():
		return "", fmt.Errorf("no answer was read (%w); nothing was approved", ctx.Err())
	}
}

func decide(ctx context.Context, c *cliClient, p PendingView, o CLIOptions) error {
	out := o.Out
	// If the person cannot be shown the operation, nothing is approved.
	if _, err := fmt.Fprint(out, renderPending(p)); err != nil {
		return fmt.Errorf("could not show the operation (%w); nothing was approved", err)
	}
	if _, err := fmt.Fprint(out, "Type 'yes' to approve, anything else to deny: "); err != nil {
		return fmt.Errorf("could not show the prompt (%w); nothing was approved", err)
	}
	line, err := readAnswer(ctx, o.In, p.ExpiresAt)
	if err != nil {
		return err
	}
	approve := strings.EqualFold(strings.TrimSpace(line), "yes")
	if err := c.resolve(ctx, p.ID, approve); err != nil {
		return err
	}
	if approve {
		fmt.Fprintln(out, "Approved.")
	} else {
		fmt.Fprintln(out, "Denied.")
	}
	return nil
}

// cliClient talks to one channel socket.
type cliClient struct {
	http *http.Client
}

func newCLIClient(sock string) *cliClient {
	return &cliClient{http: &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", sock)
			},
		},
	}}
}

func (c *cliClient) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://confirm"+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set(cliHeader, cliValue)
	return c.http.Do(req)
}

func (c *cliClient) pending(ctx context.Context) ([]PendingView, error) {
	resp, err := c.do(ctx, http.MethodGet, pendingPath, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("channel answered %s", resp.Status)
	}
	var out struct {
		Pending []PendingView `json:"pending"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Pending, nil
}

func (c *cliClient) resolve(ctx context.Context, id string, approve bool) error {
	resp, err := c.do(ctx, http.MethodPost, resolvePath, map[string]any{"id": id, "approve": approve})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var e struct {
		Error string `json:"error"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&e)
	if e.Error == "" {
		e.Error = resp.Status
	}
	return fmt.Errorf("not resolved: %s", e.Error)
}
