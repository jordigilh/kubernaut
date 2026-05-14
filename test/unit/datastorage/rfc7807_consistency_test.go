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
// PHASE 8: RFC 7807 CONSISTENCY — BEHAVIORAL (TP-1088-P1)
// ========================================
//
// Issue: #1088 Phase 8 (API Contract)
// File Under Test: pkg/datastorage/server/response/rfc7807.go
//
// BR-STORAGE-024: All error responses MUST use RFC 7807 Problem Details
// with application/problem+json content type and kubernaut.ai/problems/* URIs.
//
// All tests exercise production WriteRFC7807* helpers and assert observable
// HTTP response properties (status, content-type, body fields).
//
// ========================================

var _ = Describe("Phase 8: RFC 7807 Error Consistency (TP-1088-P1)", func() {

	logger := kubelog.NewLogger(kubelog.DefaultOptions())

	Describe("Double-prefix prevention (8.3 - reconstruction_handler)", func() {
		It("UT-DS-1088-P8-001: WriteRFC7807InternalError must produce single domain prefix", func() {
			rr := httptest.NewRecorder()

			errorType := "https://kubernaut.ai/problems/reconstruction/unexpected-error"
			response.WriteRFC7807InternalError(rr, errorType, "Unexpected Error",
				fmt.Errorf("test error"), logger)

			var problem response.RFC7807Problem
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())

			Expect(strings.Count(problem.Type, "https://kubernaut.ai/problems/")).To(Equal(1),
				"RFC 7807 type URI must have exactly one domain prefix, found double prefix")
		})
	})

	Describe("Domain prefix standardization (8.3 - audit_export_handler)", func() {
		It("UT-DS-1088-P8-002: shared WriteRFC7807Error uses kubernaut.ai domain", func() {
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

	Describe("Content-Type header compliance (8.3)", func() {
		It("UT-DS-1088-P8-003: WriteRFC7807Error sets application/problem+json content type", func() {
			rr := httptest.NewRecorder()

			response.WriteRFC7807Error(rr, http.StatusBadRequest,
				"invalid-request", "Invalid Request",
				"Request body is malformed", logger)

			Expect(rr.Header().Get("Content-Type")).To(Equal("application/problem+json"),
				"RFC 7807 responses must have application/problem+json content type")
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("RFC 7807 required fields (8.3)", func() {
		It("UT-DS-1088-P8-004: all error responses must contain type, title, status, detail", func() {
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
