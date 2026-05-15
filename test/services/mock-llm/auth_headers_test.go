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
package mockllm_test

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/services/mock-llm/tracker"
)

var _ = Describe("Auth Header Recorder", func() {

	Describe("UT-MOCK-006-001: Recorder captures Authorization and custom X- headers", func() {
		It("should record Authorization header from HTTP request", func() {
			h := tracker.NewHeaderRecorder("authorization,x-custom-token")
			req, _ := http.NewRequest("POST", "/test", nil)
			req.Header.Set("Authorization", "Bearer test-token-123")
			req.Header.Set("X-Custom-Token", "custom-value")
			req.Header.Set("Content-Type", "application/json")

			h.RecordFrom(req)
			recorded := h.GetRecordedHeaders()

			Expect(recorded).To(HaveLen(2))
			Expect(recorded["Authorization"]).To(Equal("Bearer test-token-123"))
			Expect(recorded["X-Custom-Token"]).To(Equal("custom-value"))
		})
	})

	Describe("UT-MOCK-006-002: Recorder ignores unconfigured headers", func() {
		It("should not record headers not in the configured list", func() {
			h := tracker.NewHeaderRecorder("authorization")
			req, _ := http.NewRequest("POST", "/test", nil)
			req.Header.Set("Authorization", "Bearer tok")
			req.Header.Set("X-Untracked", "ignored")

			h.RecordFrom(req)
			recorded := h.GetRecordedHeaders()
			Expect(recorded).To(HaveKey("Authorization"))
			Expect(recorded).NotTo(HaveKey("X-Untracked"))
		})
	})

	Describe("UT-MOCK-006-003: Recorder supports reset", func() {
		It("should clear recorded headers after reset", func() {
			h := tracker.NewHeaderRecorder("authorization")
			req, _ := http.NewRequest("POST", "/test", nil)
			req.Header.Set("Authorization", "Bearer tok")
			h.RecordFrom(req)

			h.Reset()
			Expect(h.GetRecordedHeaders()).To(BeEmpty())
		})
	})
})
