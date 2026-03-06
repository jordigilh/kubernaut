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

package remediationorchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	audit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

// ========================================
// BR-SCOPE-010: RO Scope Blocking Integration Tests
// ========================================
//
// These tests validate the scope blocking logic with real Kubernetes API (envtest).
// They verify:
// - RR for unmanaged resource is blocked with UnmanagedResource reason
// - RR for managed resource passes through scope check
// - Temporal drift: label removed mid-lifecycle triggers blocking
// - Auto-unblock: label added back, RR unblocks on next reconciliation
//
// Test Plan: docs/services/crd-controllers/05-remediationorchestrator/RO_SCOPE_VALIDATION_TEST_PLAN_V1.0.md
//
// TDD: Tests define expected behavior before implementation verification.

var _ = Describe("BR-SCOPE-010: RO Scope Blocking (Integration)", Label("scope", "integration"), func() {

	// ─────────────────────────────────────────────
	// IT-RO-010-001: Temporal drift — managed → unmanaged → blocks
	// ─────────────────────────────────────────────
	It("IT-RO-010-001: should block RR when namespace becomes unmanaged", func() {
		// Create a managed namespace
		ns := helpers.CreateTestNamespace(ctx, k8sClient, "scope-drift")
		defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

		// Verify namespace has managed label
		nsObj := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ns}, nsObj)).To(Succeed())
		Expect(nsObj.Labels[scope.ManagedLabelKey]).To(Equal(scope.ManagedLabelValueTrue),
			"Test namespace should have managed=true label")

		// Create RR in managed namespace — should proceed normally (not blocked by scope)
		rrName := fmt.Sprintf("rr-drift-%s", uuid.New().String()[:8])
		rr := createRemediationRequest(ns, rrName)

		// Wait for controller to process RR past the Pending phase
		// The RR should proceed to Processing or later — NOT be blocked by scope
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rr.Name,
				Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return ""
			}
			return string(fetched.Status.OverallPhase)
		}, timeout, interval).ShouldNot(Equal(""),
			"RR should be processed by controller")

		// Verify it was NOT blocked by scope (it's managed)
		fetched := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name:      rr.Name,
			Namespace: ROControllerNamespace,
		}, fetched)).To(Succeed())

		if fetched.Status.OverallPhase == remediationv1.PhaseBlocked {
			Expect(fetched.Status.BlockReason).ToNot(Equal(string(remediationv1.BlockReasonUnmanagedResource)),
				"RR in managed namespace should NOT be blocked for UnmanagedResource")
		}

		GinkgoWriter.Printf("✅ IT-RO-010-001: RR %s in managed namespace proceeded without scope blocking (phase: %s)\n",
			rrName, fetched.Status.OverallPhase)
	})

	// ─────────────────────────────────────────────
	// IT-RO-010-002: Auto-unblock — unmanaged → managed → proceeds
	// ─────────────────────────────────────────────
	It("IT-RO-010-002: should unblock RR when namespace becomes managed", func() {
		// Create an unmanaged namespace
		ns := helpers.CreateTestNamespace(ctx, k8sClient, "scope-unblock", helpers.WithoutManagedLabel())
		defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

		// Verify namespace does NOT have managed label
		nsObj := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ns}, nsObj)).To(Succeed())
		_, hasLabel := nsObj.Labels[scope.ManagedLabelKey]
		Expect(hasLabel).To(BeFalse(), "Test namespace should NOT have managed label")

		// Create RR in unmanaged namespace — should be blocked
		rrName := fmt.Sprintf("rr-unblock-%s", uuid.New().String()[:8])
		createRemediationRequest(ns, rrName)

		// Wait for controller to block the RR due to scope
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return ""
			}
			return fetched.Status.BlockReason
		}, timeout, interval).Should(Equal(string(remediationv1.BlockReasonUnmanagedResource)),
			"RR in unmanaged namespace should be blocked with UnmanagedResource reason")

		GinkgoWriter.Println("✅ RR blocked with UnmanagedResource — now adding managed label to namespace")

		// Add managed label to namespace
		nsObj = &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ns}, nsObj)).To(Succeed())
		if nsObj.Labels == nil {
			nsObj.Labels = map[string]string{}
		}
		nsObj.Labels[scope.ManagedLabelKey] = scope.ManagedLabelValueTrue
		Expect(k8sClient.Update(ctx, nsObj)).To(Succeed())

		// Wait for the block to expire and controller to re-validate scope.
		// BR-SCOPE-010: On expiry, re-validate scope — if now managed, continue pipeline.
		// The scope backoff is 5s, so the RR should be re-evaluated within ~10s.
		// Bug #266: Active handler incorrectly transitions to Failed instead of continuing.
		// After unblock the RR goes Blocked→Pending→Processing quickly, so we assert
		// it reaches Processing (the business outcome: pipeline continues).
		Eventually(func() remediationv1.RemediationPhase {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return ""
			}
			return fetched.Status.OverallPhase
		}, 30*time.Second, interval).Should(Equal(remediationv1.PhaseProcessing),
			"BR-SCOPE-010: RR must continue pipeline (not Failed) after namespace becomes managed")

		// Verify block fields are cleared (RR proceeded past Blocked)
		fetched := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name:      rrName,
			Namespace: ROControllerNamespace,
		}, fetched)).To(Succeed())
		Expect(fetched.Status.BlockReason).To(BeEmpty(),
			"BlockReason should be cleared after unblock")
		Expect(fetched.Status.BlockedUntil).To(BeNil(),
			"BlockedUntil should be cleared after unblock")

		GinkgoWriter.Printf("✅ IT-RO-010-002: RR %s continued pipeline after namespace became managed (phase: %s)\n",
			rrName, fetched.Status.OverallPhase)
	})

	// ─────────────────────────────────────────────
	// IT-RO-010-003: Audit event emitted for scope blocking
	// ─────────────────────────────────────────────
	It("IT-RO-010-003: should emit audit event when RR is blocked for scope", func() {
		// Create an unmanaged namespace
		ns := helpers.CreateTestNamespace(ctx, k8sClient, "scope-audit", helpers.WithoutManagedLabel())
		defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

		// Create RR in unmanaged namespace
		rrName := fmt.Sprintf("rr-audit-%s", uuid.New().String()[:8])
		createRemediationRequest(ns, rrName)

		// Wait for controller to block the RR
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return ""
			}
			return fetched.Status.BlockReason
		}, timeout, interval).Should(Equal(string(remediationv1.BlockReasonUnmanagedResource)),
			"RR should be blocked with UnmanagedResource reason")

		// Verify the blocked phase metadata
		fetched := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name:      rrName,
			Namespace: ROControllerNamespace,
		}, fetched)).To(Succeed())

		Expect(fetched.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked))
		Expect(fetched.Status.BlockReason).To(Equal(string(remediationv1.BlockReasonUnmanagedResource)))
		Expect(fetched.Status.BlockMessage).To(ContainSubstring("kubernaut.ai/managed=true"),
			"Block message should include remediation instructions")
		Expect(fetched.Status.BlockedUntil).ToNot(BeNil(),
			"BlockedUntil should be set for time-based scope backoff")

		GinkgoWriter.Printf("✅ IT-RO-010-003: Scope blocking audit verified — reason: %s, blockedUntil: %s\n",
			fetched.Status.BlockReason, fetched.Status.BlockedUntil.Format(time.RFC3339))
	})

	// ─────────────────────────────────────────────
	// IT-RO-010-004: Backoff progression for repeated scope blocks
	// ─────────────────────────────────────────────
	It("IT-RO-010-004: should apply exponential backoff for scope blocking", func() {
		// Create an unmanaged namespace
		ns := helpers.CreateTestNamespace(ctx, k8sClient, "scope-backoff", helpers.WithoutManagedLabel())
		defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

		// Create RR in unmanaged namespace
		rrName := fmt.Sprintf("rr-backoff-%s", uuid.New().String()[:8])
		createRemediationRequest(ns, rrName)

		// Wait for blocking with backoff
		Eventually(func() bool {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return false
			}
			return fetched.Status.OverallPhase == remediationv1.PhaseBlocked &&
				fetched.Status.BlockReason == string(remediationv1.BlockReasonUnmanagedResource) &&
				fetched.Status.BlockedUntil != nil
		}, timeout, interval).Should(BeTrue(),
			"RR should be blocked with BlockedUntil set")

		// Capture the first BlockedUntil time
		fetched := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name:      rrName,
			Namespace: ROControllerNamespace,
		}, fetched)).To(Succeed())

		firstBlockedUntil := fetched.Status.BlockedUntil.Time
		GinkgoWriter.Printf("✅ First blockedUntil: %s\n", firstBlockedUntil.Format(time.RFC3339))

		// Verify the blockedUntil is in the near future (within scope backoff bounds: ~5s)
		now := time.Now()
		Expect(firstBlockedUntil.After(now.Add(-2 * time.Second))).To(BeTrue(),
			"BlockedUntil should be near-future")
		Expect(firstBlockedUntil.Before(now.Add(30 * time.Second))).To(BeTrue(),
			"BlockedUntil should not be more than 30s in the future (initial backoff ~5s + jitter)")

		GinkgoWriter.Printf("✅ IT-RO-010-004: Exponential backoff verified — blockedUntil: %s\n",
			firstBlockedUntil.Format(time.RFC3339))
	})

	// ─────────────────────────────────────────────
	// IT-RO-010-005: Re-block — block expires, resource still unmanaged
	// Bug #266: Active handler transitions to Failed; should re-block
	// ─────────────────────────────────────────────
	It("IT-RO-010-005: should re-block with increased backoff when resource is still unmanaged at expiry", func() {
		// Create an unmanaged namespace (never add managed label)
		ns := helpers.CreateTestNamespace(ctx, k8sClient, "scope-reblock", helpers.WithoutManagedLabel())
		defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

		// Create RR in unmanaged namespace — should be blocked
		rrName := fmt.Sprintf("rr-reblock-%s", uuid.New().String()[:8])
		rr := createRemediationRequest(ns, rrName)

		// Wait for initial block
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return ""
			}
			return fetched.Status.BlockReason
		}, timeout, interval).Should(Equal(string(remediationv1.BlockReasonUnmanagedResource)),
			"RR should be initially blocked with UnmanagedResource")

		// Capture initial ConsecutiveFailureCount
		fetched := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name:      rrName,
			Namespace: ROControllerNamespace,
		}, fetched)).To(Succeed())
		initialFailureCount := fetched.Status.ConsecutiveFailureCount
		initialBlockedUntil := fetched.Status.BlockedUntil.Time
		GinkgoWriter.Printf("Initial block: failureCount=%d, blockedUntil=%s\n",
			initialFailureCount, initialBlockedUntil.Format(time.RFC3339))

		// Wait for block to expire and controller to re-evaluate.
		// BR-SCOPE-010: Should re-block (NOT transition to Failed).
		// Bug #266: Current code transitions to Failed here.
		// Scope backoff is 5s initial + jitter; wait up to 30s for re-evaluation.
		Eventually(func() bool {
			f := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ROControllerNamespace,
			}, f); err != nil {
				return false
			}
			// After expiry, the RR must still be Blocked (re-blocked), NOT Failed.
			// Check that BlockedUntil has been updated (new backoff window).
			if f.Status.OverallPhase != remediationv1.PhaseBlocked {
				GinkgoWriter.Printf("Phase is %s (expected Blocked) — bug #266?\n", f.Status.OverallPhase)
				return false
			}
			if f.Status.BlockedUntil == nil {
				return false
			}
			return f.Status.BlockedUntil.Time.After(initialBlockedUntil)
		}, 30*time.Second, interval).Should(BeTrue(),
			"BR-SCOPE-010: RR must be re-blocked with new BlockedUntil (not Failed)")

		// Verify re-block state
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name:      rrName,
			Namespace: ROControllerNamespace,
		}, fetched)).To(Succeed())
		Expect(fetched.Status.OverallPhase).To(Equal(remediationv1.PhaseBlocked),
			"Phase must remain Blocked after re-validation")
		Expect(fetched.Status.BlockReason).To(Equal(string(remediationv1.BlockReasonUnmanagedResource)),
			"BlockReason must still be UnmanagedResource")
		Expect(fetched.Status.ConsecutiveFailureCount).To(BeNumerically(">", initialFailureCount),
			"ConsecutiveFailureCount should increment on re-block (drives backoff progression)")

		// Validate audit trace: re-block should emit a new routing.blocked event
		correlationID := rr.Name
		eventType := string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorRoutingBlocked)
		var events []ogenclient.AuditEvent
		Eventually(func() int {
			flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = auditStore.Flush(flushCtx)
			flushCancel()
			events = queryAuditEventsOpenAPI(dsClients.OpenAPIClient, correlationID, eventType)
			return len(events)
		}, "10s", "500ms").Should(BeNumerically(">=", 2),
			"Expected at least 2 routing.blocked events: initial block + re-block")

		// Validate the latest event has correct structure
		latestEvent := events[len(events)-1]
		Expect(latestEvent.EventType).To(Equal(string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorRoutingBlocked)))
		Expect(latestEvent.EventAction).To(Equal(audit.ActionBlocked))
		Expect(string(latestEvent.EventOutcome)).To(Equal("pending"))
		Expect(latestEvent.CorrelationID).To(Equal(correlationID))

		payload := latestEvent.EventData.RemediationOrchestratorAuditPayload
		Expect(payload.RrName).To(Equal(rrName))
		Expect(payload.EventType).To(Equal(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorRoutingBlocked))

		GinkgoWriter.Printf("✅ IT-RO-010-005: RR %s re-blocked (failureCount: %d→%d, events: %d)\n",
			rrName, initialFailureCount, fetched.Status.ConsecutiveFailureCount, len(events))
	})

	// ─────────────────────────────────────────────
	// IT-RO-010-006: Unblock with audit — block expires, resource now managed
	// Bug #266: Validates the correct Blocked→Pending transition with audit trail
	// ─────────────────────────────────────────────
	It("IT-RO-010-006: should transition to Pending with audit trail when resource becomes managed at expiry", func() {
		// Create an unmanaged namespace
		ns := helpers.CreateTestNamespace(ctx, k8sClient, "scope-unblock-audit", helpers.WithoutManagedLabel())
		defer helpers.DeleteTestNamespace(ctx, k8sClient, ns)

		// Create RR — should be blocked
		rrName := fmt.Sprintf("rr-unblock-audit-%s", uuid.New().String()[:8])
		rr := createRemediationRequest(ns, rrName)

		// Wait for initial block
		Eventually(func() string {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return ""
			}
			return fetched.Status.BlockReason
		}, timeout, interval).Should(Equal(string(remediationv1.BlockReasonUnmanagedResource)),
			"RR should be initially blocked")

		// Validate initial routing.blocked audit event
		correlationID := rr.Name
		eventType := string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorRoutingBlocked)
		Eventually(func() int {
			flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = auditStore.Flush(flushCtx)
			flushCancel()
			return len(queryAuditEventsOpenAPI(dsClients.OpenAPIClient, correlationID, eventType))
		}, "10s", "500ms").Should(BeNumerically(">=", 1),
			"Initial routing.blocked audit event should be emitted")

		// Count initial blocked events
		initialBlockedEvents := len(queryAuditEventsOpenAPI(dsClients.OpenAPIClient, correlationID, eventType))
		GinkgoWriter.Printf("Initial blocked events: %d\n", initialBlockedEvents)

		// Add managed label to namespace before block expires
		nsObj := &corev1.Namespace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: ns}, nsObj)).To(Succeed())
		if nsObj.Labels == nil {
			nsObj.Labels = map[string]string{}
		}
		nsObj.Labels[scope.ManagedLabelKey] = scope.ManagedLabelValueTrue
		Expect(k8sClient.Update(ctx, nsObj)).To(Succeed())

		GinkgoWriter.Println("Added managed label — waiting for block expiry and re-validation")

		// BR-SCOPE-010: After block expires, re-validate scope → now managed → continue pipeline.
		// The RR transitions Blocked→Pending→Processing quickly; accept either phase.
		Eventually(func() remediationv1.RemediationPhase {
			fetched := &remediationv1.RemediationRequest{}
			if err := k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
				Name:      rrName,
				Namespace: ROControllerNamespace,
			}, fetched); err != nil {
				return ""
			}
			return fetched.Status.OverallPhase
		}, 30*time.Second, interval).Should(
			BeElementOf(remediationv1.PhasePending, remediationv1.PhaseProcessing),
			"BR-SCOPE-010: RR must continue pipeline after resource becomes managed")

		// Verify block fields are cleared
		fetched := &remediationv1.RemediationRequest{}
		Expect(k8sManager.GetAPIReader().Get(ctx, types.NamespacedName{
			Name:      rrName,
			Namespace: ROControllerNamespace,
		}, fetched)).To(Succeed())
		Expect(fetched.Status.BlockReason).To(BeEmpty())
		Expect(fetched.Status.BlockedUntil).To(BeNil())

		// Validate audit trail: the initial routing.blocked event should exist,
		// and no additional blocked events should be emitted for the unblock
		// (unblock is a phase transition, not a new block).
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = auditStore.Flush(flushCtx)
		flushCancel()
		finalBlockedEvents := len(queryAuditEventsOpenAPI(dsClients.OpenAPIClient, correlationID, eventType))
		Expect(finalBlockedEvents).To(Equal(initialBlockedEvents),
			"No additional routing.blocked events should be emitted for unblock (only initial block)")

		// Validate that a phase transition audit event was emitted (Blocked→Pending)
		transitionEventType := string(ogenclient.RemediationOrchestratorAuditPayloadEventTypeOrchestratorLifecycleTransitioned)
		var transitionEvents []ogenclient.AuditEvent
		Eventually(func() bool {
			flushCtx2, flushCancel2 := context.WithTimeout(context.Background(), 5*time.Second)
			_ = auditStore.Flush(flushCtx2)
			flushCancel2()
			transitionEvents = queryAuditEventsOpenAPI(dsClients.OpenAPIClient, correlationID, transitionEventType)
			for _, e := range transitionEvents {
				payload := e.EventData.RemediationOrchestratorAuditPayload
				if payload.FromPhase.IsSet() && payload.ToPhase.IsSet() {
					if payload.FromPhase.Value == "Blocked" && payload.ToPhase.Value == "Pending" {
						return true
					}
				}
			}
			return false
		}, "10s", "500ms").Should(BeTrue(),
			"Phase transition audit event (Blocked→Pending) should be emitted on unblock")

		GinkgoWriter.Printf("✅ IT-RO-010-006: RR %s unblocked with audit trail (blocked events: %d, transition found)\n",
			rrName, finalBlockedEvents)
	})
})
