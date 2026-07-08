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
	"errors"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/shared/llm/openaicompat"
)

// Issue #1585: ChatWithParams (and, via #1612, the new streaming retry path)
// need a typed, structured error exposing the HTTP status code from a
// non-2xx response so callers can classify retryable vs. non-retryable
// failures. Prior to this change, Client.do() returned a bare fmt.Errorf
// string with no programmatically-accessible status code.
var _ = Describe("openaicompat.APIError — #1585", func() {
	var server *httptest.Server

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
	})

	newFailingClient := func(status int, body string) *openaicompat.Client {
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(body))
		}))
		return openaicompat.New("gpt-4o", server.URL, "test-key")
	}

	Describe("UT-COMPAT-1585-001: non-200 response yields a typed *APIError", func() {
		It("exposes StatusCode and Body via errors.As", func() {
			client := newFailingClient(http.StatusUnauthorized, `{"error":"invalid api key"}`)

			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
			})

			Expect(err).To(HaveOccurred())
			var apiErr *openaicompat.APIError
			Expect(errors.As(err, &apiErr)).To(BeTrue(), "error must unwrap to *openaicompat.APIError")
			Expect(apiErr.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(apiErr.Body).To(ContainSubstring("invalid api key"))
		})

		It("preserves the existing error string format for log/debug compatibility", func() {
			client := newFailingClient(http.StatusNotFound, "model not found")

			_, err := client.Chat(context.Background(), openaicompat.Request{
				Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
			})

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(Equal("openaicompat: API error (HTTP 404): model not found"))
		})

		It("also applies to the streaming path", func() {
			client := newFailingClient(http.StatusBadRequest, "bad request")

			err := client.StreamChat(context.Background(), openaicompat.Request{
				Messages: []openaicompat.Message{{Role: "user", Content: "hi"}},
			}, func(openaicompat.StreamEvent) bool { return true })

			Expect(err).To(HaveOccurred())
			var apiErr *openaicompat.APIError
			Expect(errors.As(err, &apiErr)).To(BeTrue())
			Expect(apiErr.StatusCode).To(Equal(http.StatusBadRequest))
		})
	})
})
