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
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// WORKFLOW LIFECYCLE HANDLER UNIT TESTS
// ========================================
// GAP-WF-1: DD-WORKFLOW-017 Phase 4.4 - PATCH /enable and PATCH /deprecate
//
// Strategy: Unit tests for HandleEnableWorkflow and HandleDeprecateWorkflow.
// Uses mock WorkflowLifecycleRepository for 404 and 200 scenarios.
// ========================================

// mockWorkflowLifecycleRepo implements server.WorkflowLifecycleRepository for testing.
type mockWorkflowLifecycleRepo struct {
	getByIDFn      func(ctx context.Context, workflowID string) (*models.RemediationWorkflow, error)
	updateStatusFn func(ctx context.Context, workflowID, version, status, reason, updatedBy string) error
}

func (m *mockWorkflowLifecycleRepo) GetByID(ctx context.Context, workflowID string) (*models.RemediationWorkflow, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(ctx, workflowID)
	}
	return nil, nil
}

func (m *mockWorkflowLifecycleRepo) UpdateStatus(ctx context.Context, workflowID, version, status, reason, updatedBy string) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, workflowID, version, status, reason, updatedBy)
	}
	return nil
}

func reqWithWorkflowID(method, pathSuffix, body string, workflowID string) *http.Request {
	var bodyReader *bytes.Reader
	if body != "" {
		bodyReader = bytes.NewReader([]byte(body))
	} else {
		bodyReader = bytes.NewReader([]byte("{}"))
	}
	req := httptest.NewRequest(method, "/api/v1/workflows/"+workflowID+pathSuffix, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("workflowID", workflowID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req
}

var _ = Describe("Workflow Lifecycle Handlers (GAP-WF-1)", func() {
	const testWorkflowID = "550e8400-e29b-41d4-a716-446655440000"

	// ========================================
	// PATCH /enable
	// ========================================
	Describe("PATCH /enable", func() {
		It("should return 400 when reason is missing", func() {
			handler := server.NewHandler(nil)
			req := reqWithWorkflowID(http.MethodPatch, "/enable", "{}", testWorkflowID)
			rr := httptest.NewRecorder()

			handler.HandleEnableWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("missing-reason"))
			Expect(problem["detail"]).To(Equal("reason is required for lifecycle operations"))
		})

		It("should return 400 when reason is empty string", func() {
			handler := server.NewHandler(nil)
			req := reqWithWorkflowID(http.MethodPatch, "/enable", `{"reason": ""}`, testWorkflowID)
			rr := httptest.NewRecorder()

			handler.HandleEnableWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("missing-reason"))
		})

		It("should return 404 for non-existent workflow", func() {
			mock := &mockWorkflowLifecycleRepo{
				getByIDFn: func(_ context.Context, _ string) (*models.RemediationWorkflow, error) {
					return nil, nil
				},
			}
			handler := server.NewHandler(nil, server.WithWorkflowLifecycleRepository(mock))
			req := reqWithWorkflowID(http.MethodPatch, "/enable", `{"reason": "Re-enabling for production use"}`, testWorkflowID)
			rr := httptest.NewRecorder()

			handler.HandleEnableWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusNotFound))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("not-found"))
		})

		It("should return 200 for valid request with reason", func() {
			workflow := &models.RemediationWorkflow{
				WorkflowID:   testWorkflowID,
				WorkflowName: "test-workflow",
				Version:      "v1.0.0",
				Status:       "disabled",
			}
			mock := &mockWorkflowLifecycleRepo{
				getByIDFn: func(_ context.Context, id string) (*models.RemediationWorkflow, error) {
					if id == testWorkflowID {
						return workflow, nil
					}
					return nil, nil
				},
				updateStatusFn: func(_ context.Context, wfID, version, status, reason, updatedBy string) error {
					Expect(wfID).To(Equal(testWorkflowID))
					Expect(status).To(Equal("active"))
					Expect(reason).To(Equal("Re-enabling for production use"))
					workflow.Status = "active"
					return nil
				},
			}
			handler := server.NewHandler(nil, server.WithWorkflowLifecycleRepository(mock))
			req := reqWithWorkflowID(http.MethodPatch, "/enable", `{"reason": "Re-enabling for production use"}`, testWorkflowID)
			rr := httptest.NewRecorder()

			handler.HandleEnableWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			var resp models.RemediationWorkflow
			Expect(json.NewDecoder(rr.Body).Decode(&resp)).To(Succeed())
			Expect(resp.Status).To(Equal("active"))
			Expect(resp.WorkflowID).To(Equal(testWorkflowID))
		})
	})

	// ========================================
	// PATCH /deprecate
	// ========================================
	Describe("PATCH /deprecate", func() {
		It("should return 400 when reason is missing", func() {
			handler := server.NewHandler(nil)
			req := reqWithWorkflowID(http.MethodPatch, "/deprecate", "{}", testWorkflowID)
			rr := httptest.NewRecorder()

			handler.HandleDeprecateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("missing-reason"))
			Expect(problem["detail"]).To(Equal("reason is required for lifecycle operations"))
		})

		It("should return 400 when reason is empty string", func() {
			handler := server.NewHandler(nil)
			req := reqWithWorkflowID(http.MethodPatch, "/deprecate", `{"reason": ""}`, testWorkflowID)
			rr := httptest.NewRecorder()

			handler.HandleDeprecateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("missing-reason"))
		})

		It("should return 404 for non-existent workflow", func() {
			mock := &mockWorkflowLifecycleRepo{
				getByIDFn: func(_ context.Context, _ string) (*models.RemediationWorkflow, error) {
					return nil, nil
				},
			}
			handler := server.NewHandler(nil, server.WithWorkflowLifecycleRepository(mock))
			req := reqWithWorkflowID(http.MethodPatch, "/deprecate", `{"reason": "Superseded by v2"}`, testWorkflowID)
			rr := httptest.NewRecorder()

			handler.HandleDeprecateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusNotFound))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("not-found"))
		})

		It("should return 200 for valid request with reason", func() {
			workflow := &models.RemediationWorkflow{
				WorkflowID:   testWorkflowID,
				WorkflowName: "test-workflow",
				Version:      "v1.0.0",
				Status:       "active",
			}
			mock := &mockWorkflowLifecycleRepo{
				getByIDFn: func(_ context.Context, id string) (*models.RemediationWorkflow, error) {
					if id == testWorkflowID {
						return workflow, nil
					}
					return nil, nil
				},
				updateStatusFn: func(_ context.Context, wfID, version, status, reason, updatedBy string) error {
					Expect(wfID).To(Equal(testWorkflowID))
					Expect(status).To(Equal("deprecated"))
					Expect(reason).To(Equal("Superseded by v2"))
					workflow.Status = "deprecated"
					return nil
				},
			}
			handler := server.NewHandler(nil, server.WithWorkflowLifecycleRepository(mock))
			req := reqWithWorkflowID(http.MethodPatch, "/deprecate", `{"reason": "Superseded by v2"}`, testWorkflowID)
			rr := httptest.NewRecorder()

			handler.HandleDeprecateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			var resp models.RemediationWorkflow
			Expect(json.NewDecoder(rr.Body).Decode(&resp)).To(Succeed())
			Expect(resp.Status).To(Equal("deprecated"))
			Expect(resp.WorkflowID).To(Equal(testWorkflowID))
		})
	})
})
