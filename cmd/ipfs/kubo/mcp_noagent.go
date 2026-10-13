//go:build !nomcp && noagent

package kubo

import (
	"errors"

	cmds "github.com/ipfs/go-ipfs-cmds"
	mcpserver "github.com/ipfs/kubo/mcp/server"
)

// agentInputs is empty in builds with the noagent tag: the --llm-* flags do
// not exist.
type agentInputs struct{}

func agentInputsFromRequest(*cmds.Request) agentInputs { return agentInputs{} }

// agentResolved is unused without the agent code.
type agentResolved struct{}

// validateAgent refuses an llm section: this build has no agent code to run
// it, and ignoring it silently would hide a configuration the operator
// expects to take effect.
func validateAgent(s *mcpSettings) error {
	if s.cfg != nil && s.cfg.LLM != nil {
		return errors.New("the config file has an llm section, but this build has no agent support (compiled with the noagent tag)")
	}
	return nil
}

func enableMCPAgent(*mcpserver.Server, *mcpSettings) error { return nil }
