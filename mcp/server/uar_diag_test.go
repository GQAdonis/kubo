//go:build !noagent

package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithUARSkillVersion(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"adds the key after the opening delimiter": {
			in:   "---\nname: a\ndescription: d\n---\nbody\n",
			want: "---\nversion: 1.0.0\nname: a\ndescription: d\n---\nbody\n",
		},
		"keeps an existing version": {
			in:   "---\nname: a\nversion: 2.1.0\ndescription: d\n---\nbody\n",
			want: "---\nname: a\nversion: 2.1.0\ndescription: d\n---\nbody\n",
		},
		"a nested version is not the top-level key": {
			in:   "---\nname: a\nmetadata:\n  version: x\n---\nb\n",
			want: "---\nversion: 1.0.0\nname: a\nmetadata:\n  version: x\n---\nb\n",
		},
		"CRLF line endings": {
			in:   "---\r\nname: a\r\n---\r\nb\r\n",
			want: "---\r\nversion: 1.0.0\r\nname: a\r\n---\r\nb\r\n",
		},
		"no frontmatter is left for UAR to refuse": {in: "# no frontmatter\n", want: "# no frontmatter\n"},
		"unterminated frontmatter is left alone":   {in: "---\nname: a\n", want: "---\nname: a\n"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, c.want, withUARSkillVersion(c.in))
		})
	}
}

// Every skill the repository ships gets the key and so passes the fake's copy
// of UAR's required-field check.
func TestShippedSkillsCarryTheVersionUARRequires(t *testing.T) {
	skills, err := loadUARSkills(repoSkillsDir(t))
	require.NoError(t, err)
	require.NotEmpty(t, skills)
	for _, sk := range skills {
		assert.Regexp(t, `(?m)^version:\s*\S+`, sk.Markdown, sk.Name)
	}
}

func TestUARSkillImportRefusedByTheInstanceIsDiagnosable(t *testing.T) {
	// A skill whose frontmatter lacks "name" is refused by the fake as UAR
	// refuses it; the operator's log names the request and carries UAR's text.
	dir := oneSkillDir(t, "alpha", "---\ndescription: d\n---\nbody\n")
	e := newUARProxyEnv(t, replay(t, "run-text.sse"), uarProxyOpts{skillsDir: dir})
	r := e.say("t1", "r1", "hello")
	require.Equal(t, http.StatusBadGateway, r.Status, r.Body)
	assert.Contains(t, r.Body, "HTTP 400")
	assert.NotContains(t, r.Body, "skill_markdown_invalid", "the client never sees UAR's words")
	logs := e.logs.String()
	assert.Contains(t, logs, "op=\"skill import\"")
	assert.Contains(t, logs, "status=400")
	assert.Contains(t, logs, "method=POST")
	assert.Contains(t, logs, "path=/api/uar/skills/import-content")
	assert.Contains(t, logs, "skill_markdown_invalid")
}

func TestUARRunErrorBodyIsLoggedScrubbedAndCut(t *testing.T) {
	long := strings.Repeat("x", 2000)
	body := `{"code":"run_mcp_destination_unregistered","error":"leak ` + fakeUARToken + ` ` + uarTestMCPToken + ` ` + long + `"}`
	e := newUARProxyEnv(t, func(int, map[string]any) (int, []byte) { return http.StatusUnprocessableEntity, []byte(body) }, uarProxyOpts{})
	r := e.say("t1", "r1", "hello")
	require.Equal(t, http.StatusBadGateway, r.Status, r.Body)
	assert.Contains(t, r.Body, "HTTP 422")
	assert.NotContains(t, r.Body, "run_mcp_destination_unregistered", "the client-facing error has no UAR body")

	logs := e.logs.String()
	assert.Contains(t, logs, "op=run")
	assert.Contains(t, logs, "status=422")
	assert.Contains(t, logs, "path=/api/uar/ag-ui/agents/kubo-ipfs")
	assert.Contains(t, logs, "run_mcp_destination_unregistered")
	assert.NotContains(t, logs, fakeUARToken, "the UAR key is scrubbed")
	assert.NotContains(t, logs, uarTestMCPToken, "the run grant is scrubbed")
	assert.Contains(t, logs, "[redacted]")
	assert.NotContains(t, logs, strings.Repeat("x", maxUARLoggedBody), "the excerpt is cut")
}
