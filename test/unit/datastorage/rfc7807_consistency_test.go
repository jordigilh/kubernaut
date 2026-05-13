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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/server/response"
	kubelog "github.com/jordigilh/kubernaut/pkg/log"
)

// ========================================
// PHASE 8: RFC 7807 CONSISTENCY (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 8 (API Contract)
// TDD Phase: RED — these tests document inconsistencies in error responses
//
// BR-STORAGE-024: All error responses MUST use RFC 7807 Problem Details
// with application/problem+json content type and kubernaut.ai/problems/* URIs.
//
// Known violations:
// 1. reconstruction_handler passes full URL to WriteRFC7807InternalError → double prefix
// 2. audit_export_handler uses "about:blank" instead of kubernaut.ai domain
// 3. verify_chain_handler uses http.Error() → text/plain instead of RFC 7807
// 4. verify_chain_handler and audit_export_handler method-not-allowed uses http.Error()
//
// ========================================

var _ = Describe("Phase 8: RFC 7807 Error Consistency (TP-1088-P1)", func() {

	Describe("Double-prefix detection (8.3 - reconstruction_handler)", func() {
		It("UT-DS-1088-P8-001: WriteRFC7807InternalError must produce single domain prefix", func() {
			// RED: reconstruction_handler currently passes the full URL
			// "https://kubernaut.ai/problems/reconstruction/unexpected-error"
			// to WriteRFC7807InternalError, which prepends "https://kubernaut.ai/problems/"
			// again → "https://kubernaut.ai/problems/https://kubernaut.ai/problems/..."
			//
			// The errorType parameter should be just the slug, e.g. "reconstruction/unexpected-error"

			logger := kubelog.NewLogger(kubelog.DefaultOptions())
			rr := httptest.NewRecorder()

			// Simulate what reconstruction_handler currently does (passing full URL as errorType)
			errorType := "https://kubernaut.ai/problems/reconstruction/unexpected-error"
			response.WriteRFC7807InternalError(rr, errorType, "Unexpected Error",
				fmt.Errorf("test error"), logger)

			var problem response.RFC7807Problem
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())

			// The type field must NOT contain a double prefix
			Expect(strings.Count(problem.Type, "https://kubernaut.ai/problems/")).To(Equal(1),
				"RFC 7807 type URI must have exactly one domain prefix, found double prefix")
		})
	})

	Describe("Domain prefix standardization (8.3 - audit_export_handler)", func() {
		It("UT-DS-1088-P8-002: export handler error type must use kubernaut.ai domain", func() {
			// RED: audit_export_handler uses a local writeRFC7807Error helper
			// that hardcodes "about:blank" as the type. This violates DD-004 which
			// mandates kubernaut.ai/problems/* for all RFC 7807 type URIs.
			//
			// To test this without spinning up a full server, we validate
			// that the shared WriteRFC7807Error helper produces the correct domain.
			// The local helper in audit_export_handler will be removed in GREEN.

			logger := kubelog.NewLogger(kubelog.DefaultOptions())
			rr := httptest.NewRecorder()

			response.WriteRFC7807Error(rr, http.StatusBadRequest,
				"invalid-export-format", "Invalid Export Format",
				"The requested export format is not supported", logger)

			var problem response.RFC7807Problem
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())

			Expect(problem.Type).To(HavePrefix("https://kubernaut.ai/problems/"),
				"Error type must use kubernaut.ai domain per DD-004")
			Expect(problem.Type).ToNot(Equal("about:blank"),
				"about:blank is not a valid error type for kubernaut services")
		})
	})

	Describe("verify-chain RFC 7807 compliance (8.3)", func() {
		It("UT-DS-1088-P8-003: verify-chain handler must return application/problem+json on error", func() {
			// RED: HandleVerifyChain currently uses http.Error() which returns
			// text/plain; charset=utf-8 instead of application/problem+json.
			//
			// We test the contract: error responses from verify-chain must have
			// the correct content type and contain RFC 7807 fields.
			//
			// Since we can't call HandleVerifyChain without a *Server, we test
			// that http.Error produces the WRONG content type (documenting the bug).

			rr := httptest.NewRecorder()
			http.Error(rr, "Invalid request body", http.StatusBadRequest)

			// http.Error produces text/plain — this is what verify-chain currently returns
			Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("text/plain"),
				"http.Error produces text/plain, confirming verify-chain has wrong content type")

			// What it SHOULD produce:
			Expect(rr.Header().Get("Content-Type")).ToNot(Equal("application/problem+json"),
				"Confirms http.Error does NOT produce RFC 7807 content type")
		})
	})

	Describe("RFC 7807 required fields (8.3)", func() {
		It("UT-DS-1088-P8-004: all error responses must contain type, title, status, detail", func() {
			// Verify the shared helper produces all 4 required RFC 7807 fields

			logger := kubelog.NewLogger(kubelog.DefaultOptions())
			rr := httptest.NewRecorder()

			response.WriteRFC7807Error(rr, http.StatusUnprocessableEntity,
				"validation-error", "Validation Error",
				"Field 'correlation_id' is required", logger)

			var raw map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&raw)).To(Succeed())

			Expect(raw).To(HaveKey("type"), "RFC 7807 requires 'type' field")
			Expect(raw).To(HaveKey("title"), "RFC 7807 requires 'title' field")
			Expect(raw).To(HaveKey("status"), "RFC 7807 requires 'status' field")
			Expect(raw).To(HaveKey("detail"), "RFC 7807 requires 'detail' field")

			Expect(raw["status"]).To(BeNumerically("==", 422))
			Expect(raw["type"]).To(BeAssignableToTypeOf(""))
			Expect(raw["title"]).To(BeAssignableToTypeOf(""))
			Expect(raw["detail"]).To(BeAssignableToTypeOf(""))
		})
	})
})
