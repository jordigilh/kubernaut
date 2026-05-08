package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
