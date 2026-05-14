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

package controller

import (
	"context"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheus "github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	prodaudit "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/audit"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
)

var _ = Describe("Issue #1033 Gap 1: VerifyingHandler completion audit outcome (BR-AUDIT-005)", func() {

	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(eav1.AddToScheme(scheme)).To(Succeed())
	})

	newHandler := func(c client.Client, cbs prodcontroller.VerifyingCallbacks) *prodcontroller.VerifyingHandler {
		m := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		return prodcontroller.NewVerifyingHandler(
			c, m, prodcontroller.TimeoutConfig{Verifying: 10 * time.Minute}, cbs,
		)
	}

	// ========================================
	// P0: EmitCompletionAudit receives rr.Status.Outcome, not "success"
	// ========================================
	Describe("EmitCompletionAudit outcome argument (P0)", func() {

		It("UT-RO-1033-001: EA terminal completion passes rr.Status.Outcome to EmitCompletionAudit", func() {
			rr := newRemediationRequest("ver-outcome-ea", "default", remediationv1.PhaseVerifying)
			rr.Status.Outcome = "Remediated"
			startTime := metav1.NewTime(time.Now().Add(-5 * time.Minute))
			rr.Status.StartTime = &startTime
			rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
				Kind: "EffectivenessAssessment", Name: "ea-ver-outcome-ea", Namespace: "default",
			}
			dl := metav1.NewTime(time.Now().Add(10 * time.Minute))
			rr.Status.VerificationDeadline = &dl
			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-ver-outcome-ea", Namespace: "default"},
				Status:     eav1.EffectivenessAssessmentStatus{Phase: eav1.PhasePending},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			var capturedOutcome string
			cbs := noopCallbacks()
			cbs.TrackEffectivenessStatus = func(_ context.Context, rr *remediationv1.RemediationRequest) error {
				rr.Status.OverallPhase = phase.Completed
				rr.Status.Outcome = "Remediated"
				return nil
			}
			cbs.EmitCompletionAudit = func(_ context.Context, _ *remediationv1.RemediationRequest, outcome string, _ int64) {
				capturedOutcome = outcome
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedOutcome).To(Equal("Remediated"),
				"EmitCompletionAudit should receive rr.Status.Outcome, not 'success'")
		})

		It("UT-RO-1033-002: Safety-net timeout passes VerificationTimedOut to EmitCompletionAudit", func() {
			rr := newRemediationRequest("ver-outcome-safety", "default", remediationv1.PhaseVerifying)
			rr.CreationTimestamp = metav1.NewTime(time.Now().Add(-15 * time.Minute))
			rr.Status.Outcome = "Remediated"
			startTime := metav1.NewTime(time.Now().Add(-15 * time.Minute))
			rr.Status.StartTime = &startTime
			rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
				Kind: "EffectivenessAssessment", Name: "ea-ver-outcome-safety", Namespace: "default",
			}

			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-ver-outcome-safety", Namespace: "default"},
				Status:     eav1.EffectivenessAssessmentStatus{Phase: eav1.PhasePending},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			var capturedOutcome string
			cbs := noopCallbacks()
			cbs.EmitCompletionAudit = func(_ context.Context, _ *remediationv1.RemediationRequest, outcome string, _ int64) {
				capturedOutcome = outcome
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedOutcome).To(Equal("VerificationTimedOut"),
				"Safety-net timeout should pass VerificationTimedOut, not 'success'")
		})

		It("UT-RO-1033-003: Verification deadline expired passes VerificationTimedOut to EmitCompletionAudit", func() {
			rr := newRemediationRequest("ver-outcome-dl", "default", remediationv1.PhaseVerifying)
			rr.Status.Outcome = "Remediated"
			startTime := metav1.NewTime(time.Now().Add(-10 * time.Minute))
			rr.Status.StartTime = &startTime
			rr.Status.EffectivenessAssessmentRef = &corev1.ObjectReference{
				Kind: "EffectivenessAssessment", Name: "ea-ver-outcome-dl", Namespace: "default",
			}
			dl := metav1.NewTime(time.Now().Add(-1 * time.Minute))
			rr.Status.VerificationDeadline = &dl

			ea := &eav1.EffectivenessAssessment{
				ObjectMeta: metav1.ObjectMeta{Name: "ea-ver-outcome-dl", Namespace: "default"},
				Status:     eav1.EffectivenessAssessmentStatus{Phase: eav1.PhasePending},
			}

			c := fake.NewClientBuilder().WithScheme(scheme).
				WithObjects(rr, ea).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			var capturedOutcome string
			cbs := noopCallbacks()
			cbs.EmitCompletionAudit = func(_ context.Context, _ *remediationv1.RemediationRequest, outcome string, _ int64) {
				capturedOutcome = outcome
			}

			h := newHandler(c, cbs)
			_, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(capturedOutcome).To(Equal("VerificationTimedOut"),
				"Verification deadline expiry should pass VerificationTimedOut, not 'success'")
		})
	})

	// ========================================
	// P0: BuildCompletionEvent maps all 5 CRD outcomes to crd_outcome correctly
	// ========================================
	Describe("BuildCompletionEvent crd_outcome mapping (P0)", func() {

		It("UT-RO-1033-004: table-driven — all 5 RR outcome values produce correct crd_outcome", func() {
			mgr := prodaudit.NewManager("test-orchestrator")

			outcomes := []string{
				"Remediated",
				"Inconclusive",
				"VerificationTimedOut",
				"DryRun",
				"ManualReviewRequired",
			}

			for _, outcome := range outcomes {
				event, err := mgr.BuildCompletionEvent(
					"corr-"+outcome,
					"default",
					"rr-"+outcome,
					outcome,
					5000,
				)
				Expect(err).ToNot(HaveOccurred(), "BuildCompletionEvent should not fail for outcome %s", outcome)

				payload, ok := event.EventData.GetRemediationOrchestratorAuditPayload()
				Expect(ok).To(BeTrue(), "event_data should be orchestrator.lifecycle.completed for outcome %s", outcome)
				Expect(payload.CrdOutcome.IsSet()).To(BeTrue(), "crd_outcome should be set for outcome %s", outcome)
				Expect(payload.CrdOutcome.Value).To(Equal(outcome),
					"crd_outcome should equal the CRD outcome value for %s", outcome)
			}
		})
	})

	// ========================================
	// P1: Backward compatibility — OpenAPI outcome always "Success"
	// ========================================
	Describe("Backward compatibility (P1)", func() {

		It("UT-RO-1033-005: OpenAPI outcome field is always Success regardless of crd_outcome", func() {
			mgr := prodaudit.NewManager("test-orchestrator")

			event, err := mgr.BuildCompletionEvent(
				"corr-compat",
				"default",
				"rr-compat",
				"VerificationTimedOut",
				5000,
			)
			Expect(err).ToNot(HaveOccurred())

			payload, ok := event.EventData.GetRemediationOrchestratorAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.Outcome.IsSet()).To(BeTrue())
			Expect(payload.Outcome.Value).To(Equal(
				ogenclient.RemediationOrchestratorAuditPayloadOutcomeSuccess),
				"OpenAPI-level outcome must remain 'Success' for all completion events (backward compat)")
			Expect(payload.CrdOutcome.Value).To(Equal("VerificationTimedOut"),
				"crd_outcome should carry the CRD vocabulary")
		})
	})

	// ========================================
	// P1: Nil/zero edge — empty outcome
	// ========================================
	Describe("Nil/zero edge cases (P1)", func() {

		It("UT-RO-1033-006: empty rr.Status.Outcome → crd_outcome is unset (OptString.Set=false)", func() {
			mgr := prodaudit.NewManager("test-orchestrator")

			event, err := mgr.BuildCompletionEvent(
				"corr-empty",
				"default",
				"rr-empty",
				"",
				5000,
			)
			Expect(err).ToNot(HaveOccurred())

			payload, ok := event.EventData.GetRemediationOrchestratorAuditPayload()
			Expect(ok).To(BeTrue())
			Expect(payload.CrdOutcome.IsSet()).To(BeFalse(),
				"crd_outcome should NOT be set when outcome is empty string")
		})
	})

	// ========================================
	// P1: Concurrency — BuildCompletionEvent under -race
	// ========================================
	Describe("Concurrency (P1)", func() {

		It("UT-RO-1033-007: 10 goroutines calling BuildCompletionEvent concurrently under --race", func() {
			mgr := prodaudit.NewManager("test-orchestrator")

			var wg sync.WaitGroup
			outcomes := []string{
				"Remediated", "Inconclusive", "VerificationTimedOut",
				"DryRun", "ManualReviewRequired",
				"Remediated", "Inconclusive", "VerificationTimedOut",
				"DryRun", "ManualReviewRequired",
			}

			for i, outcome := range outcomes {
				wg.Add(1)
				go func(idx int, o string) {
					defer GinkgoRecover()
					defer wg.Done()
					event, err := mgr.BuildCompletionEvent(
						"corr-concurrent",
						"default",
						"rr-concurrent",
						o,
						int64(idx*1000),
					)
					Expect(err).ToNot(HaveOccurred())
					Expect(event.Version).To(Equal("1.0"))
				}(i, outcome)
			}
			wg.Wait()
		})
	})

	// ========================================
	// P2: Adversarial inputs
	// ========================================
	Describe("Adversarial inputs (P2)", func() {

		It("UT-RO-1033-008: adversarial outcome strings do not panic and produce correct set/unset behavior", func() {
			mgr := prodaudit.NewManager("test-orchestrator")

			cases := []struct {
				name      string
				outcome   string
				expectSet bool
			}{
				{name: "empty string", outcome: "", expectSet: false},
				{name: "max-length+1", outcome: strings.Repeat("x", 1024), expectSet: true},
				{name: "path traversal", outcome: "../../etc/passwd", expectSet: true},
				{name: "null bytes", outcome: "Remediated\x00Injected", expectSet: true},
				{name: "unicode edge", outcome: "\uffff\u0000\u200b", expectSet: true},
				{name: "very long", outcome: strings.Repeat("a", 65536), expectSet: true},
			}

			for _, tc := range cases {
				event, err := mgr.BuildCompletionEvent(
					"corr-adversarial",
					"default",
					"rr-adversarial",
					tc.outcome,
					5000,
				)
				Expect(err).ToNot(HaveOccurred(), "BuildCompletionEvent should not panic for %s", tc.name)

				payload, ok := event.EventData.GetRemediationOrchestratorAuditPayload()
				Expect(ok).To(BeTrue(), "payload should be extractable for %s", tc.name)
				Expect(payload.CrdOutcome.IsSet()).To(Equal(tc.expectSet),
					"crd_outcome set/unset should match expected for %s", tc.name)
				if tc.expectSet {
					Expect(payload.CrdOutcome.Value).To(Equal(tc.outcome),
						"crd_outcome value should be passthrough for %s", tc.name)
				}
			}
		})
	})
})
