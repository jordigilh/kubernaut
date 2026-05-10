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

package remediationorchestrator

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	eav1 "github.com/jordigilh/kubernaut/api/effectivenessassessment/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
	"github.com/prometheus/client_golang/prometheus"
)

var _ = Describe("Fix 8: RO Notification Alignment Verdict Rendering", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		_ = remediationv1.AddToScheme(scheme)
		_ = notificationv1.AddToScheme(scheme)
		_ = aianalysisv1.AddToScheme(scheme)
		_ = eav1.AddToScheme(scheme)
	})

	// UT-RO-NOTIF-001: Circuit breaker activated — body contains "SUSPICIOUS (Circuit Breaker Activated)"
	It("UT-RO-NOTIF-001: buildManualReviewBody renders circuit breaker verdict prominently", func() {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		rr := helpers.NewRemediationRequest("test-rr-notif-001", "default")
		reviewCtx := &creator.ManualReviewContext{
			Source:            notificationv1.ReviewSourceAIAnalysis,
			Reason:            "WorkflowResolutionFailed",
			SubReason:         "alignment_check_failed",
			HumanReviewReason: "alignment_check_failed",
			RootCauseAnalysis: "Partial RCA before circuit break",
			AlignmentVerdict: &aianalysisv1.AlignmentVerdictStatus{
				Result:                  "suspicious",
				CircuitBreakerActivated: true,
				Summary:                 "Shadow agent detected suspicious tool execution pattern",
				Flagged:                 2,
				Total:                   5,
				Findings: []aianalysisv1.AlignmentFindingStatus{
					{StepIndex: 1, StepKind: "tool_result", Tool: "kubectl_get", Explanation: "Unexpected data access pattern"},
					{StepIndex: 3, StepKind: "llm_reasoning", Explanation: "Model attempted to access restricted resources"},
				},
			},
		}

		name, err := nc.CreateManualReviewNotification(context.Background(), rr, reviewCtx)
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

		body := nr.Spec.Body
		Expect(body).To(ContainSubstring("SUSPICIOUS"), "Verdict must show SUSPICIOUS")
		Expect(body).To(ContainSubstring("Circuit Breaker"), "Circuit breaker activation must be mentioned")
		Expect(body).To(ContainSubstring("Shadow Agent"), "Shadow agent attribution must be present")
		Expect(body).To(ContainSubstring("Unexpected data access pattern"), "Findings must be rendered")

		// Shadow findings must appear BEFORE RCA section
		shadowIdx := strings.Index(body, "Shadow Agent")
		rcaIdx := strings.Index(body, "Root Cause Analysis")
		if rcaIdx >= 0 {
			Expect(shadowIdx).To(BeNumerically("<", rcaIdx),
				"Shadow agent findings must appear before RCA section")
		}
	})

	// UT-RO-NOTIF-002: Aligned verdict — short positive confirmation
	It("UT-RO-NOTIF-002: buildManualReviewBody renders aligned verdict as short confirmation", func() {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		rr := helpers.NewRemediationRequest("test-rr-notif-002", "default")
		reviewCtx := &creator.ManualReviewContext{
			Source:            notificationv1.ReviewSourceAIAnalysis,
			Reason:            "WorkflowResolutionFailed",
			SubReason:         "LowConfidence",
			RootCauseAnalysis: "Normal RCA",
			AlignmentVerdict: &aianalysisv1.AlignmentVerdictStatus{
				Result:  "clean",
				Flagged: 0,
				Total:   5,
			},
		}

		name, err := nc.CreateManualReviewNotification(context.Background(), rr, reviewCtx)
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

		body := nr.Spec.Body
		Expect(body).To(ContainSubstring("ALIGNED"), "Aligned verdict must show ALIGNED")
		Expect(body).NotTo(ContainSubstring("Findings"), "Aligned verdict must not show findings section")
	})

	// UT-RO-NOTIF-003: AlignmentVerdict=nil — no alignment section (backward compat)
	It("UT-RO-NOTIF-003: buildManualReviewBody with nil AlignmentVerdict has no alignment section", func() {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		rr := helpers.NewRemediationRequest("test-rr-notif-003", "default")
		reviewCtx := &creator.ManualReviewContext{
			Source:            notificationv1.ReviewSourceAIAnalysis,
			Reason:            "WorkflowResolutionFailed",
			SubReason:         "WorkflowNotFound",
			RootCauseAnalysis: "Normal RCA",
		}

		name, err := nc.CreateManualReviewNotification(context.Background(), rr, reviewCtx)
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

		body := nr.Spec.Body
		Expect(body).NotTo(ContainSubstring("Shadow Agent Alignment Verdict"),
			"No alignment section when AlignmentVerdict is nil")
	})

	// UT-RO-NOTIF-004: mapManualReviewPriority with alignment_check_failed -> Critical
	It("UT-RO-NOTIF-004: alignment_check_failed maps to Critical priority", func() {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		rr := helpers.NewRemediationRequest("test-rr-notif-004", "default")
		reviewCtx := &creator.ManualReviewContext{
			Source:            notificationv1.ReviewSourceAIAnalysis,
			Reason:            "WorkflowResolutionFailed",
			SubReason:         "alignment_check_failed",
			HumanReviewReason: "alignment_check_failed",
			AlignmentVerdict: &aianalysisv1.AlignmentVerdictStatus{
				Result:                  "suspicious",
				CircuitBreakerActivated: true,
				Summary:                 "Suspicious",
				Flagged:                 1,
				Total:                   3,
			},
		}

		name, err := nc.CreateManualReviewNotification(context.Background(), rr, reviewCtx)
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

		Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityCritical),
			"alignment_check_failed must escalate to Critical priority")
	})

	// UT-RO-NOTIF-005: populateManualReviewContext with AlignmentVerdict present
	// (tested indirectly through AIAnalysisHandler integration — here we verify the field exists)
	It("UT-RO-NOTIF-005: ManualReviewContext accepts AlignmentVerdict", func() {
		verdict := &aianalysisv1.AlignmentVerdictStatus{
			Result:                  "suspicious",
			CircuitBreakerActivated: true,
			Summary:                 "test",
			Flagged:                 1,
			Total:                   3,
		}
		ctx := &creator.ManualReviewContext{
			AlignmentVerdict: verdict,
		}
		Expect(ctx.AlignmentVerdict).NotTo(BeNil())
		Expect(ctx.AlignmentVerdict.Result).To(Equal("suspicious"))
		Expect(ctx.AlignmentVerdict.CircuitBreakerActivated).To(BeTrue())
	})

	// UT-RO-NOTIF-006: buildManualReviewContext maps AlignmentVerdict to ReviewContext CRD fields
	It("UT-RO-NOTIF-006: buildManualReviewContext maps alignment verdict to ReviewContext", func() {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		rr := helpers.NewRemediationRequest("test-rr-notif-006", "default")
		reviewCtx := &creator.ManualReviewContext{
			Source:            notificationv1.ReviewSourceAIAnalysis,
			Reason:            "WorkflowResolutionFailed",
			SubReason:         "alignment_check_failed",
			HumanReviewReason: "alignment_check_failed",
			AlignmentVerdict: &aianalysisv1.AlignmentVerdictStatus{
				Result:                  "suspicious",
				CircuitBreakerActivated: true,
				Summary:                 "Suspicious tool execution",
				Flagged:                 2,
				Total:                   5,
			},
		}

		name, err := nc.CreateManualReviewNotification(context.Background(), rr, reviewCtx)
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

		Expect(nr.Spec.Context).NotTo(BeNil())
		Expect(nr.Spec.Context.Review).NotTo(BeNil())
		Expect(nr.Spec.Context.Review.AlignmentVerdict).To(Equal("suspicious"),
			"ReviewContext must contain alignment verdict result")
		Expect(nr.Spec.Context.Review.CircuitBreakerActivated).To(BeTrue(),
			"ReviewContext must contain circuit breaker flag")
	})
})
