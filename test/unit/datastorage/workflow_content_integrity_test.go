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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgconn"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// mockWorkflowIntegrityRepo implements server.WorkflowContentIntegrityRepository for unit tests.
// Simulates pre-existing workflows in the catalog to test content integrity decisions.
type mockWorkflowIntegrityRepo struct {
	activeWorkflow        *models.RemediationWorkflow
	activeByNameWorkflow  *models.RemediationWorkflow // Issue #371: cross-version lookup
	disabledWorkflow      *models.RemediationWorkflow
	createdWorkflows      []*models.RemediationWorkflow
	updateStatusCalls     []statusUpdateCall
	createErr             error
}

type statusUpdateCall struct {
	WorkflowID string
	Version    string
	Status     string
	Reason     string
}

func (m *mockWorkflowIntegrityRepo) GetActiveByNameAndVersion(_ context.Context, _, _ string) (*models.RemediationWorkflow, error) {
	return m.activeWorkflow, nil
}

func (m *mockWorkflowIntegrityRepo) GetActiveByWorkflowName(_ context.Context, _ string) (*models.RemediationWorkflow, error) {
	return m.activeByNameWorkflow, nil
}

func (m *mockWorkflowIntegrityRepo) GetLatestDisabledByNameAndVersion(_ context.Context, _, _ string) (*models.RemediationWorkflow, error) {
	return m.disabledWorkflow, nil
}

func (m *mockWorkflowIntegrityRepo) Create(_ context.Context, workflow *models.RemediationWorkflow) error {
	if m.createErr != nil {
		return m.createErr
	}
	workflow.WorkflowID = "new-uuid-" + workflow.WorkflowName
	m.createdWorkflows = append(m.createdWorkflows, workflow)
	return nil
}

func (m *mockWorkflowIntegrityRepo) UpdateStatus(_ context.Context, workflowID, version, status, reason, _ string) error {
	m.updateStatusCalls = append(m.updateStatusCalls, statusUpdateCall{
		WorkflowID: workflowID,
		Version:    version,
		Status:     status,
		Reason:     reason,
	})
	return nil
}

func (m *mockWorkflowIntegrityRepo) SupersedeAndCreate(ctx context.Context, oldID, oldVersion, reason string, newWorkflow *models.RemediationWorkflow) error {
	if err := m.UpdateStatus(ctx, oldID, oldVersion, "Superseded", reason, ""); err != nil {
		return err
	}
	return m.Create(ctx, newWorkflow)
}

// Workflow YAML variants for integrity tests. Same name+version, different content.
var integrityBaseYAML = func() string {
	crd := testutil.NewTestWorkflowCRD("integrity-test-wf", "ScaleMemory", "job")
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:      "Scales memory limits for OOM-killed pods",
		WhenToUse: "When pods are OOM-killed repeatedly",
	}
	crd.Spec.Execution.Bundle = "quay.io/kubernaut/workflows/scale-memory:v1.0.0@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abc1"
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource"},
	}
	return testutil.MarshalWorkflowCRD(crd)
}()

var integrityModifiedYAML = func() string {
	crd := testutil.NewTestWorkflowCRD("integrity-test-wf", "RollbackDeployment", "job")
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:      "Rolls back a deployment experiencing CrashLoopBackOff",
		WhenToUse: "When pods are crash-looping due to a bad config",
	}
	crd.Spec.Labels.Component = "deployment"
	crd.Spec.Execution.Bundle = "quay.io/kubernaut/workflows/rollback:v1.0.0@sha256:def456abc123def456abc123def456abc123def456abc123def456abc123def4"
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target deployment"},
	}
	return testutil.MarshalWorkflowCRD(crd)
}()

