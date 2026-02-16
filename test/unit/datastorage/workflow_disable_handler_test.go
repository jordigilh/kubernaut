/*
Copyright 2025 Jordi Gil.

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
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-chi/chi/v5"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// WORKFLOW DISABLE HANDLER UNIT TESTS
// ========================================
// GAP-WF-5: DD-WORKFLOW-017 Phase 4.4 - reason mandatory on PATCH /disable
//
// Strategy: Unit tests for HandleDisableWorkflow validation.
// ========================================

var _ = Describe("Workflow Disable Handler (GAP-WF-5)", func() {

	// Helper to create request with chi route context (workflowID in URL)
	reqWithWorkflowID := func(method, body string, workflowID string) *http.Request {
		var bodyReader *bytes.Reader
		if body != "" {
			bodyReader = bytes.NewReader([]byte(body))
		} else {
			bodyReader = bytes.NewReader([]byte("{}"))
		}
		req := httptest.NewRequest(method, "/api/v1/workflows/"+workflowID+"/disable", bodyReader)
		req.Header.Set("Content-Type", "application/json")
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("workflowID", workflowID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		return req
	}

	Describe("GAP-WF-5: reason mandatory on PATCH /disable", func() {
		It("should return 400 when reason is missing", func() {
			// Arrange: empty body (no reason)
			handler := server.NewHandler(nil)
			req := reqWithWorkflowID(http.MethodPatch, "{}", "550e8400-e29b-41d4-a716-446655440000")
			rr := httptest.NewRecorder()

			// Act
			handler.HandleDisableWorkflow(rr, req)

			// Assert
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("missing-reason"))
			Expect(problem["title"]).To(Equal("Missing Required Field"))
			Expect(problem["detail"]).To(Equal("reason is required for lifecycle operations"))
		})

		It("should return 400 when reason is empty string", func() {
			// Arrange: reason present but empty
			handler := server.NewHandler(nil)
			body := `{"reason": ""}`
			req := reqWithWorkflowID(http.MethodPatch, body, "550e8400-e29b-41d4-a716-446655440000")
			rr := httptest.NewRecorder()

			// Act
			handler.HandleDisableWorkflow(rr, req)

			// Assert
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("missing-reason"))
		})

		It("should return 400 when reason is whitespace only", func() {
			// Arrange: reason is only spaces
			handler := server.NewHandler(nil)
			body := `{"reason": "   "}`
			req := reqWithWorkflowID(http.MethodPatch, body, "550e8400-e29b-41d4-a716-446655440000")
			rr := httptest.NewRecorder()

			// Act
			handler.HandleDisableWorkflow(rr, req)

			// Assert
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("missing-reason"))
		})
	})
})
