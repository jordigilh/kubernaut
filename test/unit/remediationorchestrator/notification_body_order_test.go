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

var _ = Describe("Issue #627: Notification Body Field Reordering", func() {
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

	It("UT-RO-627-001: Completion body has Status and Outcome before Signal", func() {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		rr := helpers.NewRemediationRequest("test-rr-627-001", "default")
		rr.Status.OverallPhase = "Completed"
		rr.Status.Outcome = "Remediated"
		ai := helpers.NewCompletedAIAnalysis("test-ai-627-001", "default")

		name, err := nc.CreateCompletionNotification(context.Background(), rr, ai, "argo", nil)
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

		statusIdx := strings.Index(nr.Spec.Body, "**Status**:")
		outcomeIdx := strings.Index(nr.Spec.Body, "**Outcome**:")
		signalIdx := strings.Index(nr.Spec.Body, "**Signal**:")
		Expect(statusIdx).To(BeNumerically(">=", 0), "#628: Status field must be present")
		Expect(outcomeIdx).To(BeNumerically(">=", 0), "Outcome field must be present (deprecated, retained one release)")
		Expect(signalIdx).To(BeNumerically(">=", 0), "Signal field must be present")
		Expect(statusIdx).To(BeNumerically("<", outcomeIdx),
			"#628: Status must appear before deprecated Outcome")
		Expect(outcomeIdx).To(BeNumerically("<", signalIdx),
			"Outcome must appear before Signal for faster operator triage")
	})

	It("UT-RO-627-002: Manual Review body has Action Required before Failure Source", func() {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		rr := helpers.NewRemediationRequest("test-rr-627-002", "default")
		reviewCtx := &creator.ManualReviewContext{
			Source:  notificationv1.ReviewSourceAIAnalysis,
			Reason:  "LowConfidence",
			Message: "Confidence below threshold",
		}

		name, err := nc.CreateManualReviewNotification(context.Background(), rr, reviewCtx)
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

		actionIdx := strings.Index(nr.Spec.Body, "**Action Required**:")
		failureIdx := strings.Index(nr.Spec.Body, "**Failure Source**:")
		Expect(actionIdx).To(BeNumerically(">=", 0), "Action Required must be present")
		Expect(failureIdx).To(BeNumerically(">=", 0), "Failure Source must be present")
		Expect(actionIdx).To(BeNumerically("<", failureIdx),
			"Action Required must appear before Failure Source for faster triage")
	})

	It("UT-RO-627-003: Approval body has approve/reject before Selection Rationale", func() {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		rr := helpers.NewRemediationRequest("test-rr-627-003", "default")
		ai := helpers.NewCompletedAIAnalysis("test-ai-627-003", "default")

		name, err := nc.CreateApprovalNotification(context.Background(), rr, ai)
		Expect(err).ToNot(HaveOccurred())

		nr := &notificationv1.NotificationRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())

		approveIdx := strings.Index(nr.Spec.Body, "approve/reject")
		rationaleIdx := strings.Index(nr.Spec.Body, "**Selection Rationale**:")
		Expect(approveIdx).To(BeNumerically(">=", 0), "approve/reject prompt must be present")
		Expect(rationaleIdx).To(BeNumerically(">=", 0), "Selection Rationale must be present")
		Expect(approveIdx).To(BeNumerically("<", rationaleIdx),
			"approve/reject prompt must appear before Selection Rationale for faster triage")
	})

	It("UT-RO-627-004: Bulk Duplicate and Self-Resolved bodies contain expected fields (regression guard)", func() {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

		rr := helpers.NewRemediationRequest("test-rr-627-004", "default")
		rr.Status.OverallPhase = "Completed"
		rr.Status.DuplicateCount = 2
		ai := helpers.NewCompletedAIAnalysis("test-ai-627-004", "default")

		bulkName, err := nc.CreateBulkDuplicateNotification(context.Background(), rr)
		Expect(err).ToNot(HaveOccurred())
		bulkNR := &notificationv1.NotificationRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: bulkName, Namespace: "default"}, bulkNR)).To(Succeed())
		Expect(bulkNR.Spec.Body).To(ContainSubstring("**Signal**:"))
		Expect(bulkNR.Spec.Body).To(ContainSubstring("**Duplicate Remediations**:"))

		selfName, err := nc.CreateSelfResolvedNotification(context.Background(), rr, ai)
		Expect(err).ToNot(HaveOccurred())
		selfNR := &notificationv1.NotificationRequest{}
		Expect(cl.Get(context.Background(), types.NamespacedName{Name: selfName, Namespace: "default"}, selfNR)).To(Succeed())
		Expect(selfNR.Spec.Body).To(ContainSubstring("**Signal**:"))
		Expect(selfNR.Spec.Body).To(ContainSubstring("no remediation was needed"))
	})
})
