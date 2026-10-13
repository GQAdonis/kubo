//go:build !noagent

package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"slices"
	"strings"

	"github.com/ipfs/kubo/mcp/contract"
)

const (
	a2aProtocolVersion = "1.0"
	// a2aCardMaxAge is how long a client may reuse the card (A2A 1.0 section
	// 8.6). The card changes only when the operator restarts ipfs mcp.
	a2aCardMaxAge = 300
	// maxSkillExamples bounds the examples a skill lists.
	maxSkillExamples = 3
)

// a2aAgentCard is the AgentCard subset ipfs mcp serves.
type a2aAgentCard struct {
	Name                 string                       `json:"name"`
	Description          string                       `json:"description"`
	SupportedInterfaces  []a2aInterface               `json:"supportedInterfaces"`
	Version              string                       `json:"version"`
	Capabilities         a2aCapabilities              `json:"capabilities"`
	SecuritySchemes      map[string]a2aSecurityScheme `json:"securitySchemes"`
	SecurityRequirements []a2aSecurityRequirement     `json:"securityRequirements"`
	DefaultInputModes    []string                     `json:"defaultInputModes"`
	DefaultOutputModes   []string                     `json:"defaultOutputModes"`
	Skills               []a2aSkill                   `json:"skills"`
}

type a2aInterface struct {
	URL             string `json:"url"`
	ProtocolBinding string `json:"protocolBinding"`
	ProtocolVersion string `json:"protocolVersion"`
}

type a2aCapabilities struct {
	Streaming         bool `json:"streaming"`
	PushNotifications bool `json:"pushNotifications"`
}

type a2aSecurityScheme struct {
	HTTPAuth *a2aHTTPAuth `json:"httpAuthSecurityScheme,omitempty"`
}

type a2aHTTPAuth struct {
	Scheme      string `json:"scheme"`
	Description string `json:"description,omitempty"`
}

type a2aSecurityRequirement struct {
	Schemes map[string]a2aStringList `json:"schemes"`
}

type a2aStringList struct {
	List []string `json:"list"`
}

type a2aSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
	InputModes  []string `json:"inputModes,omitempty"`
	OutputModes []string `json:"outputModes,omitempty"`
}

// skillSource is one entry of the contract's agentic/a2a-skills.json.
type skillSource struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Groups      []string `json:"groups"`
	Tags        []string `json:"tags"`
	InputModes  []string `json:"inputModes"`
	OutputModes []string `json:"outputModes"`
}

func loadSkillSources() ([]skillSource, error) {
	raw, err := fs.ReadFile(contract.FS(), "agentic/a2a-skills.json")
	if err != nil {
		return nil, fmt.Errorf("read a2a skills: %w", err)
	}
	var doc struct {
		Skills []skillSource `json:"skills"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse a2a skills: %w", err)
	}
	return doc.Skills, nil
}

// cardSkills derives the skills of the card from the contract: a skill is
// listed when this server offers at least one of its tools and at least one of
// the A2A principals may call it. The card is public, so it never depends on
// who asks; it shows what the configured A2A principals can use at most.
func (s *Server) cardSkills(peers []Identity) ([]a2aSkill, error) {
	sources, err := loadSkillSources()
	if err != nil {
		return nil, err
	}
	skills := make([]a2aSkill, 0, len(sources))
	for _, src := range sources {
		var examples []string
		offered := false
		for _, name := range s.registered {
			t := s.registry[name].tool
			if t.A2A.Skill != src.ID || !slices.ContainsFunc(peers, func(id Identity) bool { return id.Allows(t) }) {
				continue
			}
			offered = true
			if t.A2A.Example != "" && len(examples) < maxSkillExamples && !slices.Contains(examples, t.A2A.Example) {
				examples = append(examples, t.A2A.Example)
			}
		}
		if !offered {
			continue
		}
		skills = append(skills, a2aSkill{
			ID: src.ID, Name: src.Name, Description: src.Description, Tags: src.Tags,
			Examples: examples, InputModes: src.InputModes, OutputModes: src.OutputModes,
		})
	}
	return skills, nil
}

// buildCard assembles the agent card for the interface URL.
func (s *Server) buildCard(url string, peers []Identity) (*a2aAgentCard, error) {
	skills, err := s.cardSkills(peers)
	if err != nil {
		return nil, err
	}
	version := s.opts.Version
	if version == "" {
		version = "dev"
	}
	return &a2aAgentCard{
		Name: "ipfs mcp",
		Description: "IPFS agent for the connected Kubo node. Writes and destructive changes wait for a person to approve " +
			"them on the node's machine; no A2A message can approve them.",
		SupportedInterfaces: []a2aInterface{{URL: url, ProtocolBinding: "JSONRPC", ProtocolVersion: a2aProtocolVersion}},
		Version:             version,
		Capabilities:        a2aCapabilities{Streaming: true, PushNotifications: false},
		SecuritySchemes: map[string]a2aSecurityScheme{
			"bearer": {HTTPAuth: &a2aHTTPAuth{Scheme: "Bearer", Description: "A bearer token of an A2A principal of this ipfs mcp."}},
		},
		SecurityRequirements: []a2aSecurityRequirement{{Schemes: map[string]a2aStringList{"bearer": {List: []string{}}}}},
		DefaultInputModes:    []string{"text/plain", "application/json"},
		DefaultOutputModes:   []string{"text/plain", "application/json"},
		Skills:               skills,
	}, nil
}

// cardHandler serves GET /.well-known/agent-card.json. The route is public
// (A2A 1.0 section 8.1) and carries metadata only.
type cardHandler struct {
	srv   *Server
	peers []Identity
}

func (h *cardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != RouteAgentCard {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// The interface URL is the address the caller reached us on. Host was
	// checked by the listener (loopback listeners accept loopback hosts only).
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	card, err := h.srv.buildCard(scheme+"://"+r.Host+RouteA2A, h.peers)
	if err != nil {
		h.srv.log.Warn("agent card failed", "error_type", fmt.Sprintf("%T", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	body, err := json.Marshal(card)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	hd := w.Header()
	hd.Set("Content-Type", "application/json")
	// The interface URL comes from the Host header, so a shared cache must not
	// serve one host's card to a request for another.
	hd.Set("Cache-Control", fmt.Sprintf("private, max-age=%d", a2aCardMaxAge))
	hd.Add("Vary", "Host")
	hd.Set("ETag", etag)
	if strings.Contains(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}
