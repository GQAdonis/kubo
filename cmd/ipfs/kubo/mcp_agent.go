//go:build !nomcp && !noagent

package kubo

import (
	"fmt"
	"os"
	"slices"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	mcpserver "github.com/ipfs/kubo/mcp/server"
)

// Options of the agent backend. None has a default: without a base URL and a
// model (here or in the llm section of --mcp-config) the agent is off and no
// model call is ever made; an enabled /agui stays mounted and answers 503
// backend_not_configured until a backend is configured.
const (
	mcpLLMBaseURLOptionName = "llm-base-url"
	mcpLLMModelOptionName   = "llm-model"
	mcpLLMKeyEnvOptionName  = "llm-api-key-env"
	mcpLLMKeyFileOptionName = "llm-api-key-file"
	mcpAGUIOptionName       = "agui"
	mcpA2AOptionName        = "a2a"
)

// mcpAgentHelp is added to the command help only in builds that have the agent
// code, so a noagent build does not document flags it lacks.
const mcpAgentHelp = `Agent backend. The agent loop is off unless you name a model server: it has
no default endpoint and no default model, and without both no model call is
ever made. --llm-base-url and --llm-model (or the llm section of --mcp-config)
name an OpenAI-compatible server, for example one you run locally. The API key
is never a literal: --llm-api-key-env takes the NAME of an environment
variable and --llm-api-key-file an absolute path (or llm.api_key in the config
file). Node content reaches the model only when the server's host is in the
principal's egress_ack. Remove the agent code at build time with:
make build GOTAGS=noagent

AG-UI. --agui (or "agui": true under "agent" in the config file) also
serves POST /agui on the --http listener, for chat UIs such as the Web UI
chat. It is off by default and mounted only with --http; without --agui the
path answers 404. With no model backend the mounted endpoint answers every
run request 503 backend_not_configured until one is configured, which the
settings API can do without a restart. It accepts only a bearer token of a
principal of kind ui. A write the chat asks for ends the run with an
interrupt, and only a user gesture in the chat approves it; a destructive
call also needs the system prompt. A browser page may call it only from an
origin listed in "origin_allowlist" under "agent" (none by default). Tool
results are also streamed as A2UI surface messages; set "a2ui": false under
"agent" to turn those off (the run stream is otherwise unchanged).

A2A. --a2a (or "a2a": true under "agent" in the config file) also serves the
A2A 1.0 JSON-RPC endpoint POST /a2a and the agent card at
/.well-known/agent-card.json on the --http listener, so other agents can
delegate IPFS work to this node. It is off by default and mounted only with
--http and a model backend; otherwise both paths answer 404. /a2a accepts only
a bearer token of a principal of kind a2a, read-only unless the config file
grants write groups. A2A peers can never approve a change: a write or
destructive call leaves the task input-required until a person approves it on
this machine (the system prompt or the paired Desktop window).
`

func init() {
	const marker = "Remove the command at build time with: make build GOTAGS=nomcp"
	mcpCmd.Helptext.LongDescription = strings.Replace(mcpCmd.Helptext.LongDescription, marker, mcpAgentHelp+marker, 1)
	mcpCmd.Options = append(mcpCmd.Options,
		cmds.StringOption(mcpLLMBaseURLOptionName, "Base URL of an OpenAI-compatible model server for the agent loop. No default; unset means no agent."),
		cmds.StringOption(mcpLLMModelOptionName, "Model name sent to the model server. No default."),
		cmds.StringOption(mcpLLMKeyEnvOptionName, "NAME of the environment variable that holds the model server's API key. A literal key is rejected."),
		cmds.StringOption(mcpLLMKeyFileOptionName, "Absolute path of a file that holds the model server's API key."),
		cmds.BoolOption(mcpA2AOptionName, "Serve the A2A endpoint POST /a2a and the agent card on the --http listener (also agent.a2a in --mcp-config). Needs a model backend. Off by default.").WithDefault(false),
		cmds.BoolOption(mcpAGUIOptionName, "Serve the AG-UI endpoint POST /agui on the --http listener (also agent.agui in --mcp-config). With no model backend it answers 503 backend_not_configured. Off by default.").WithDefault(false),
	)
}

