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

package anthropicfamily_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm"
	"github.com/jordigilh/kubernaut/pkg/kubernautagent/llm/anthropicfamily"
)

// Issue #1585: Chat/StreamChat must classify the Anthropic SDK's typed
// *anthropic.Error (StatusCode field) at this package's translation
// boundary and mark permanent 400/401/403/404-class failures non-retryable
// via llm.MarkNonRetryable, so ChatWithParams and the streaming retry path
// (#1612) both fail fast on them instead of consuming the retry budget.
var _ = Describe("anthropicfamily error classification — #1585", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	newFailingClient := func(status int) llm.Client {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"boom"}}`))
		}))
		client, err := anthropicfamily.NewWithAPIKey("sk-ant-fake-key", "claude-sonnet-4-6",
			anthropicfamily.WithSDKOptions(option.WithBaseURL(server.URL), option.WithMaxRetries(0)),
		)
		Expect(err).NotTo(HaveOccurred())
		return client
	}

	DescribeTable("UT-KA-1585-006: Chat() classification by HTTP status",
		func(status int, wantRetryable bool) {
			client := newFailingClient(status)
			_, err := client.Chat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hi"}},
			})
			Expect(err).To(HaveOccurred())
			Expect(llm.IsRetryable(err)).To(Equal(wantRetryable))
		},
		Entry("401 Unauthorized is non-retryable", http.StatusUnauthorized, false),
		Entry("404 Not Found is non-retryable", http.StatusNotFound, false),
		Entry("429 Too Many Requests is retryable", http.StatusTooManyRequests, true),
		Entry("500 Internal Server Error is retryable", http.StatusInternalServerError, true),
	)

	Describe("UT-KA-1585-007: StreamChat() classification by HTTP status", func() {
		It("marks a 403 Forbidden stream error non-retryable", func() {
			client := newFailingClient(http.StatusForbidden)
			_, err := client.StreamChat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hi"}},
			}, func(llm.ChatStreamEvent) error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(llm.IsRetryable(err)).To(BeFalse())
		})

		It("leaves a 529 Overloaded stream error retryable", func() {
			client := newFailingClient(529)
			_, err := client.StreamChat(context.Background(), llm.ChatRequest{
				Messages: []llm.Message{{Role: "user", Content: "hi"}},
			}, func(llm.ChatStreamEvent) error { return nil })
			Expect(err).To(HaveOccurred())
			Expect(llm.IsRetryable(err)).To(BeTrue())
		})
	})
})