var _ = Describe("Workflow Content Integrity (BR-WORKFLOW-006)", func() {

	makeInlineRequest := func(content string) *http.Request {
		body := map[string]string{"content": content}
		jsonBody, err := json.Marshal(body)
		Expect(err).ToNot(HaveOccurred())
		return httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(jsonBody))
	}

	newIntegrityHandler := func(mockRepo *mockWorkflowIntegrityRepo) *server.Handler {
		puller := oci.NewMockImagePuller(integrityBaseYAML)
		parser := schema.NewParser()
		extractor := oci.NewSchemaExtractor(puller, parser)
		return server.NewHandler(nil,
			server.WithSchemaExtractor(extractor),
			server.WithWorkflowContentIntegrityRepository(mockRepo),
		)
	}

	// ========================================
	// UT-DS-INTEGRITY-001: Active + same hash -> 200, same UUID (idempotent)
	// BR-WORKFLOW-006: Idempotent re-apply of unchanged workflow
	// ========================================
	Describe("UT-DS-INTEGRITY-001: Active workflow with same content hash", func() {
		It("should return 200 OK with the existing workflow UUID (idempotent)", func() {
			existingUUID := "existing-uuid-001"
			mockRepo := &mockWorkflowIntegrityRepo{
				activeWorkflow: &models.RemediationWorkflow{
					WorkflowID:   existingUUID,
					WorkflowName: "integrity-test-wf",
					Version:      "1.0.0",
					Status:       "Active",
					Content:      integrityBaseYAML,
					ContentHash:  computeTestHash(integrityBaseYAML),
				},
			}

			handler := newIntegrityHandler(mockRepo)
			req := makeInlineRequest(integrityBaseYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"Active workflow with same content hash should return 200 (idempotent), got %d: %s", rr.Code, rr.Body.String())

			var resp map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["workflowId"]).To(Equal(existingUUID),
				"Should return the existing workflow's UUID, not a new one")

			Expect(mockRepo.createdWorkflows).To(BeEmpty(),
				"No new workflow should be created for idempotent re-apply")
			Expect(mockRepo.updateStatusCalls).To(BeEmpty(),
				"No status update should occur for idempotent re-apply")
		})
	})

	// ========================================
	// UT-DS-INTEGRITY-002: Active + different hash -> 201, new UUID, old superseded
	// BR-WORKFLOW-006: Spec change for same name+version supersedes old record
	// ========================================
	Describe("UT-DS-INTEGRITY-002: Active workflow with different content hash", func() {
		It("should supersede the old workflow and return 201 with a new UUID", func() {
			oldUUID := "old-uuid-002"
			mockRepo := &mockWorkflowIntegrityRepo{
				activeWorkflow: &models.RemediationWorkflow{
					WorkflowID:   oldUUID,
					WorkflowName: "integrity-test-wf",
					Version:      "1.0.0",
					Status:       "Active",
					Content:      integrityBaseYAML,
					ContentHash:  computeTestHash(integrityBaseYAML),
				},
			}

			handler := newIntegrityHandler(mockRepo)
			req := makeInlineRequest(integrityModifiedYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusCreated),
				"Different content hash should return 201 (new workflow created), got %d: %s", rr.Code, rr.Body.String())

			var resp map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["workflowId"]).ToNot(Equal(oldUUID),
				"Should return a NEW UUID, not the old one")

			Expect(mockRepo.updateStatusCalls).To(HaveLen(1),
				"Old workflow should have its status updated")
			Expect(mockRepo.updateStatusCalls[0].WorkflowID).To(Equal(oldUUID))
			Expect(mockRepo.updateStatusCalls[0].Status).To(Equal("Superseded"),
				"Old workflow should be marked as superseded")

			Expect(mockRepo.createdWorkflows).To(HaveLen(1),
				"A new workflow record should be created")
		})
	})

	// ========================================
	// UT-DS-INTEGRITY-003: Disabled + same hash -> 200, same UUID (re-enabled)
	// BR-WORKFLOW-006: Re-enable unchanged disabled workflow
	// ========================================
	Describe("UT-DS-INTEGRITY-003: Disabled workflow with same content hash", func() {
		It("should re-enable the disabled workflow and return 200 with the same UUID", func() {
			disabledUUID := "disabled-uuid-003"
			mockRepo := &mockWorkflowIntegrityRepo{
				disabledWorkflow: &models.RemediationWorkflow{
					WorkflowID:   disabledUUID,
					WorkflowName: "integrity-test-wf",
					Version:      "1.0.0",
					Status:       "Disabled",
					Content:      integrityBaseYAML,
					ContentHash:  computeTestHash(integrityBaseYAML),
				},
			}

			handler := newIntegrityHandler(mockRepo)
			req := makeInlineRequest(integrityBaseYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"Disabled workflow with same hash should return 200 (re-enabled), got %d: %s", rr.Code, rr.Body.String())

			var resp map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["workflowId"]).To(Equal(disabledUUID),
				"Should return the re-enabled workflow's original UUID")

			Expect(mockRepo.updateStatusCalls).To(HaveLen(1))
			Expect(mockRepo.updateStatusCalls[0].WorkflowID).To(Equal(disabledUUID))
			Expect(mockRepo.updateStatusCalls[0].Status).To(Equal("Active"),
				"Disabled workflow should be re-enabled to active")

			Expect(mockRepo.createdWorkflows).To(BeEmpty(),
				"No new workflow should be created when re-enabling")
		})
	})

	// ========================================
	// UT-DS-INTEGRITY-004: Disabled + different hash -> 201, new UUID
	// BR-WORKFLOW-006: Different content for disabled workflow creates new record
	// ========================================
	Describe("UT-DS-INTEGRITY-004: Disabled workflow with different content hash", func() {
		It("should create a new workflow and return 201 with a new UUID", func() {
			disabledUUID := "disabled-uuid-004"
			mockRepo := &mockWorkflowIntegrityRepo{
				disabledWorkflow: &models.RemediationWorkflow{
					WorkflowID:   disabledUUID,
					WorkflowName: "integrity-test-wf",
					Version:      "1.0.0",
					Status:       "Disabled",
					Content:      integrityBaseYAML,
					ContentHash:  computeTestHash(integrityBaseYAML),
				},
			}

			handler := newIntegrityHandler(mockRepo)
			req := makeInlineRequest(integrityModifiedYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusCreated),
				"Different content for disabled workflow should return 201, got %d: %s", rr.Code, rr.Body.String())

			var resp map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["workflowId"]).ToNot(Equal(disabledUUID),
				"Should return a NEW UUID, not the disabled one")

			Expect(mockRepo.createdWorkflows).To(HaveLen(1),
				"A new workflow record should be created")
		})
	})

	// ========================================
	// UT-DS-INTEGRITY-005: Historical UUID retrievable after supersede
	// BR-WORKFLOW-006: Audit trail preservation — old UUID stays in catalog
	// ========================================
	Describe("UT-DS-INTEGRITY-005: Superseded workflow UUID preserved for audit", func() {
		It("should not delete the old workflow record when superseding", func() {
			oldUUID := "old-uuid-005"
			mockRepo := &mockWorkflowIntegrityRepo{
				activeWorkflow: &models.RemediationWorkflow{
					WorkflowID:   oldUUID,
					WorkflowName: "integrity-test-wf",
					Version:      "1.0.0",
					Status:       "Active",
					Content:      integrityBaseYAML,
					ContentHash:  computeTestHash(integrityBaseYAML),
				},
			}

			handler := newIntegrityHandler(mockRepo)
			req := makeInlineRequest(integrityModifiedYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusCreated))

			Expect(mockRepo.updateStatusCalls).To(HaveLen(1),
				"Old workflow should be status-updated, not deleted")
			Expect(mockRepo.updateStatusCalls[0].Status).To(Equal("Superseded"),
				"Old record must be marked 'Superseded' — never deleted")
			Expect(mockRepo.updateStatusCalls[0].Reason).To(ContainSubstring("content hash"),
				"Reason should reference content hash mismatch for auditability")
		})
	})

	// ========================================
	// UT-DS-INTEGRITY-006: Discovery excludes superseded workflows
	// BR-WORKFLOW-006: Superseded records are invisible to catalog discovery
	// ========================================
	Describe("UT-DS-INTEGRITY-006: Superseded workflows excluded from catalog response", func() {
		It("should not return superseded workflows in creation response", func() {
			mockRepo := &mockWorkflowIntegrityRepo{
				activeWorkflow: &models.RemediationWorkflow{
					WorkflowID:   "old-uuid-006",
					WorkflowName: "integrity-test-wf",
					Version:      "1.0.0",
					Status:       "Active",
					Content:      integrityBaseYAML,
					ContentHash:  computeTestHash(integrityBaseYAML),
				},
			}

			handler := newIntegrityHandler(mockRepo)
			req := makeInlineRequest(integrityModifiedYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusCreated))

			var resp map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["status"]).To(Equal("Active"),
				"Response should contain the new ACTIVE workflow, not the superseded one")
		})
	})

	// ========================================
	// UT-DS-INTEGRITY-007: Concurrent Create race (none found -> 23505) -> idempotent 200
	// ========================================
	Describe("UT-DS-INTEGRITY-007: Concurrent create race — 23505 on INSERT retries to idempotent 200", func() {
		It("should return 200 OK when Create fails with 23505 and retry finds the committed workflow", func() {
			winnerUUID := "winner-uuid-007"
			raceRepo := &raceConditionIntegrityRepo{
				activeOnRetry: &models.RemediationWorkflow{
					WorkflowID:   winnerUUID,
					WorkflowName: "integrity-test-wf",
					Version:      "1.0.0",
					Status:       "Active",
					Content:      integrityBaseYAML,
					ContentHash:  computeTestHash(integrityBaseYAML),
				},
			}

			puller := oci.NewMockImagePuller(integrityBaseYAML)
			parser := schema.NewParser()
			extractor := oci.NewSchemaExtractor(puller, parser)
			handler := server.NewHandler(nil,
				server.WithSchemaExtractor(extractor),
				server.WithWorkflowContentIntegrityRepository(raceRepo),
			)

			req := makeInlineRequest(integrityBaseYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"23505 race should retry lookup and return 200 (idempotent), got %d: %s", rr.Code, rr.Body.String())

			var resp map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["workflowId"]).To(Equal(winnerUUID),
				"Should return the winner's UUID from the retried lookup")

			Expect(raceRepo.getActiveCalls.Load()).To(BeNumerically("==", 2),
				"GetActiveByNameAndVersion should be called twice: initial + retry after 23505")
		})
	})

	// ========================================
	// UT-DS-371-001: Cross-version supersession — old version superseded
	// Issue #371, BR-WORKFLOW-006: When a new version of an existing workflow
	// is registered, the old active entry must be marked superseded.
	// ========================================
	Describe("UT-DS-371-001: Cross-version supersession marks old entry superseded", func() {
		It("should supersede old version and create new entry when workflow_name matches but version differs", func() {
			oldUUID := "old-uuid-v090"
			mockRepo := &mockWorkflowIntegrityRepo{
				activeByNameWorkflow: &models.RemediationWorkflow{
					WorkflowID:   oldUUID,
					WorkflowName: "integrity-test-wf",
					Version:      "0.9.0",
					ActionType:   "ScaleMemory",
					Status:       "Active",
					Content:      "old-content",
					ContentHash:  "old-hash",
				},
			}

			handler := newIntegrityHandler(mockRepo)
			req := makeInlineRequest(integrityBaseYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusCreated),
				"Cross-version update should create a new entry (201), got %d: %s", rr.Code, rr.Body.String())

			Expect(mockRepo.updateStatusCalls).To(HaveLen(1),
				"Exactly one UpdateStatus call expected (supersede old)")
			Expect(mockRepo.updateStatusCalls[0].WorkflowID).To(Equal(oldUUID),
				"Superseded workflow should be the old version")
			Expect(mockRepo.updateStatusCalls[0].Status).To(Equal("Superseded"),
				"Old entry must be marked superseded")

			Expect(mockRepo.createdWorkflows).To(HaveLen(1),
				"Exactly one new workflow should be created")
			Expect(mockRepo.createdWorkflows[0].WorkflowName).To(Equal("integrity-test-wf"))
		})
	})

	// ========================================
	// UT-DS-371-002: Cross-version detection creates new entry after superseding
	// Issue #371, BR-WORKFLOW-006: Verifies the full flow — GetActiveByNameAndVersion
	// returns nil (version mismatch), GetActiveByWorkflowName finds the old version.
	// ========================================
	Describe("UT-DS-371-002: Cross-version detection flow correctness", func() {
		It("should fall through name+version check, find old by name-only, supersede, and create", func() {
			oldUUID := "old-uuid-v100"
			mockRepo := &mockWorkflowIntegrityRepo{
				activeByNameWorkflow: &models.RemediationWorkflow{
					WorkflowID:   oldUUID,
					WorkflowName: "integrity-test-wf",
					Version:      "1.0.0",
					ActionType:   "ScaleMemory",
					Status:       "Active",
					Content:      integrityModifiedYAML,
					ContentHash:  computeTestHash(integrityModifiedYAML),
				},
			}

			handler := newIntegrityHandler(mockRepo)
			req := makeInlineRequest(integrityBaseYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusCreated),
				"New version should be created (201), got %d: %s", rr.Code, rr.Body.String())

			Expect(mockRepo.updateStatusCalls).To(HaveLen(1))
			Expect(mockRepo.updateStatusCalls[0].Status).To(Equal("Superseded"),
				"Old entry status should be superseded")
			Expect(mockRepo.updateStatusCalls[0].Reason).To(ContainSubstring("superseded"),
				"Reason should explain the supersession")

			var resp map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["workflowId"]).ToNot(Equal(oldUUID),
				"Response should contain the new workflow UUID, not the superseded one")
		})
	})

	// ========================================
	// UT-DS-371-003: Idempotent re-apply — no supersession
	// Issue #371, BR-WORKFLOW-006: Same name+version+hash returns 200 without
	// triggering cross-version supersession.
	// ========================================
	Describe("UT-DS-371-003: Idempotent re-apply does not trigger supersession", func() {
		It("should return 200 without superseding when content hash matches", func() {
			existingUUID := "existing-uuid-idempotent"
			mockRepo := &mockWorkflowIntegrityRepo{
				activeWorkflow: &models.RemediationWorkflow{
					WorkflowID:   existingUUID,
					WorkflowName: "integrity-test-wf",
					Version:      "1.0.0",
					Status:       "Active",
					Content:      integrityBaseYAML,
					ContentHash:  computeTestHash(integrityBaseYAML),
				},
			}

			handler := newIntegrityHandler(mockRepo)
			req := makeInlineRequest(integrityBaseYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"Idempotent re-apply should return 200, got %d: %s", rr.Code, rr.Body.String())

			Expect(mockRepo.updateStatusCalls).To(BeEmpty(),
				"No UpdateStatus calls expected for idempotent re-apply (no supersession)")
			Expect(mockRepo.createdWorkflows).To(BeEmpty(),
				"No Create calls expected for idempotent re-apply")

			var resp map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["workflowId"]).To(Equal(existingUUID),
				"Response should return the existing UUID")
		})
	})
})

