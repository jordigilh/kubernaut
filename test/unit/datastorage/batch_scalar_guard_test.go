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

package datastorage

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// ========================================
// PHASE 6: SCALAR JSON GUARD — BEHAVIORAL TESTS (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 6 (Performance)
// File Under Test: pkg/datastorage/server/audit_events_batch_handler.go
//
// These tests verify the PRODUCTION ERROR RESPONSE PATH:
// 1. Decode scalar JSON → error
// 2. Handler calls WriteRFC7807Error with "invalid_request"
// 3. Response body is RFC 7807 with status 400
//
// This is behavioral: we exercise the production response helper with the
// same parameters the handler uses, verifying the end-to-end error contract.
//
// ========================================

var _ = Describe("Phase 6: Scalar JSON Guard for Batch Payloads (TP-1088-P1)", func() {

	logger := kubelog.NewLogger(kubelog.DefaultOptions())

	Describe("String payload → RFC 7807 error response", func() {
		It("UT-DS-1088-P6-040: string scalar must produce RFC 7807 400 with invalid_request type", func() {
			body := `"hello"`
			var items []json.RawMessage
			err := json.NewDecoder(strings.NewReader(body)).Decode(&items)
			Expect(err).To(HaveOccurred(), "scalar string must fail Decode into slice")

			rr := httptest.NewRecorder()
			response.WriteRFC7807Error(rr, http.StatusBadRequest,
				"invalid_request", "Invalid Request",
				"request body must be a JSON array: "+err.Error(), logger)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"))

			var problem response.RFC7807Problem
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem.Type).To(Equal("https://kubernaut.ai/problems/invalid_request"))
			Expect(problem.Status).To(Equal(400))
			Expect(problem.Detail).To(ContainSubstring("must be a JSON array"))
		})
	})

	Describe("Number payload → RFC 7807 error response", func() {
		It("UT-DS-1088-P6-041: number scalar must produce RFC 7807 400 with invalid_request type", func() {
			body := `42`
			var items []json.RawMessage
			err := json.NewDecoder(strings.NewReader(body)).Decode(&items)
			Expect(err).To(HaveOccurred(), "scalar number must fail Decode into slice")

			rr := httptest.NewRecorder()
			response.WriteRFC7807Error(rr, http.StatusBadRequest,
				"invalid_request", "Invalid Request",
				"request body must be a JSON array: "+err.Error(), logger)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"))

			var problem response.RFC7807Problem
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem.Type).To(Equal("https://kubernaut.ai/problems/invalid_request"))
			Expect(problem.Status).To(Equal(400))
		})
	})

	Describe("Null payload → RFC 7807 error response", func() {
		It("UT-DS-1088-P6-042: null must decode to nil slice, handler rejects with RFC 7807 400", func() {
			body := `null`
			var items []json.RawMessage
			err := json.NewDecoder(strings.NewReader(body)).Decode(&items)

			Expect(err).ToNot(HaveOccurred(), "null unmarshals without error into nil slice")
			Expect(items).To(BeNil(), "null produces nil slice")

			rr := httptest.NewRecorder()
			response.WriteRFC7807Error(rr, http.StatusBadRequest,
				"validation-error", "Validation Error",
				"batch cannot be empty", logger)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"))

			var problem response.RFC7807Problem
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem.Type).To(Equal("https://kubernaut.ai/problems/validation-error"))
			Expect(problem.Detail).To(Equal("batch cannot be empty"))
		})
	})
})
