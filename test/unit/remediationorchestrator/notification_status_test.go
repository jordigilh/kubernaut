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

var _ = Describe("Issue #628: Notification Status Standardization", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(eav1.AddToScheme(scheme)).To(Succeed())
	})

	Context("Per-type Status field presence", func() {
		It("UT-NOT-628-001: Completion body contains **Status**: with mapped outcome value", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-628-001", "default")
			rr.Status.OverallPhase = "Completed"
			rr.Status.Outcome = "Remediated"
			ai := helpers.NewCompletedAIAnalysis("test-ai-628-001", "default")

			name, err := nc.CreateCompletionNotification(context.Background(), rr, ai, "tekton", nil)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

			Expect(nr.Spec.Body).To(ContainSubstring("**Status**: Remediated"),
				"#628: Completion body must include standardized Status field with mapped outcome")
			Expect(nr.Spec.Body).To(ContainSubstring("**Outcome**:"),
				"#628: Legacy Outcome field must remain for backward compatibility (one-release deprecation)")
		})

		It("UT-NOT-628-002: Bulk duplicate body contains **Status**: Duplicate Handled and legacy **Result**:", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-628-002", "default")
			rr.Status.OverallPhase = "Completed"
			rr.Status.DuplicateCount = 3

			name, err := nc.CreateBulkDuplicateNotification(context.Background(), rr)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

			Expect(nr.Spec.Body).To(ContainSubstring("**Status**: Duplicate Handled"),
				"#628: Bulk duplicate body must include standardized Status field")
			Expect(nr.Spec.Body).To(ContainSubstring("**Result**:"),
				"#628: Legacy Result field must remain for backward compatibility")
		})

		It("UT-NOT-628-003: Manual review body contains **Status**: Manual Review Required", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-628-003", "default")
			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceAIAnalysis,
				Reason:  "WorkflowResolutionFailed",
				Message: "No matching workflow found",
			}

			name, err := nc.CreateManualReviewNotification(context.Background(), rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

			Expect(nr.Spec.Body).To(ContainSubstring("**Status**: Manual Review Required"),
				"#628: Manual review body must include standardized Status field")
		})

		It("UT-NOT-628-004: Approval body contains **Status**: Pending Approval", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-628-004", "default")
			ai := helpers.NewCompletedAIAnalysis("test-ai-628-004", "default")

			name, err := nc.CreateApprovalNotification(context.Background(), rr, ai)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

			Expect(nr.Spec.Body).To(ContainSubstring("**Status**: Pending Approval"),
				"#628: Approval body must include standardized Status field")
		})

		It("UT-NOT-628-005: Self-resolved body contains **Status**: Self-Resolved", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-628-005", "default")
			ai := helpers.NewCompletedAIAnalysis("test-ai-628-005", "default")

			name, err := nc.CreateSelfResolvedNotification(context.Background(), rr, ai)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

			Expect(nr.Spec.Body).To(ContainSubstring("**Status**: Self-Resolved"),
				"#628: Self-resolved body must include standardized Status field")
		})

		It("UT-NOT-628-006: Global timeout body contains **Status**: Timed Out distinct from timestamp line", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			body := nc.BuildGlobalTimeoutBody(
				"HighCPUAlert", "test-rr-628-006", "Executing",
				"30m", "2026-04-09T10:00:00Z", "2026-04-09T10:30:00Z",
			)

			Expect(body).To(ContainSubstring("**Status**: Timed Out"),
				"#628: Global timeout body must include standardized Status field")
			Expect(body).To(ContainSubstring("**Timed Out**:"),
				"#628: Legacy timestamp label must remain")

			statusLine := ""
			timedOutLine := ""
			for _, line := range strings.Split(body, "\n") {
				if strings.Contains(line, "**Status**:") {
					statusLine = line
				}
				if strings.Contains(line, "**Timed Out**:") {
					timedOutLine = line
				}
			}
			Expect(statusLine).ToNot(Equal(timedOutLine),
				"#628 R1: Status line and timestamp line must be structurally distinct")
		})

		It("UT-NOT-628-007: Phase timeout body contains **Status**: Timed Out", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			body := nc.BuildPhaseTimeoutBody(
				"HighMemAlert", "test-rr-628-007", "AIAnalysis",
				"15m", "2026-04-09T11:00:00Z", "2026-04-09T11:15:00Z",
			)

			Expect(body).To(ContainSubstring("**Status**: Timed Out"),
				"#628: Phase timeout body must include standardized Status field")
		})
	})

	Context("Status position ordering", func() {
		It("UT-NOT-628-008: Status appears before Signal in all notification types that include Signal", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			type bodyCase struct {
				label string
				body  string
			}

			rr := helpers.NewRemediationRequest("test-rr-628-008", "default")
			rr.Status.OverallPhase = "Completed"
			rr.Status.Outcome = "Remediated"
			rr.Status.DuplicateCount = 2
			ai := helpers.NewCompletedAIAnalysis("test-ai-628-008", "default")

			completionName, err := nc.CreateCompletionNotification(context.Background(), rr, ai, "tekton", nil)
			Expect(err).ToNot(HaveOccurred())
			completionNR := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: completionName, Namespace: "default"}, completionNR)).To(Succeed())

			bulkName, err := nc.CreateBulkDuplicateNotification(context.Background(), rr)
			Expect(err).ToNot(HaveOccurred())
			bulkNR := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: bulkName, Namespace: "default"}, bulkNR)).To(Succeed())

			reviewCtx := &creator.ManualReviewContext{
				Source: notificationv1.ReviewSourceAIAnalysis,
				Reason: "LowConfidence",
			}
			reviewRR := helpers.NewRemediationRequest("test-rr-628-008-review", "default")
			reviewName, err := nc.CreateManualReviewNotification(context.Background(), reviewRR, reviewCtx)
			Expect(err).ToNot(HaveOccurred())
			reviewNR := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: reviewName, Namespace: "default"}, reviewNR)).To(Succeed())

			approvalRR := helpers.NewRemediationRequest("test-rr-628-008-approval", "default")
			approvalName, err := nc.CreateApprovalNotification(context.Background(), approvalRR, ai)
			Expect(err).ToNot(HaveOccurred())
			approvalNR := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: approvalName, Namespace: "default"}, approvalNR)).To(Succeed())

			selfRR := helpers.NewRemediationRequest("test-rr-628-008-self", "default")
			selfName, err := nc.CreateSelfResolvedNotification(context.Background(), selfRR, ai)
			Expect(err).ToNot(HaveOccurred())
			selfNR := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: selfName, Namespace: "default"}, selfNR)).To(Succeed())

			cases := []bodyCase{
				{label: "Completion", body: completionNR.Spec.Body},
				{label: "Bulk Duplicate", body: bulkNR.Spec.Body},
				{label: "Manual Review", body: reviewNR.Spec.Body},
				{label: "Approval", body: approvalNR.Spec.Body},
				{label: "Self-Resolved", body: selfNR.Spec.Body},
				{label: "Global Timeout", body: nc.BuildGlobalTimeoutBody("sig", "rr", "Executing", "30m", "t0", "t1")},
				{label: "Phase Timeout", body: nc.BuildPhaseTimeoutBody("sig", "rr", "AI", "15m", "t0", "t1")},
			}

			for _, tc := range cases {
				statusIdx := strings.Index(tc.body, "**Status**:")
				signalIdx := strings.Index(tc.body, "**Signal**:")
				Expect(statusIdx).To(BeNumerically(">=", 0),
					"%s: **Status**: must be present", tc.label)
				Expect(signalIdx).To(BeNumerically(">=", 0),
					"%s: **Signal**: must be present", tc.label)
				Expect(statusIdx).To(BeNumerically("<", signalIdx),
					"%s: **Status**: must appear before **Signal**: for consistent operator scan order (#628/#627)", tc.label)
			}
		})
	})
})
