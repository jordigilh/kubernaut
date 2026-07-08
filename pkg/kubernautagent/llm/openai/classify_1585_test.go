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

package openai_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	kaopenai "github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/openai"
)

// Issue #1585: Chat/StreamChat must classify the shared openaicompat
// client's typed *openaicompat.APIError (StatusCode field) at this
// package's translation boundary and mark permanent 400/401/403/404-class
// failures non-retryable via llm.MarkNonRetryable, so ChatWithParams and
// the streaming retry path (#1612) both fail fast on them instead of
// consuming the retry budget.
var _ = Describe("openai adapter error classification — #1585", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	newFailingClient := func(status int) llm.Client {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte("boom"))
		}))
		return kaopenai.New("gpt-4o", server.URL, "test-key")
	}

	DescribeTable("UT-KA-1585-007: Chat() classification by HTTP status",
		func(status int, wantRetryable bool) {
			client := newFailingClient(status)
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hi"}},
			})
			Expect(err).To(HaveOccurred())
			Expect(llm.IsRetryable(err)).To(Equal(wantRetryable))
		},
		Entry("400 Bad Request is non-retryable", http.StatusBadRequest, false),
		Entry("404 Not Found is non-retryable", http.StatusNotFound, false),
		Entry("429 Too Many Requests is retryable", http.StatusTooManyRequests, true),
		Entry("502 Bad Gateway is retryable", http.StatusBadGateway, true),
	)

	Describe("StreamChat() classification by HTTP status", func() {
		It("marks a 401 Unauthorized stream error non-retryable", func() {
			client := newFailingClient(http.StatusUnauthorized)
			_, err := client.StreamChat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hi"}},
			}, func(llm.ChatStreamEvent) error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(llm.IsRetryable(err)).To(BeFalse())
		})

		It("leaves a 503 Service Unavailable stream error retryable", func() {
			client := newFailingClient(http.StatusServiceUnavailable)
			_, err := client.StreamChat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hi"}},
			}, func(llm.ChatStreamEvent) error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(llm.IsRetryable(err)).To(BeTrue())
		})
	})
})
