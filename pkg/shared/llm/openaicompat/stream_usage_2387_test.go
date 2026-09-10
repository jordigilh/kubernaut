/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package openaicompat_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

// Issue #2387 / BR-KA-OBSERVABILITY-001: E2E investigations stream, and the
// shared client silently dropped provider-sent usage on streams, leaving
// Final.Usage zero. When the provider sends usage (OpenAI convention: a
// trailing chunk with a usage object and empty choices, emitted when
// stream_options.include_usage is set), the terminal Response must carry it.
var _ = Describe("openaicompat streaming usage — #2387", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	It("UT-KA-2387-100: surfaces a trailing usage chunk on the terminal streamed Response", func() {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			chunks := []string{
				`{"choices":[{"index":0,"delta":{"content":"Hel"}}]}`,
				`{"choices":[{"index":0,"delta":{"content":"lo"}}]}`,
				`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
				`{"usage":{"prompt_tokens":12,"completion_tokens":7,"total_tokens":19},"choices":[]}`,
			}
			for _, c := range chunks {
				_, _ = w.Write([]byte("data: " + c + "\n\n"))
				if flusher != nil {
					flusher.Flush()
				}
			}
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}))
		client := openaicompat.New("gpt-4o", server.URL, "test-key")

		var final *openaicompat.Response
		err := client.StreamChat(context.Background(), openaicompat.Request{
			Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
		}, func(ev openaicompat.StreamEvent) bool {
			if ev.Done {
				final = ev.Final
			}
			return true
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(final).NotTo(BeNil())
		Expect(final.Usage.PromptTokens).To(Equal(12))
		Expect(final.Usage.CompletionTokens).To(Equal(7))
		Expect(final.Usage.TotalTokens).To(Equal(19))
	})
})
