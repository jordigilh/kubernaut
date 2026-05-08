package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"

	"github.com/a2aproject/a2a-go/a2a"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
)

// AgentSkill represents a skill in the agent card.
type AgentSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// GroupMapping maps OIDC group names to AF role keys defined in rbac_roles.yaml.
type GroupMapping map[string]string

// RBACRoles maps role keys to their allowed tool names.
type RBACRoles map[string][]string

// AgentCardConfig holds configuration for the agent card handler.
type AgentCardConfig struct {
	Name        string
	Description string
	URL         string
	Version     string
	Skills      []AgentSkill

	// RBAC filtering (optional). When set, enables per-request skill filtering.
	RBACRoles    RBACRoles
	GroupMapping GroupMapping
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
// When RBACRoles and GroupMapping are configured, the handler returns:
//   - Unauthenticated requests: shell card (metadata only, empty skills)
//   - Authenticated requests: persona-filtered skills based on JWT groups
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

	skillIndex := make(map[string]AgentSkill, len(skills))
	for _, s := range skills {
		skillIndex[s.ID] = s
	}

	baseCard := agentCard{
		Name:            cfg.Name,
		Description:     cfg.Description,
		URL:             cfg.URL,
		Version:         cfg.Version,
		ProtocolVersion: string(a2a.Version),
		Skills:          skills,
		Authentication: agentAuth{
			Schemes: []authScheme{{Scheme: "bearer"}},
		},
		Capabilities: agentCaps{
			Streaming:    false,
			PushNotify:   false,
			StateTransit: true,
		},
		Provider: agentProvider{
			Organization: "kubernaut.ai",
		},
	}

	rbacEnabled := len(cfg.RBACRoles) > 0

	if !rbacEnabled {
		cardBytes, err := json.Marshal(baseCard)
		if err != nil {
			return nil, fmt.Errorf("marshal agent card: %w", err)
		}
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cardBytes)
		}), nil
	}

	shellCard := baseCard
	shellCard.Skills = []AgentSkill{}
	shellBytes, err := json.Marshal(shellCard)
	if err != nil {
		return nil, fmt.Errorf("marshal shell agent card: %w", err)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		identity := auth.UserIdentityFromContext(r.Context())
		if identity == nil {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(shellBytes)
			return
		}

		allowed := resolveAllowedTools(identity.Groups, cfg.GroupMapping, cfg.RBACRoles)
		filtered := filterSkills(skillIndex, allowed)

		card := baseCard
		card.Skills = filtered

		cardBytes, marshalErr := json.Marshal(card)
		if marshalErr != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(cardBytes)
	}), nil
}

// resolveAllowedTools maps JWT groups through groupMapping to role keys,
// then collects the union of allowed tools from rbacRoles.
func resolveAllowedTools(groups []string, mapping GroupMapping, roles RBACRoles) map[string]bool {
	allowed := make(map[string]bool)
	for _, group := range groups {
		roleKey := group
		if mapping != nil {
			if mapped, ok := mapping[group]; ok {
				roleKey = mapped
			}
		}
		if tools, ok := roles[roleKey]; ok {
			for _, t := range tools {
				allowed[t] = true
			}
		}
	}
	return allowed
}

// filterSkills returns only the skills whose ID is in the allowed set, sorted by ID.
func filterSkills(index map[string]AgentSkill, allowed map[string]bool) []AgentSkill {
	result := make([]AgentSkill, 0, len(allowed))
	for id := range allowed {
		if skill, ok := index[id]; ok {
			result = append(result, skill)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
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