// agentInputs are the --llm-* flags of one run.
type agentInputs struct {
	over mcpserver.LLMOverrides
	agui bool
	a2a  bool
}

// agentResolved is the backend settings after flags and config file are
// combined; nil means the agent is off.
type agentResolved = *mcpserver.AgentSettings

func agentInputsFromRequest(req *cmds.Request) agentInputs {
	str := func(name string) string {
		v, _ := req.Options[name].(string)
		return v
	}
	agui, _ := req.Options[mcpAGUIOptionName].(bool)
	a2a, _ := req.Options[mcpA2AOptionName].(bool)
	return agentInputs{agui: agui, a2a: a2a, over: mcpserver.LLMOverrides{
		BaseURL: str(mcpLLMBaseURLOptionName),
		Model:   str(mcpLLMModelOptionName),
		KeyEnv:  str(mcpLLMKeyEnvOptionName),
		KeyFile: str(mcpLLMKeyFileOptionName),
	}}
}

// validateAgent resolves the agent settings without contacting anything, so a
// literal key or a half-configured backend fails before the daemon is
// contacted.
func validateAgent(s *mcpSettings) error {
	set, err := s.cfg.ResolveAgent(s.agent.over)
	if err != nil {
		return err
	}
	s.resolved = set
	return nil
}

// enableMCPAgent attaches the agent loop when a backend is configured and
// mounts the enabled agent surfaces on the --http listener. With no backend
// nothing is built or dialed; an enabled /agui is still mounted and answers
// 503 backend_not_configured, while /a2a needs a backend and stays unmounted.
func enableMCPAgent(srv *mcpserver.Server, s *mcpSettings) error {
	wantAGUI := s.agent.agui || s.cfg.Agent.AGUI
	wantA2A := s.agent.a2a || s.cfg.Agent.A2A
	if s.resolved == nil {
		if wantA2A {
			fmt.Fprintln(os.Stderr, "ipfs mcp: --a2a is ignored: no model backend is configured (--llm-base-url and --llm-model), so its routes are not served")
			wantA2A = false
		}
	} else {
		if err := srv.EnableAgent(s.resolved); err != nil {
			return fmt.Errorf("agent backend: %w", err)
		}
		fmt.Fprintln(os.Stderr, "ipfs mcp: agent loop enabled: "+s.resolved.Describe())
	}
	if s.http == nil {
		for flag, want := range map[string]bool{"agui": wantAGUI, "a2a": wantA2A} {
			if want {
				fmt.Fprintf(os.Stderr, "ipfs mcp: --%s is ignored: it is served on the --http listener only\n", flag)
			}
		}
		return nil
	}
	if wantAGUI {
		if err := srv.EnableAGUI(mcpserver.AGUISettings{OriginAllowlist: s.cfg.Agent.OriginAllowlist, A2UI: s.cfg.Agent.A2UI}); err != nil {
			return fmt.Errorf("agui: %w", err)
		}
		fmt.Fprintf(os.Stderr, "ipfs mcp: AG-UI endpoint enabled at %s (%d browser origins allowed)\n", mcpserver.RouteAGUI, len(s.cfg.Agent.OriginAllowlist))
		if !s.cfg.Agent.A2UIEnabled() {
			fmt.Fprintln(os.Stderr, "ipfs mcp: agent.a2ui is false, so /agui emits no A2UI surface messages")
		}
		if s.resolved == nil {
			fmt.Fprintln(os.Stderr, "ipfs mcp: no model backend is configured (--llm-base-url and --llm-model), so /agui answers 503 backend_not_configured until one is")
		}
	}
	if wantA2A {
		if err := srv.EnableA2A(); err != nil {
			return fmt.Errorf("a2a: %w", err)
		}
		fmt.Fprintf(os.Stderr, "ipfs mcp: A2A endpoint enabled at %s, agent card at %s\n", mcpserver.RouteA2A, mcpserver.RouteAgentCard)
		if !slices.ContainsFunc(s.cfg.Principals, func(p mcpserver.PrincipalConfig) bool { return p.Kind == mcpserver.KindA2A }) {
			fmt.Fprintln(os.Stderr, "ipfs mcp: no principal of kind a2a is configured in --mcp-config, so /a2a will refuse every caller")
		}
	}
	return nil
}
