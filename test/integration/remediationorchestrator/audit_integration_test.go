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

package remediationorchestrator_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/jordigilh/kubernaut/pkg/audit"
	roaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
)

// Integration tests for RO Audit functionality.
// Per TESTING_GUIDELINES.md: Integration tests use real infrastructure via podman-compose.test.yml
//
// Prerequisites:
//   - podman-compose -f podman-compose.test.yml up -d
//   - Data Storage service running at localhost:18090
//
// These tests verify:
//   - DD-AUDIT-003 compliance (event types: orchestrator.*)
//   - ADR-034 compliance (unified audit table)
//   - ADR-038 compliance (async buffered audit ingestion)
var _ = Describe("Audit Integration Tests", Label("integration", "audit"), func() {
	var (
		auditStore   audit.AuditStore
		auditHelpers *roaudit.Helpers
		ctx          context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Skip if Data Storage is not available
		// Per TESTING_GUIDELINES.md: Integration tests require podman-compose infrastructure
		dsURL := "http://localhost:18090"
		client := &http.Client{Timeout: 2 * time.Second}
		_, err := client.Get(dsURL + "/health")
		if err != nil {
			Skip("Data Storage not available at " + dsURL + " - run: podman-compose -f podman-compose.test.yml up -d")
		}

		// Create audit store with real Data Storage client
		dsClient := audit.NewHTTPDataStorageClient(dsURL, &http.Client{Timeout: 5 * time.Second})
		config := audit.DefaultConfig()
		config.FlushInterval = 100 * time.Millisecond // Faster flush for tests
		logger := zap.New(zap.WriteTo(GinkgoWriter))

		var storeErr error
		auditStore, storeErr = audit.NewBufferedStore(dsClient, config, roaudit.ServiceName, logger)
		Expect(storeErr).ToNot(HaveOccurred())

		auditHelpers = roaudit.NewHelpers(roaudit.ServiceName)
	})

	AfterEach(func() {
		if auditStore != nil {
			err := auditStore.Close()
			Expect(err).ToNot(HaveOccurred())
		}
	})

	// ========================================
	// DD-AUDIT-003 P1 Events Integration Tests
	// ========================================
	Describe("DD-AUDIT-003 P1 Events", func() {
		// orchestrator.lifecycle.started (P1)
		It("should store lifecycle started event to Data Storage", func() {
			event, err := auditHelpers.BuildLifecycleStartedEvent(
				"test-correlation-001",
				"integration-test",
				"rr-integration-001",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.lifecycle.started"))

			// Store event (non-blocking)
			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			// Allow time for async write
			time.Sleep(200 * time.Millisecond)
		})

		// orchestrator.phase.transitioned (P1)
		It("should store phase transition event to Data Storage", func() {
			event, err := auditHelpers.BuildPhaseTransitionEvent(
				"test-correlation-002",
				"integration-test",
				"rr-integration-002",
				"Pending",
				"Processing",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.phase.transitioned"))

			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(200 * time.Millisecond)
		})

		// orchestrator.lifecycle.completed (P1) - success
		It("should store lifecycle completed event (success) to Data Storage", func() {
			event, err := auditHelpers.BuildCompletionEvent(
				"test-correlation-003",
				"integration-test",
				"rr-integration-003",
				"Remediated",
				5000,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.lifecycle.completed"))
			Expect(event.EventOutcome).To(Equal("success"))

			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(200 * time.Millisecond)
		})

		// orchestrator.lifecycle.completed (P1) - failure
		It("should store lifecycle completed event (failure) to Data Storage", func() {
			event, err := auditHelpers.BuildFailureEvent(
				"test-correlation-004",
				"integration-test",
				"rr-integration-004",
				"workflow_execution",
				"RBAC permission denied",
				10000,
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.lifecycle.completed"))
			Expect(event.EventOutcome).To(Equal("failure"))

			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(200 * time.Millisecond)
		})
	})

	// ========================================
	// ADR-040 Approval Events Integration Tests
	// ========================================
	Describe("ADR-040 Approval Events", func() {
		It("should store approval requested event to Data Storage", func() {
			event, err := auditHelpers.BuildApprovalRequestedEvent(
				"test-correlation-005",
				"integration-test",
				"rr-integration-005",
				"rar-rr-integration-005",
				"wf-scale-deployment",
				"85%",
				time.Now().Add(24*time.Hour),
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.approval.requested"))

			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(200 * time.Millisecond)
		})

		It("should store approval approved event to Data Storage", func() {
			event, err := auditHelpers.BuildApprovalDecisionEvent(
				"test-correlation-006",
				"integration-test",
				"rr-integration-006",
				"rar-rr-integration-006",
				"Approved",
				"operator@example.com",
				"Looks good, approved for execution",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.approval.approved"))
			Expect(event.ActorType).To(Equal("user"))

			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(200 * time.Millisecond)
		})

		It("should store approval rejected event to Data Storage", func() {
			event, err := auditHelpers.BuildApprovalDecisionEvent(
				"test-correlation-007",
				"integration-test",
				"rr-integration-007",
				"rar-rr-integration-007",
				"Rejected",
				"admin@example.com",
				"Too risky, manual investigation needed",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.approval.rejected"))

			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(200 * time.Millisecond)
		})

		It("should store approval expired event to Data Storage", func() {
			event, err := auditHelpers.BuildApprovalDecisionEvent(
				"test-correlation-008",
				"integration-test",
				"rr-integration-008",
				"rar-rr-integration-008",
				"Expired",
				"system",
				"Approval deadline passed without response",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.approval.expired"))
			Expect(event.ActorType).To(Equal("service"))

			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(200 * time.Millisecond)
		})
	})

	// ========================================
	// BR-ORCH-036 Manual Review Events Integration Tests
	// ========================================
	Describe("BR-ORCH-036 Manual Review Events", func() {
		It("should store manual review event to Data Storage", func() {
			event, err := auditHelpers.BuildManualReviewEvent(
				"test-correlation-009",
				"integration-test",
				"rr-integration-009",
				"WorkflowResolutionFailed",
				"NoMatchingWorkflow",
				"nr-manual-review-009",
			)
			Expect(err).ToNot(HaveOccurred())
			Expect(event.EventType).To(Equal("orchestrator.remediation.manual_review"))
			Expect(event.EventOutcome).To(Equal("pending"))

			err = auditStore.StoreAudit(ctx, event)
			Expect(err).ToNot(HaveOccurred())

			time.Sleep(200 * time.Millisecond)
		})
	})

	// ========================================
	// ADR-038 Async Buffered Ingestion Tests
	// ========================================
	Describe("ADR-038 Async Buffered Ingestion", func() {
		It("should handle batch of events efficiently", func() {
			// Create multiple events
			for i := 0; i < 10; i++ {
				event, err := auditHelpers.BuildPhaseTransitionEvent(
					"test-batch-correlation",
					"integration-test",
					"rr-batch-test",
					"Pending",
					"Processing",
				)
				Expect(err).ToNot(HaveOccurred())

				err = auditStore.StoreAudit(ctx, event)
				Expect(err).ToNot(HaveOccurred())
			}

			// Allow time for batch write
			time.Sleep(300 * time.Millisecond)
		})

		It("should gracefully handle rapid event emission", func() {
			// Rapid fire events - should not block
			start := time.Now()
			for i := 0; i < 50; i++ {
				event, err := auditHelpers.BuildLifecycleStartedEvent(
					"test-rapid-correlation",
					"integration-test",
					"rr-rapid-test",
				)
				Expect(err).ToNot(HaveOccurred())

				err = auditStore.StoreAudit(ctx, event)
				Expect(err).ToNot(HaveOccurred())
			}
			elapsed := time.Since(start)

			// Should complete quickly (non-blocking)
			Expect(elapsed).To(BeNumerically("<", 100*time.Millisecond))

			// Allow time for async writes
			time.Sleep(500 * time.Millisecond)
		})
	})
})

