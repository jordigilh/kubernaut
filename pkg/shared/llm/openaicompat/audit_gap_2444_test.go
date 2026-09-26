package openaicompat_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

var _ = Describe("OpenAI-compatible stream audit failures (#2444)", func() {
	It("UT-SHARED-2444-001 rejects malformed SSE JSON", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {not-json}\n\n"))
		}))
		defer server.Close()

		client := openaicompat.New("gpt-4o", server.URL, "")
		err := client.StreamChat(context.Background(), openaicompat.Request{}, func(openaicompat.StreamEvent) bool {
			return true
		})

		Expect(err).To(MatchError(ContainSubstring("decode SSE chunk")))
	})

	It("UT-SHARED-2444-002 rejects provider EOF before [DONE]", func() {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n"))
		}))
		defer server.Close()

		client := openaicompat.New("gpt-4o", server.URL, "")
		err := client.StreamChat(context.Background(), openaicompat.Request{}, func(openaicompat.StreamEvent) bool {
			return true
		})

		Expect(err).To(MatchError(ContainSubstring("before [DONE]")))
	})
})
