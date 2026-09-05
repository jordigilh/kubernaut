package openaicompat_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

var _ = Describe("OpenAI tool choice recovery (#2365)", func() {
	It("UT-AF-2365-001 (SI-10, AU-3): emits a named tool choice when recovery requires presentation", func() {
		var body map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			Expect(json.NewDecoder(r.Body).Decode(&body)).To(Succeed())
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"call-1","type":"function","function":{"name":"kubernaut_present_decision","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}`))
		}))
		DeferCleanup(server.Close)

		client := openaicompat.New("gpt-4o", server.URL, "")
		_, err := client.Chat(context.Background(), openaicompat.Request{
			Messages:   []openaicompat.Message{{Role: "user", Content: "present the decision"}},
			Tools:      []openaicompat.ToolDefinition{{Name: "kubernaut_present_decision"}},
			ToolChoice: &openaicompat.ToolChoice{Name: "kubernaut_present_decision"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(body["tool_choice"]).To(Equal(map[string]any{
			"type":     "function",
			"function": map[string]any{"name": "kubernaut_present_decision"},
		}))
	})
})
