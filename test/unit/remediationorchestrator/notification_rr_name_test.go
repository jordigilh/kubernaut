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

var _ = Describe("Issue #626: RemediationRequest Name in Notification Bodies", func() {
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

	Describe("UT-RO-626-001/002: FormatRemediationLine", func() {
		It("UT-RO-626-001: should format RR name when non-empty", func() {
			line := creator.FormatRemediationLine("rr-abc123")
			Expect(line).To(Equal("**Remediation**: rr-abc123\n\n"))
		})

		It("UT-RO-626-002: should return empty string when name is empty", func() {
			line := creator.FormatRemediationLine("")
			Expect(line).To(BeEmpty())
		})
	})

	Describe("UT-RO-626-003..007: Body builders include RR name", func() {
		It("UT-RO-626-003: Approval notification body contains RR name", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-626-003", "default")
			ai := helpers.NewCompletedAIAnalysis("test-ai-626-003", "default")

			name, err := nc.CreateApprovalNotification(context.Background(), rr, ai)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())
			Expect(nr.Spec.Body).To(ContainSubstring("**Remediation**: test-rr-626-003"))
		})

		It("UT-RO-626-004: Completion notification body contains RR name", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-626-004", "default")
			rr.Status.OverallPhase = "Completed"
			rr.Status.Outcome = "Succeeded"
			ai := helpers.NewCompletedAIAnalysis("test-ai-626-004", "default")

			name, err := nc.CreateCompletionNotification(context.Background(), rr, ai, "argo", nil)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())
			Expect(nr.Spec.Body).To(ContainSubstring("**Remediation**: test-rr-626-004"))
		})

		It("UT-RO-626-005: Bulk duplicate notification body contains RR name", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-626-005", "default")
			rr.Status.OverallPhase = "Completed"
			rr.Status.DuplicateCount = 3

			name, err := nc.CreateBulkDuplicateNotification(context.Background(), rr)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())
			Expect(nr.Spec.Body).To(ContainSubstring("**Remediation**: test-rr-626-005"))
		})

		It("UT-RO-626-006: Manual review notification body contains RR name", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-626-006", "default")
			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceAIAnalysis,
				Reason:  "LowConfidence",
				Message: "Confidence below threshold",
			}

			name, err := nc.CreateManualReviewNotification(context.Background(), rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())
			Expect(nr.Spec.Body).To(ContainSubstring("**Remediation**: test-rr-626-006"))
		})

		It("UT-RO-626-007: Self-resolved notification body contains RR name", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-626-007", "default")
			ai := helpers.NewCompletedAIAnalysis("test-ai-626-007", "default")

			name, err := nc.CreateSelfResolvedNotification(context.Background(), rr, ai)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())
			Expect(nr.Spec.Body).To(ContainSubstring("**Remediation**: test-rr-626-007"))
		})
	})
})
