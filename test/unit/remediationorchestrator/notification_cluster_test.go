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

var _ = Describe("Issue #615: Cluster Identification in Notifications", func() {
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

	Describe("UT-RO-615-001..004: FormatClusterLine", func() {
		It("UT-RO-615-001: should format name and UUID", func() {
			line := creator.FormatClusterLine("ocp-prod", "abc-123")
			Expect(line).To(Equal("**Cluster**: ocp-prod (abc-123)\n\n"))
		})

		It("UT-RO-615-002: should format name only (no UUID)", func() {
			line := creator.FormatClusterLine("ocp-prod", "")
			Expect(line).To(Equal("**Cluster**: ocp-prod\n\n"))
		})

		It("UT-RO-615-003: should format UUID only (no name)", func() {
			line := creator.FormatClusterLine("", "abc-123")
			Expect(line).To(Equal("**Cluster**: (abc-123)\n\n"))
		})

		It("UT-RO-615-004: should return empty when both are empty", func() {
			line := creator.FormatClusterLine("", "")
			Expect(line).To(BeEmpty())
		})
	})

	Describe("UT-RO-615-005..009: Body builders prepend cluster line when SetClusterIdentity is called", func() {
		const (
			clusterName = "ocp-prod"
			clusterUUID = "abc-123"
			expectedPrefix = "**Cluster**: ocp-prod (abc-123)\n\n"
		)

		It("UT-RO-615-005: Approval notification body starts with cluster line", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
			nc.SetClusterIdentity(clusterName, clusterUUID)

			rr := helpers.NewRemediationRequest("test-rr-615-005", "default")
			ai := helpers.NewCompletedAIAnalysis("test-ai-615-005", "default")

			name, err := nc.CreateApprovalNotification(context.Background(), rr, ai)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())
			Expect(nr.Spec.Body).To(HavePrefix(expectedPrefix))
		})

		It("UT-RO-615-006: Completion notification body starts with cluster line", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
			nc.SetClusterIdentity(clusterName, clusterUUID)

			rr := helpers.NewRemediationRequest("test-rr-615-006", "default")
			rr.Status.OverallPhase = "Completed"
			rr.Status.Outcome = "Succeeded"
			ai := helpers.NewCompletedAIAnalysis("test-ai-615-006", "default")

			name, err := nc.CreateCompletionNotification(context.Background(), rr, ai, "argo", nil)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())
			Expect(nr.Spec.Body).To(HavePrefix(expectedPrefix))
		})

		It("UT-RO-615-007: Bulk duplicate notification body starts with cluster line", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
			nc.SetClusterIdentity(clusterName, clusterUUID)

			rr := helpers.NewRemediationRequest("test-rr-615-007", "default")
			rr.Status.OverallPhase = "Completed"
			rr.Status.DuplicateCount = 3

			name, err := nc.CreateBulkDuplicateNotification(context.Background(), rr)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())
			Expect(nr.Spec.Body).To(HavePrefix(expectedPrefix))
		})

		It("UT-RO-615-008: Manual review notification body starts with cluster line", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
			nc.SetClusterIdentity(clusterName, clusterUUID)

			rr := helpers.NewRemediationRequest("test-rr-615-008", "default")
			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceAIAnalysis,
				Reason:  "LowConfidence",
				Message: "Confidence below threshold",
			}

			name, err := nc.CreateManualReviewNotification(context.Background(), rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())
			Expect(nr.Spec.Body).To(HavePrefix(expectedPrefix))
		})

		It("UT-RO-615-009: Self-resolved notification body starts with cluster line", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
			nc.SetClusterIdentity(clusterName, clusterUUID)

			rr := helpers.NewRemediationRequest("test-rr-615-009", "default")
			ai := helpers.NewCompletedAIAnalysis("test-ai-615-009", "default")

			name, err := nc.CreateSelfResolvedNotification(context.Background(), rr, ai)
			Expect(err).ToNot(HaveOccurred())

			nr := &notificationv1.NotificationRequest{}
			Expect(cl.Get(context.Background(), types.NamespacedName{Name: name, Namespace: "default"}, nr)).To(Succeed())
			Expect(nr.Spec.Body).To(HavePrefix(expectedPrefix))
		})
	})

	Describe("UT-RO-621-001..003: Timeout body builders include cluster line", func() {
		It("UT-RO-621-001: Global timeout body includes cluster line when identity is set", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
			nc.SetClusterIdentity("ocp-prod", "uuid-123")

			body := nc.BuildGlobalTimeoutBody("TestSignal", "test-rr", "AIAnalysis", "30m0s", "2026-01-01T00:00:00Z", "2026-01-01T00:30:00Z")
			Expect(body).To(HavePrefix("**Cluster**: ocp-prod (uuid-123)\n\n"))
		})

		It("UT-RO-621-002: Phase timeout body includes cluster line when identity is set", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))
			nc.SetClusterIdentity("ocp-prod", "uuid-123")

			body := nc.BuildPhaseTimeoutBody("TestSignal", "test-rr", "WorkflowExecution", "10m0s", "2026-01-01T00:00:00Z", "2026-01-01T00:10:00Z")
			Expect(body).To(HavePrefix("**Cluster**: ocp-prod (uuid-123)\n\n"))
		})

		It("UT-RO-621-003: Timeout body omits cluster line when identity is empty", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			body := nc.BuildGlobalTimeoutBody("TestSignal", "test-rr", "AIAnalysis", "30m0s", "2026-01-01T00:00:00Z", "2026-01-01T00:30:00Z")
			Expect(strings.HasPrefix(body, "**Cluster**:")).To(BeFalse(),
				"Timeout body should not start with cluster line when identity is empty")
		})
	})

	Describe("UT-RO-626-008/009: Timeout body builders include RR name", func() {
		It("UT-RO-626-008: Global timeout body contains RR name", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			body := nc.BuildGlobalTimeoutBody("TestSignal", "my-rr-name", "AIAnalysis", "30m0s", "2026-01-01T00:00:00Z", "2026-01-01T00:30:00Z")
			Expect(body).To(ContainSubstring("**Remediation**: my-rr-name"))
		})

		It("UT-RO-626-009: Phase timeout body contains RR name", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			body := nc.BuildPhaseTimeoutBody("TestSignal", "my-rr-name", "WorkflowExecution", "10m0s", "2026-01-01T00:00:00Z", "2026-01-01T00:10:00Z")
			Expect(body).To(ContainSubstring("**Remediation**: my-rr-name"))
		})
	})

	Describe("UT-RO-615-010: Body builders omit cluster line when SetClusterIdentity is NOT called", func() {
		It("UT-RO-615-010: All body builders omit cluster line by default", func() {
			cl := fake.NewClientBuilder().WithScheme(scheme).Build()
			nc := creator.NewNotificationCreator(cl, scheme, rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()))

			rr := helpers.NewRemediationRequest("test-rr-615-010", "default")
			rr.Status.OverallPhase = "Completed"
			rr.Status.Outcome = "Succeeded"
			rr.Status.DuplicateCount = 1
			ai := helpers.NewCompletedAIAnalysis("test-ai-615-010", "default")

			ctx := context.Background()

			approvalName, err := nc.CreateApprovalNotification(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())
			approvalNR := &notificationv1.NotificationRequest{}
			Expect(cl.Get(ctx, types.NamespacedName{Name: approvalName, Namespace: "default"}, approvalNR)).To(Succeed())
			Expect(strings.HasPrefix(approvalNR.Spec.Body, "**Cluster**:")).To(BeFalse(),
				"Approval body should not start with cluster line when SetClusterIdentity is not called")

			completionName, err := nc.CreateCompletionNotification(ctx, rr, ai, "argo", nil)
			Expect(err).ToNot(HaveOccurred())
			completionNR := &notificationv1.NotificationRequest{}
			Expect(cl.Get(ctx, types.NamespacedName{Name: completionName, Namespace: "default"}, completionNR)).To(Succeed())
			Expect(strings.HasPrefix(completionNR.Spec.Body, "**Cluster**:")).To(BeFalse(),
				"Completion body should not start with cluster line when SetClusterIdentity is not called")

			bulkName, err := nc.CreateBulkDuplicateNotification(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			bulkNR := &notificationv1.NotificationRequest{}
			Expect(cl.Get(ctx, types.NamespacedName{Name: bulkName, Namespace: "default"}, bulkNR)).To(Succeed())
			Expect(strings.HasPrefix(bulkNR.Spec.Body, "**Cluster**:")).To(BeFalse(),
				"Bulk duplicate body should not start with cluster line when SetClusterIdentity is not called")

			reviewCtx := &creator.ManualReviewContext{
				Source:  notificationv1.ReviewSourceAIAnalysis,
				Reason:  "LowConfidence",
				Message: "Confidence below threshold",
			}
			manualName, err := nc.CreateManualReviewNotification(ctx, rr, reviewCtx)
			Expect(err).ToNot(HaveOccurred())
			manualNR := &notificationv1.NotificationRequest{}
			Expect(cl.Get(ctx, types.NamespacedName{Name: manualName, Namespace: "default"}, manualNR)).To(Succeed())
			Expect(strings.HasPrefix(manualNR.Spec.Body, "**Cluster**:")).To(BeFalse(),
				"Manual review body should not start with cluster line when SetClusterIdentity is not called")

			selfResolvedName, err := nc.CreateSelfResolvedNotification(ctx, rr, ai)
			Expect(err).ToNot(HaveOccurred())
			selfResolvedNR := &notificationv1.NotificationRequest{}
			Expect(cl.Get(ctx, types.NamespacedName{Name: selfResolvedName, Namespace: "default"}, selfResolvedNR)).To(Succeed())
			Expect(strings.HasPrefix(selfResolvedNR.Spec.Body, "**Cluster**:")).To(BeFalse(),
				"Self-resolved body should not start with cluster line when SetClusterIdentity is not called")
		})
	})
})