// raceConditionIntegrityRepo simulates the race condition where two concurrent
// CreateWorkflow requests both pass the GetActiveByNameAndVersion check (returning nil),
// one wins the INSERT, and the other gets a PostgreSQL 23505 unique constraint violation.
type raceConditionIntegrityRepo struct {
	getActiveCalls atomic.Int32
	activeOnRetry  *models.RemediationWorkflow
}

func (m *raceConditionIntegrityRepo) GetActiveByNameAndVersion(_ context.Context, _, _ string) (*models.RemediationWorkflow, error) {
	call := m.getActiveCalls.Add(1)
	if call == 1 {
		return nil, nil
	}
	return m.activeOnRetry, nil
}

func (m *raceConditionIntegrityRepo) GetActiveByWorkflowName(_ context.Context, _ string) (*models.RemediationWorkflow, error) {
	return nil, nil
}

func (m *raceConditionIntegrityRepo) GetLatestDisabledByNameAndVersion(_ context.Context, _, _ string) (*models.RemediationWorkflow, error) {
	return nil, nil
}

func (m *raceConditionIntegrityRepo) Create(_ context.Context, workflow *models.RemediationWorkflow) error {
	return &pgconn.PgError{
		Code:           "23505",
		ConstraintName: "uq_workflow_name_version_active",
		Message:        fmt.Sprintf("duplicate key value violates unique constraint for (%s, %s)", workflow.WorkflowName, workflow.Version),
	}
}

func (m *raceConditionIntegrityRepo) UpdateStatus(_ context.Context, _, _, _, _, _ string) error {
	return nil
}

func (m *raceConditionIntegrityRepo) SupersedeAndCreate(ctx context.Context, oldID, oldVersion, reason string, newWorkflow *models.RemediationWorkflow) error {
	if err := m.UpdateStatus(ctx, oldID, oldVersion, "Superseded", reason, ""); err != nil {
		return err
	}
	return m.Create(ctx, newWorkflow)
}

// computeTestHash computes SHA-256 for test content comparison.
// Mirrors the production computeContentHash (unexported in server package).
func computeTestHash(content string) string {
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:])
}
