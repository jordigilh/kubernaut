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

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/oci"
	"github.com/jordigilh/kubernaut/pkg/datastorage/schema"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
	deterministicuuid "github.com/jordigilh/kubernaut/pkg/datastorage/uuid"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	"github.com/jordigilh/kubernaut/test/testutil"
)

// deterministicUUIDRepo implements server.WorkflowContentIntegrityRepository.
// Unlike mockWorkflowIntegrityRepo, it preserves pre-set WorkflowIDs from the
// handler to verify deterministic UUID assignment before Create.
type deterministicUUIDRepo struct {
	activeWorkflow       *models.RemediationWorkflow
	activeByNameWorkflow *models.RemediationWorkflow
	disabledWorkflow     *models.RemediationWorkflow
	createdWorkflows     []*models.RemediationWorkflow
	updateStatusCalls    []statusUpdateCall
	createErr            error
}

func (m *deterministicUUIDRepo) GetActiveByNameAndVersion(_ context.Context, _, _ string) (*models.RemediationWorkflow, error) {
	return m.activeWorkflow, nil
}

func (m *deterministicUUIDRepo) GetActiveByWorkflowName(_ context.Context, _ string) (*models.RemediationWorkflow, error) {
	return m.activeByNameWorkflow, nil
}

func (m *deterministicUUIDRepo) GetLatestDisabledByNameAndVersion(_ context.Context, _, _ string) (*models.RemediationWorkflow, error) {
	return m.disabledWorkflow, nil
}

func (m *deterministicUUIDRepo) Create(_ context.Context, workflow *models.RemediationWorkflow) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.createdWorkflows = append(m.createdWorkflows, workflow)
	return nil
}

func (m *deterministicUUIDRepo) UpdateStatus(_ context.Context, workflowID, version, status, reason, _ string) error {
	m.updateStatusCalls = append(m.updateStatusCalls, statusUpdateCall{
		WorkflowID: workflowID,
		Version:    version,
		Status:     status,
		Reason:     reason,
	})
	return nil
}

func (m *deterministicUUIDRepo) SupersedeAndCreate(ctx context.Context, oldID, oldVersion, reason string, newWorkflow *models.RemediationWorkflow) error {
	if err := m.UpdateStatus(ctx, oldID, oldVersion, "Superseded", reason, ""); err != nil {
		return err
	}
	return m.Create(ctx, newWorkflow)
}

var deterministicBaseYAML = func() string {
	crd := testutil.NewTestWorkflowCRD("deterministic-wf", "ScaleMemory", "job")
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:      "Deterministic UUID test workflow",
		WhenToUse: "Testing PVC-wipe resilience",
	}
	crd.Spec.Execution.Bundle = "quay.io/kubernaut/workflows/scale-memory:v1.0.0@sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abc1"
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target resource"},
	}
	return testutil.MarshalWorkflowCRD(crd)
}()

var deterministicModifiedYAML = func() string {
	crd := testutil.NewTestWorkflowCRD("deterministic-wf", "RollbackDeployment", "job")
	crd.Spec.Description = sharedtypes.StructuredDescription{
		What:      "Modified deterministic UUID test workflow",
		WhenToUse: "Testing supersede with new content",
	}
	crd.Spec.Labels.Component = "deployment"
	crd.Spec.Execution.Bundle = "quay.io/kubernaut/workflows/rollback:v1.0.0@sha256:def456abc123def456abc123def456abc123def456abc123def456abc123def4"
	crd.Spec.Parameters = []models.WorkflowParameter{
		{Name: "TARGET_RESOURCE", Type: "string", Required: true, Description: "Target deployment"},
	}
	return testutil.MarshalWorkflowCRD(crd)
}()

