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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
	"github.com/jordigilh/kubernaut/pkg/datastorage/server"
)

// ========================================
// ON-DEMAND WORKFLOW SUCCESS-RATE AGGREGATION -- CHECKPOINT W (Issue #1661 Change 7)
// ========================================
// Authority: DD-WORKFLOW-018. Proves the full production wiring end to end
// against a real PostgreSQL database:
//   server_construction.go's WithSuccessMetricsRepository(auditDeps.auditEventsRepo)
//   -> Handler.overlaySuccessMetrics -> HandleGetWorkflowByID / HandleListWorkflows
// computes total_executions/successful_executions/actual_success_rate from
// audit_events rows written via the same AuditEventsRepository.Create path
// the WorkflowExecution reconciler uses -- not from a stored catalog column
// (migration 015 dropped it).
//
// Business Requirements: BR-STORAGE-015, BR-STORAGE-014, BR-STORAGE-039.
// ========================================
var _ = Describe("On-demand workflow success-rate aggregation (Issue #1661 Change 7)", Label("integration", "datastorage"), func() {
	var (
		auditRepo *repository.AuditEventsRepository
		handler   *server.Handler
		testID    string
	)

	BeforeEach(func() {
		auditRepo = repository.NewAuditEventsRepository(db.DB, logger)
		// #1661 Phase F: workflows are seeded via CRD (seedWorkflowCRD below), not
		// Postgres, so the handler's workflow repository must read from the shared
		// cache instead of falling back to Postgres (DD-WORKFLOW-018).
		cachedWorkflowRepo := repository.NewWorkflowRepository(db, logger)
		cachedWorkflowRepo.SetCache(sharedWorkflowCache)
		handler = server.NewHandler(
			server.WithLogger(logger),
			server.WithWorkflowRepository(cachedWorkflowRepo),
			server.WithSuccessMetricsRepository(auditRepo),
		)

		testID = generateTestID()
	})

	AfterEach(func() {
		if db == nil {
			return
		}
		_, _ = db.ExecContext(ctx,
			"DELETE FROM audit_events WHERE correlation_id LIKE $1",
			fmt.Sprintf("test-successmetrics-%s%%", testID))
	})

	createWorkflow := func(name string) string {
		return seedWorkflowCRD(workflowCRDSpec{
			Name:       name,
			ActionType: "ScaleReplicas",
			Engine:     "job",
		})
	}

	// seedExecutionEvent writes a workflowexecution.workflow.completed/.failed
	// audit event referencing workflowID, mirroring the WorkflowExecution
	// reconciler's production audit-write path (AuditEventsRepository.Create).
	seedExecutionEvent := func(workflowID, eventType string) {
		outcome := "success"
		if eventType == "workflowexecution.workflow.failed" {
			outcome = "failure"
		}
		event := &repository.AuditEvent{
			EventID:        uuid.New(),
			Version:        "1.0",
			EventTimestamp: time.Now().UTC(),
			EventType:      eventType,
			EventCategory:  "workflow",
			EventAction:    "executed",
			EventOutcome:   outcome,
			CorrelationID:  fmt.Sprintf("test-successmetrics-%s-%s", testID, uuid.New().String()),
			ResourceType:   "WorkflowExecution",
			ResourceID:     workflowID,
			ActorType:      "controller",
			ActorID:        "workflowexecution-controller",
			EventData: map[string]interface{}{
				"workflow_id": workflowID,
			},
		}
		_, err := auditRepo.Create(ctx, event)
		Expect(err).ToNot(HaveOccurred())
	}

	getWorkflowByID := func(workflowID string) *models.RemediationWorkflow {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/"+workflowID, nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("workflowID", workflowID)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		rec := httptest.NewRecorder()
		handler.HandleGetWorkflowByID(rec, req)
		Expect(rec.Code).To(Equal(http.StatusOK), "response body: %s", rec.Body.String())

		var wf models.RemediationWorkflow
		Expect(json.Unmarshal(rec.Body.Bytes(), &wf)).To(Succeed())
		return &wf
	}

	It("IT-DS-1661-702-001: HandleGetWorkflowByID computes actual_success_rate from audit_events, not a stored column", func() {
		workflowID := createWorkflow(fmt.Sprintf("wf-successmetrics-%s-executed", testID))
		seedExecutionEvent(workflowID, "workflowexecution.workflow.completed")
		seedExecutionEvent(workflowID, "workflowexecution.workflow.completed")
		seedExecutionEvent(workflowID, "workflowexecution.workflow.failed")

		Eventually(func() int {
			return getWorkflowByID(workflowID).TotalExecutions
		}, 10*time.Second, 200*time.Millisecond).Should(Equal(3), "audit_events writes must be visible before asserting on them")

		got := getWorkflowByID(workflowID)
		Expect(got.TotalExecutions).To(Equal(3))
		Expect(got.SuccessfulExecutions).To(Equal(2))
		Expect(got.ActualSuccessRate).ToNot(BeNil())
		Expect(*got.ActualSuccessRate).To(BeNumerically("~", 2.0/3.0, 0.0001))
	})

	It("IT-DS-1661-702-002: a never-executed workflow returns zero-value metrics, not a stale stored value", func() {
		workflowID := createWorkflow(fmt.Sprintf("wf-successmetrics-%s-neverexecuted", testID))

		got := getWorkflowByID(workflowID)
		Expect(got.TotalExecutions).To(Equal(0))
		Expect(got.SuccessfulExecutions).To(Equal(0))
		Expect(got.ActualSuccessRate).To(BeNil())
	})
})
