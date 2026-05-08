package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut-apifrontend/internal/auth"
	"github.com/jordigilh/kubernaut-apifrontend/internal/handler"
)

var _ = Describe("Agent Card Handler", func() {
	It("UT-AF-230-001: NewAgentCardHandler returns non-nil handler", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:        "kubernaut-apifrontend",
			Description: "Kubernaut API Frontend agent for incident triage",
			URL:         "https://kubernaut.example.com",
			Version:     "0.1.0",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(h).NotTo(BeNil())
	})

	It("UT-AF-230-002: returns error when Name is empty", func() {
		_, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:    "",
			URL:     "https://example.com",
			Version: "0.1.0",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("name"))
	})

	It("UT-AF-230-003: returns error when URL is empty", func() {
		_, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:    "test",
			URL:     "",
			Version: "0.1.0",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("URL"))
	})

	It("UT-AF-230-004: serves valid JSON with correct Content-Type", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:        "kubernaut-apifrontend",
			Description: "Test agent",
			URL:         "https://kubernaut.example.com",
			Version:     "0.1.0",
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Header().Get("Content-Type")).To(Equal("application/json"))

		var card map[string]any
		err = json.Unmarshal(rec.Body.Bytes(), &card)
		Expect(err).NotTo(HaveOccurred())
	})

	It("UT-AF-230-005: card includes name and description", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:        "kubernaut-apifrontend",
			Description: "Kubernaut API Frontend agent for incident triage",
			URL:         "https://kubernaut.example.com",
			Version:     "0.1.0",
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var card map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &card)
		Expect(card["name"]).To(Equal("kubernaut-apifrontend"))
		Expect(card["description"]).To(Equal("Kubernaut API Frontend agent for incident triage"))
	})

	It("UT-AF-230-006: card includes version", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:    "kubernaut-apifrontend",
			URL:     "https://kubernaut.example.com",
			Version: "0.2.0",
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var card map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &card)
		Expect(card["version"]).To(Equal("0.2.0"))
	})

	It("UT-AF-230-007: card includes skills matching 14 tools", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:    "kubernaut-apifrontend",
			URL:     "https://kubernaut.example.com",
			Version: "0.1.0",
			Skills:  handler.DefaultAgentSkills(),
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var card map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &card)
		skills, ok := card["skills"].([]any)
		Expect(ok).To(BeTrue())
		Expect(skills).To(HaveLen(14))
	})

	It("UT-AF-230-008: card declares authentication requirements", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:    "kubernaut-apifrontend",
			URL:     "https://kubernaut.example.com",
			Version: "0.1.0",
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var card map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &card)
		authInfo, ok := card["authentication"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(authInfo["schemes"]).NotTo(BeNil())
	})

	It("UT-AF-230-009: card includes url field", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:    "kubernaut-apifrontend",
			URL:     "https://kubernaut.example.com",
			Version: "0.1.0",
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var card map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &card)
		Expect(card["url"]).To(Equal("https://kubernaut.example.com"))
	})

	It("UT-AF-230-010: card includes capabilities", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:    "kubernaut-apifrontend",
			URL:     "https://kubernaut.example.com",
			Version: "0.1.0",
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var card map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &card)
		capabilities, ok := card["capabilities"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(capabilities["streaming"]).To(BeFalse())
	})

	It("UT-AF-230-011: card includes protocolVersion", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:    "kubernaut-apifrontend",
			URL:     "https://kubernaut.example.com",
			Version: "0.1.0",
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var card map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &card)
		Expect(card["protocolVersion"]).To(Equal("0.3.0"))
	})
})

