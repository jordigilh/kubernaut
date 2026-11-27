package launcher_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"google.golang.org/adk/v2/model"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/launcher"
	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// Regression for dependabot PR #2706 (genai v1.77.0 -> v1.79.1): genai >=
// v1.78 changed the default Gemini Developer API version from v1beta to v1.
// Every Kubernaut call site that overrides HTTPOptions.BaseURL to point the
// BackendGeminiAPI client at an operator-configured endpoint (AI gateway,
// mock-llm in E2E) must pin the previous v1beta path explicitly, otherwise
// requests go to /v1/models/{model}:generateContent and 404 against
// endpoints that serve the Gemini-compat surface under /v1beta/models/.
var _ = Describe("Gemini API version pinning (BR-AI-087)", func() {

	var (
		mockServer *httptest.Server
		mu         sync.Mutex
		paths      []string
	)

	BeforeEach(func() {
		paths = nil
		mockServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			paths = append(paths, r.URL.Path)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]}}]}`))
		}))
	})

	AfterEach(func() {
		if mockServer != nil {
			mockServer.Close()
		}
	})

	// IT-AF-2706-001: the model request must hit the v1beta Gemini-compat
	// path even under genai >= v1.78, whose SDK default became v1.
	It("IT-AF-2706-001 NewModelFromConfig with gemini provider requests /v1beta/models/ against a custom endpoint", func() {
		cfg := types.LLMConfig{
			Provider: types.LLMProviderGemini,
			Model:    "gemini-2.0-flash",
			Endpoint: mockServer.URL,
			APIKey:   "test-api-key",
		}

		m, err := launcher.NewModelFromConfig(context.Background(), cfg)
		Expect(err).NotTo(HaveOccurred())
		Expect(m).NotTo(BeNil())

		req := &model.LLMRequest{
			Contents: []*genai.Content{
				{Role: "user", Parts: []*genai.Part{{Text: "Hello"}}},
			},
		}
		for resp, genErr := range m.GenerateContent(context.Background(), req, false) {
			Expect(genErr).NotTo(HaveOccurred())
			Expect(resp).NotTo(BeNil())
		}

		mu.Lock()
		defer mu.Unlock()
		Expect(paths).NotTo(BeEmpty(),
			"gemini model must have sent at least one request to the mock server")
		for _, p := range paths {
			Expect(strings.HasPrefix(p, "/v1beta/models/")).To(BeTrue(),
				"genai SDK default API version changed to v1; the Gemini-compat request path must stay pinned to v1beta, got %q", p)
		}
	})
})