var _ = Describe("Deterministic UUID Handler Integration (#548)", func() {

	makeInlineRequest := func(content string) *http.Request {
		body := map[string]string{"content": content}
		jsonBody, err := json.Marshal(body)
		Expect(err).ToNot(HaveOccurred())
		return httptest.NewRequest(http.MethodPost, "/api/v1/workflows", bytes.NewReader(jsonBody))
	}

	newDeterministicHandler := func(mockRepo *deterministicUUIDRepo) *server.Handler {
		puller := oci.NewMockImagePuller(deterministicBaseYAML)
		parser := schema.NewParser()
		extractor := oci.NewSchemaExtractor(puller, parser)
		return server.NewHandler(nil,
			server.WithSchemaExtractor(extractor),
			server.WithWorkflowContentIntegrityRepository(mockRepo),
		)
	}

	// ========================================
	// UT-DS-548-006: New workflow gets deterministic UUID from content hash
	// ========================================
	Describe("UT-DS-548-006: New workflow gets deterministic UUID derived from content_hash", func() {
		It("should assign WorkflowID = DeterministicUUID(contentHash) before calling Create", func() {
			mockRepo := &deterministicUUIDRepo{}

			handler := newDeterministicHandler(mockRepo)
			req := makeInlineRequest(deterministicBaseYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusCreated),
				"New workflow should return 201 Created, got %d: %s", rr.Code, rr.Body.String())

			Expect(mockRepo.createdWorkflows).To(HaveLen(1))
			created := mockRepo.createdWorkflows[0]
			expectedUUID := deterministicuuid.DeterministicUUID(created.ContentHash)
			Expect(created.WorkflowID).To(Equal(expectedUUID),
				"WorkflowID should be DeterministicUUID(contentHash), got %q, expected %q",
				created.WorkflowID, expectedUUID)

			var resp map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["workflowId"]).To(Equal(expectedUUID),
				"Response should include the deterministic UUID")
		})
	})

	// ========================================
	// UT-DS-548-007: Idempotent re-apply returns same deterministic UUID
	// ========================================
	Describe("UT-DS-548-007: Idempotent re-apply returns same deterministic UUID", func() {
		It("should return 200 with the existing deterministic UUID when content hash matches", func() {
			contentHash := computeTestHash(deterministicBaseYAML)
			existingUUID := deterministicuuid.DeterministicUUID(contentHash)

			mockRepo := &deterministicUUIDRepo{
				activeWorkflow: &models.RemediationWorkflow{
					WorkflowID:   existingUUID,
					WorkflowName: "deterministic-wf",
					Version:      "1.0.0",
					Status:       "Active",
					Content:      deterministicBaseYAML,
					ContentHash:  contentHash,
				},
			}

			handler := newDeterministicHandler(mockRepo)
			req := makeInlineRequest(deterministicBaseYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusOK),
				"Idempotent re-apply should return 200, got %d: %s", rr.Code, rr.Body.String())

			var resp map[string]interface{}
			Expect(json.Unmarshal(rr.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["workflowId"]).To(Equal(existingUUID),
				"Should return the same deterministic UUID from the existing workflow")

			Expect(mockRepo.createdWorkflows).To(BeEmpty(),
				"No new workflow should be created for idempotent re-apply")
		})
	})

	// ========================================
	// UT-DS-548-008: Same version + different content returns 409 (Issue #773)
	// Previously: Supersede produced new deterministic UUID from new content
	// Now: Version-locked content immutability rejects the request
	// ========================================
	Describe("UT-DS-548-008: Same version different content returns 409 (Issue #773)", func() {
		It("should return 409 Conflict for same version with different content", func() {
			oldHash := computeTestHash(deterministicBaseYAML)
			oldUUID := deterministicuuid.DeterministicUUID(oldHash)

			mockRepo := &deterministicUUIDRepo{
				activeWorkflow: &models.RemediationWorkflow{
					WorkflowID:   oldUUID,
					WorkflowName: "deterministic-wf",
					Version:      "1.0.0",
					Status:       "Active",
					Content:      deterministicBaseYAML,
					ContentHash:  oldHash,
				},
			}

			handler := newDeterministicHandler(mockRepo)
			req := makeInlineRequest(deterministicModifiedYAML)
			rr := httptest.NewRecorder()

			handler.HandleCreateWorkflow(rr, req)

			Expect(rr.Code).To(Equal(http.StatusConflict),
				"Same version + different content should return 409, got %d: %s", rr.Code, rr.Body.String())

			Expect(mockRepo.updateStatusCalls).To(BeEmpty(),
				"No status update should occur — request is rejected")
			Expect(mockRepo.createdWorkflows).To(BeEmpty(),
				"No new workflow should be created — request is rejected")
		})
	})
})