var _ = Describe("Agent Card RBAC Filtering", func() {
	var rbacRoles handler.RBACRoles
	var groupMapping handler.GroupMapping
	var allSkills []handler.AgentSkill

	BeforeEach(func() {
		rbacRoles = handler.RBACRoles{
			"sre":      {"tool_a", "tool_b", "tool_c"},
			"cicd":     {"tool_a", "tool_d"},
			"l3-audit": {"tool_b", "tool_e"},
		}
		groupMapping = handler.GroupMapping{
			"platform-sre-team": "sre",
			"github-actions":    "cicd",
			"audit-team":        "l3-audit",
		}
		allSkills = []handler.AgentSkill{
			{ID: "tool_a", Name: "Tool A", Description: "Does A"},
			{ID: "tool_b", Name: "Tool B", Description: "Does B"},
			{ID: "tool_c", Name: "Tool C", Description: "Does C"},
			{ID: "tool_d", Name: "Tool D", Description: "Does D"},
			{ID: "tool_e", Name: "Tool E", Description: "Does E"},
		}
	})

	It("UT-AF-083-001: unauthenticated request returns shell card with empty skills", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:         "kubernaut-apifrontend",
			URL:          "https://kubernaut.example.com",
			Version:      "0.1.0",
			Skills:       allSkills,
			RBACRoles:    rbacRoles,
			GroupMapping: groupMapping,
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		var card map[string]any
		err = json.Unmarshal(rec.Body.Bytes(), &card)
		Expect(err).NotTo(HaveOccurred())
		Expect(card["name"]).To(Equal("kubernaut-apifrontend"))
		skills, ok := card["skills"].([]any)
		Expect(ok).To(BeTrue())
		Expect(skills).To(BeEmpty())
	})

	It("UT-AF-083-002: authenticated SRE sees only SRE tools", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:         "kubernaut-apifrontend",
			URL:          "https://kubernaut.example.com",
			Version:      "0.1.0",
			Skills:       allSkills,
			RBACRoles:    rbacRoles,
			GroupMapping: groupMapping,
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
			Username: "sre-user",
			Groups:   []string{"platform-sre-team"},
			Issuer:   "test",
		})
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		Expect(rec.Code).To(Equal(http.StatusOK))
		var card map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &card)
		skills, ok := card["skills"].([]any)
		Expect(ok).To(BeTrue())
		Expect(skills).To(HaveLen(3))

		ids := extractSkillIDs(skills)
		Expect(ids).To(ContainElement("tool_a"))
		Expect(ids).To(ContainElement("tool_b"))
		Expect(ids).To(ContainElement("tool_c"))
	})

	It("UT-AF-083-003: multiple groups returns union of tools (deduplicated)", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:         "kubernaut-apifrontend",
			URL:          "https://kubernaut.example.com",
			Version:      "0.1.0",
			Skills:       allSkills,
			RBACRoles:    rbacRoles,
			GroupMapping: groupMapping,
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
			Username: "multi-role-user",
			Groups:   []string{"platform-sre-team", "audit-team"},
			Issuer:   "test",
		})
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var card map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &card)
		skills, ok := card["skills"].([]any)
		Expect(ok).To(BeTrue())
		// sre: tool_a, tool_b, tool_c; l3-audit: tool_b, tool_e => union = tool_a, tool_b, tool_c, tool_e
		Expect(skills).To(HaveLen(4))
	})

	It("UT-AF-083-004: unmapped group with direct role key match still works", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:         "kubernaut-apifrontend",
			URL:          "https://kubernaut.example.com",
			Version:      "0.1.0",
			Skills:       allSkills,
			RBACRoles:    rbacRoles,
			GroupMapping: groupMapping,
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
			Username: "direct-role-user",
			Groups:   []string{"cicd"},
			Issuer:   "test",
		})
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var card map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &card)
		skills, ok := card["skills"].([]any)
		Expect(ok).To(BeTrue())
		Expect(skills).To(HaveLen(2))

		ids := extractSkillIDs(skills)
		Expect(ids).To(ContainElement("tool_a"))
		Expect(ids).To(ContainElement("tool_d"))
	})

	It("UT-AF-083-005: authenticated user with no matching role gets empty skills", func() {
		h, err := handler.NewAgentCardHandler(handler.AgentCardConfig{
			Name:         "kubernaut-apifrontend",
			URL:          "https://kubernaut.example.com",
			Version:      "0.1.0",
			Skills:       allSkills,
			RBACRoles:    rbacRoles,
			GroupMapping: groupMapping,
		})
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest("GET", "/.well-known/agent-card.json", http.NoBody)
		ctx := auth.WithUserIdentity(req.Context(), &auth.UserIdentity{
			Username: "unknown-role-user",
			Groups:   []string{"marketing-team"},
			Issuer:   "test",
		})
		req = req.WithContext(ctx)

		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		var card map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &card)
		skills, ok := card["skills"].([]any)
		Expect(ok).To(BeTrue())
		Expect(skills).To(BeEmpty())
	})
})

func extractSkillIDs(skills []any) []string {
	ids := make([]string, 0, len(skills))
	for _, s := range skills {
		skill, ok := s.(map[string]any)
		if ok {
			if id, idOK := skill["id"].(string); idOK {
				ids = append(ids, id)
			}
		}
	}
	return ids
}
