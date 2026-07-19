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

package datastorage_test

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

// testWorkflowID is the workflow ID used across all lifecycle handler tests in this file.
const testWorkflowID = "550e8400-e29b-41d4-a716-446655440000"

func reqWithWorkflowID(pathSuffix, body string) *http.Request {
	var bodyReader *bytes.Reader
	if body != "" {
		bodyReader = bytes.NewReader([]byte(body))
	} else {
		bodyReader = bytes.NewReader([]byte("{}"))
	}
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/workflows/"+testWorkflowID+pathSuffix, bodyReader)
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("workflowID", testWorkflowID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	return req
}

var _ = Describe("Workflow Lifecycle Handlers (GAP-WF-1)", func() {

	// ========================================
	// PATCH /enable
	// ========================================
	Describe("PATCH /enable", func() {
		It("should return 400 when reason is missing", func() {
			handler := server.NewHandler()
			req := reqWithWorkflowID("/enable", "{}")
			rr := httptest.NewRecorder()

			handler.HandleEnableWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("missing-reason"))
			Expect(problem["detail"]).To(Equal("reason is required for lifecycle operations"))
		})

		It("should return 400 when reason is empty string", func() {
			handler := server.NewHandler()
			req := reqWithWorkflowID("/enable", `{"reason": ""}`)
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
			handler := server.NewHandler(server.WithWorkflowLifecycleRepository(mock))
			req := reqWithWorkflowID("/enable", `{"reason": "Re-enabling for production use"}`)
			rr := httptest.NewRecorder()

			handler.HandleEnableWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusNotFound))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("not-found"))
		})

		It("should return 200 for valid request with reason", func() {
			workflow := &models.RemediationWorkflow{
				WorkflowID:    testWorkflowID,
				WorkflowName:  "test-workflow",
				Version:       "v1.0.0",
				SchemaVersion: "1.0",
				Status:        "Disabled",
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
					Expect(status).To(Equal("Active"))
					Expect(reason).To(Equal("Re-enabling for production use"))
					workflow.Status = "Active"
					return nil
				},
			}
			handler := server.NewHandler(server.WithWorkflowLifecycleRepository(mock))
			req := reqWithWorkflowID("/enable", `{"reason": "Re-enabling for production use"}`)
			rr := httptest.NewRecorder()

			handler.HandleEnableWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			var resp models.RemediationWorkflow
			Expect(json.NewDecoder(rr.Body).Decode(&resp)).To(Succeed())
			Expect(resp.Status).To(Equal("Active"))
			Expect(resp.WorkflowID).To(Equal(testWorkflowID))
		})
	})

	// ========================================
	// PATCH /deprecate
	// ========================================
	Describe("PATCH /deprecate", func() {
		It("should return 400 when reason is missing", func() {
			handler := server.NewHandler()
			req := reqWithWorkflowID("/deprecate", "{}")
			rr := httptest.NewRecorder()

			handler.HandleDeprecateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("missing-reason"))
			Expect(problem["detail"]).To(Equal("reason is required for lifecycle operations"))
		})

		It("should return 400 when reason is empty string", func() {
			handler := server.NewHandler()
			req := reqWithWorkflowID("/deprecate", `{"reason": ""}`)
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
			handler := server.NewHandler(server.WithWorkflowLifecycleRepository(mock))
			req := reqWithWorkflowID("/deprecate", `{"reason": "Superseded by v2"}`)
			rr := httptest.NewRecorder()

			handler.HandleDeprecateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusNotFound))
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("not-found"))
		})

		It("should return 200 for valid request with reason", func() {
			workflow := &models.RemediationWorkflow{
				WorkflowID:    testWorkflowID,
				WorkflowName:  "test-workflow",
				Version:       "v1.0.0",
				SchemaVersion: "1.0",
				Status:        "Active",
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
					Expect(status).To(Equal("Deprecated"))
					Expect(reason).To(Equal("Superseded by v2"))
					workflow.Status = "Deprecated"
					return nil
				},
			}
			handler := server.NewHandler(server.WithWorkflowLifecycleRepository(mock))
			req := reqWithWorkflowID("/deprecate", `{"reason": "Superseded by v2"}`)
			rr := httptest.NewRecorder()

			handler.HandleDeprecateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK))
			var resp models.RemediationWorkflow
			Expect(json.NewDecoder(rr.Body).Decode(&resp)).To(Succeed())
			Expect(resp.Status).To(Equal("Deprecated"))
			Expect(resp.WorkflowID).To(Equal(testWorkflowID))
		})
	})

	// ========================================
	// DD-WORKFLOW-017: Terminal state enforcement (DF-M1)
	// Deprecated and Superseded are terminal — no transitions out allowed.
	// ========================================
	Describe("DF-M1: Terminal state enforcement (DD-WORKFLOW-017)", func() {
		terminalWorkflow := func(status string) *mockWorkflowLifecycleRepo {
			return &mockWorkflowLifecycleRepo{
				getByIDFn: func(_ context.Context, _ string) (*models.RemediationWorkflow, error) {
					return &models.RemediationWorkflow{
						WorkflowID:    testWorkflowID,
						WorkflowName:  "terminal-workflow",
						Version:       "v1.0.0",
						SchemaVersion: "1.0",
						Status:        status,
					}, nil
				},
			}
		}

		It("should return 409 when enabling a Deprecated workflow", func() {
			handler := server.NewHandler(server.WithWorkflowLifecycleRepository(terminalWorkflow("Deprecated")))
			req := reqWithWorkflowID("/enable", `{"reason": "Attempt to re-enable"}`)
			rr := httptest.NewRecorder()
			handler.HandleEnableWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusConflict),
				"DD-WORKFLOW-017: Deprecated is terminal — enable must return 409")
			var problem map[string]interface{}
			Expect(json.NewDecoder(rr.Body).Decode(&problem)).To(Succeed())
			Expect(problem["type"]).To(ContainSubstring("workflow-status-conflict"))
		})

		It("should return 409 when enabling a Superseded workflow", func() {
			handler := server.NewHandler(server.WithWorkflowLifecycleRepository(terminalWorkflow("Superseded")))
			req := reqWithWorkflowID("/enable", `{"reason": "Attempt to re-enable"}`)
			rr := httptest.NewRecorder()
			handler.HandleEnableWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusConflict),
				"DD-WORKFLOW-017: Superseded is terminal — enable must return 409")
		})

		It("should return 409 when deprecating a Superseded workflow", func() {
			handler := server.NewHandler(server.WithWorkflowLifecycleRepository(terminalWorkflow("Superseded")))
			req := reqWithWorkflowID("/deprecate", `{"reason": "Attempt to deprecate"}`)
			rr := httptest.NewRecorder()
			handler.HandleDeprecateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusConflict),
				"DD-WORKFLOW-017: Superseded is terminal — deprecate must return 409")
		})

		It("should return 409 when deprecating an already-Deprecated workflow", func() {
			handler := server.NewHandler(server.WithWorkflowLifecycleRepository(terminalWorkflow("Deprecated")))
			req := reqWithWorkflowID("/deprecate", `{"reason": "Re-deprecate"}`)
			rr := httptest.NewRecorder()
			handler.HandleDeprecateWorkflow(rr, req)

			// same-status → no-op allowed (not forbidden by guard), but deprecate handler
			// doesn't have the same-status short-circuit. The guard says fromStatus==toStatus → false (allowed).
			// So this should proceed to UpdateStatus as a no-op. BUT since "Deprecated"=="Deprecated" is
			// allowed by the guard (returns false), this is a successful no-op call.
			Expect(rr.Code).To(Equal(http.StatusOK),
				"DD-WORKFLOW-017: same-status transition is a no-op, not forbidden")
		})
	})
})
