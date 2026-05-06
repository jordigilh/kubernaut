package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// AgentSkill represents a skill in the agent card.
type AgentSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// AgentCardConfig holds configuration for the agent card handler.
type AgentCardConfig struct {
	Name        string
	Description string
	URL         string
	Version     string
	Skills      []AgentSkill
}

//nolint:gocritic // hugeParam: value copy intentional for validation
func (c AgentCardConfig) validate() error {
	if c.Name == "" {
		return fmt.Errorf("name is required")
	}
	if c.URL == "" {
		return fmt.Errorf("URL is required")
	}
	return nil
}

// agentCard is the JSON structure served at /.well-known/agent-card.json.
type agentCard struct {
	Name            string        `json:"name"`
	Description     string        `json:"description,omitempty"`
	URL             string        `json:"url"`
	Version         string        `json:"version"`
	ProtocolVersion string        `json:"protocolVersion"`
	Skills          []AgentSkill  `json:"skills"`
	Authentication  agentAuth     `json:"authentication"`
	Capabilities    agentCaps     `json:"capabilities"`
	Provider        agentProvider `json:"provider"`
}

type agentAuth struct {
	Schemes []authScheme `json:"schemes"`
}

type authScheme struct {
	Scheme string `json:"scheme"`
}

type agentCaps struct {
	Streaming    bool `json:"streaming"`
	PushNotify   bool `json:"pushNotifications"`
	StateTransit bool `json:"stateTransitionHistory"`
}

type agentProvider struct {
	Organization string `json:"organization"`
}

// NewAgentCardHandler creates an http.Handler that serves the agent card JSON
// at /.well-known/agent-card.json per the A2A spec.
//
//nolint:gocritic // hugeParam: value copy intentional; function is called once at startup
func NewAgentCardHandler(cfg AgentCardConfig) (http.Handler, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid agent card config: %w", err)
	}

	skills := cfg.Skills
	if skills == nil {
		skills = []AgentSkill{}
	}

	card := agentCard{
		Name:            cfg.Name,
		Description:     cfg.Description,
		URL:             cfg.URL,
		Version:         cfg.Version,
		ProtocolVersion: "0.3.0",
		Skills:          skills,
		Authentication: agentAuth{
			Schemes: []authScheme{{Scheme: "bearer"}},
		},
		Capabilities: agentCaps{
			Streaming:    true,
			PushNotify:   false,
			StateTransit: true,
		},
		Provider: agentProvider{
			Organization: "kubernaut.ai",
		},
	}

	cardBytes, err := json.Marshal(card)
	if err != nil {
		return nil, fmt.Errorf("marshal agent card: %w", err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(cardBytes)
	}), nil
}

// DefaultAgentSkills returns the 14 agent skills corresponding to the tools.
func DefaultAgentSkills() []AgentSkill {
	skills := make([]AgentSkill, len(mcpToolRegistry))
	for i, t := range mcpToolRegistry {
		skills[i] = AgentSkill{
			ID:          t.Name,
			Name:        t.Name,
			Description: t.Description,
		}
	}
	return skills
}
