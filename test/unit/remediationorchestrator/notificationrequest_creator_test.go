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

// Package remediationorchestrator contains unit tests for the Remediation Orchestrator controller.
// BR-ORCH-001: Approval Notification Creation
// BR-ORCH-029: Completion Notification Handling
// BR-ORCH-030: Failure Notification Handling
// BR-ORCH-031: Cascade Deletion via Owner References
// BR-ORCH-034: Duplicate Notification Handling
package remediationorchestrator

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	"github.com/jordigilh/kubernaut/pkg/testutil"
)

var _ = Describe("BR-ORCH-001: NotificationRequest Child CRD Creation", func() {
	var (
		ctx        context.Context
		fakeClient client.Client
		scheme     *runtime.Scheme
		nrCreator  *creator.NotificationRequestCreator
		rr         *remediationv1.RemediationRequest
		ai         *aianalysisv1.AIAnalysis
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()

		// Register schemes
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().
			WithScheme(scheme).
			Build()

		nrCreator = creator.NewNotificationRequestCreator(fakeClient, scheme)

		// Create test RemediationRequest using factory
		rr = testutil.NewRemediationRequest("test-rr", "default")
		Expect(fakeClient.Create(ctx, rr)).To(Succeed())

		// Create test AIAnalysis requiring approval using factory
		ai = testutil.NewAIAnalysisRequiringApproval("ai-test-rr", "default", "Confidence below 80% threshold")
		Expect(fakeClient.Create(ctx, ai)).To(Succeed())
	})

	Describe("CreateApprovalNotification (BR-ORCH-001)", func() {
		DescribeTable("should create approval NotificationRequest with correct data",
			func(fieldName string, validateFunc func(*notificationv1.NotificationRequest)) {
				name, err := nrCreator.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
				Expect(name).To(Equal("nr-approval-test-rr"))

				nr := &notificationv1.NotificationRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: rr.Namespace,
				}, nr)).To(Succeed())

				validateFunc(nr)
			},
			// Notification type and priority
			Entry("Type is escalation",
				"Type",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeEscalation))
				}),
			Entry("Priority matches signal priority (P1=critical)",
				"Priority",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityCritical))
				}),

			// Recipients and Channels are NOT set - determined by Notification Service routing rules (BR-NOT-065)
			Entry("Recipients empty (determined by routing rules BR-NOT-065)",
				"Recipients",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Recipients).To(BeEmpty())
				}),
			Entry("Channels empty (determined by routing rules BR-NOT-065)",
				"Channels",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Channels).To(BeEmpty())
				}),

			// Subject and body
			Entry("Subject contains signal name",
				"Subject",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Subject).To(ContainSubstring("TestSignal"))
				}),
			Entry("Body contains signal details",
				"Body:SignalDetails",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Body).To(ContainSubstring("Signal Details"))
				}),
			Entry("Body contains approval reason",
				"Body:ApprovalReason",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Body).To(ContainSubstring("Confidence below"))
				}),

			// Metadata
			Entry("Metadata includes remediationRequestName",
				"Metadata:remediationRequestName",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Metadata).To(HaveKeyWithValue("remediationRequestName", "test-rr"))
				}),
			Entry("Metadata includes notificationType",
				"Metadata:notificationType",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Metadata).To(HaveKeyWithValue("notificationType", "approval_required"))
				}),
			Entry("Metadata includes aiAnalysisName",
				"Metadata:aiAnalysisName",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Metadata).To(HaveKeyWithValue("aiAnalysisName", "ai-test-rr"))
				}),

			// Owner reference (BR-ORCH-031)
			Entry("Owner reference set for cascade deletion",
				"OwnerReference",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.OwnerReferences).To(HaveLen(1))
					Expect(nr.OwnerReferences[0].Name).To(Equal("test-rr"))
					Expect(nr.OwnerReferences[0].Kind).To(Equal("RemediationRequest"))
					Expect(*nr.OwnerReferences[0].Controller).To(BeTrue())
				}),

			// Labels
			Entry("remediation-request label set",
				"Label:remediation-request",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/remediation-request", "test-rr"))
				}),
			Entry("notification-type label set",
				"Label:notification-type",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/notification-type", "approval_required"))
				}),

			// Routing labels for BR-NOT-065 (Channel Routing Based on Labels)
			Entry("severity routing label set (BR-NOT-065)",
				"Label:severity",
				func(nr *notificationv1.NotificationRequest) {
					// Default factory severity is "warning"
					Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/severity", "warning"))
				}),
			Entry("environment routing label set (BR-NOT-065)",
				"Label:environment",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/environment", "production"))
				}),
			Entry("priority routing label set (BR-NOT-065)",
				"Label:priority",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Labels).To(HaveKeyWithValue("kubernaut.ai/priority", "P1"))
				}),
		)

		Context("idempotency", func() {
			It("should return existing name if NotificationRequest already exists", func() {
				name1, err := nrCreator.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())

				name2, err := nrCreator.CreateApprovalNotification(ctx, rr, ai)
				Expect(err).NotTo(HaveOccurred())
				Expect(name2).To(Equal(name1))

				nrList := &notificationv1.NotificationRequestList{}
				Expect(fakeClient.List(ctx, nrList, client.InNamespace(rr.Namespace))).To(Succeed())
				Expect(nrList.Items).To(HaveLen(1))
			})
		})
	})

	Describe("CreateCompletionNotification (BR-ORCH-029)", func() {
		It("should create completion NotificationRequest", func() {
			name, err := nrCreator.CreateCompletionNotification(ctx, rr)
			Expect(err).NotTo(HaveOccurred())
			Expect(name).To(Equal("nr-completed-test-rr"))

			nr := &notificationv1.NotificationRequest{}
			Expect(fakeClient.Get(ctx, client.ObjectKey{
				Name:      name,
				Namespace: rr.Namespace,
			}, nr)).To(Succeed())

			Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeStatusUpdate))
			Expect(nr.Spec.Subject).To(ContainSubstring("Completed"))
			Expect(nr.Spec.Subject).To(ContainSubstring("✅"))
		})
	})

	Describe("CreateFailureNotification (BR-ORCH-030)", func() {
		DescribeTable("should create failure NotificationRequest with correct data",
			func(fieldName string, validateFunc func(*notificationv1.NotificationRequest)) {
				name, err := nrCreator.CreateFailureNotification(ctx, rr, "Workflow execution failed: OOMKilled")
				Expect(err).NotTo(HaveOccurred())
				Expect(name).To(Equal("nr-failed-test-rr"))

				nr := &notificationv1.NotificationRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: rr.Namespace,
				}, nr)).To(Succeed())

				validateFunc(nr)
			},
			Entry("Type is escalation for failures",
				"Type",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeEscalation))
				}),
			Entry("Priority is high for failures",
				"Priority",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityHigh))
				}),
			Entry("Subject indicates failure",
				"Subject",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Subject).To(ContainSubstring("Failed"))
					Expect(nr.Spec.Subject).To(ContainSubstring("❌"))
				}),
			Entry("Body contains failure reason",
				"Body",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Body).To(ContainSubstring("OOMKilled"))
				}),
			Entry("Metadata includes failureReason",
				"Metadata:failureReason",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Metadata).To(HaveKeyWithValue("failureReason", "Workflow execution failed: OOMKilled"))
				}),
			Entry("RetentionDays is 30 for failures",
				"RetentionDays",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.RetentionDays).To(Equal(30))
				}),
		)
	})

	Describe("CreateSkippedNotification (BR-ORCH-034)", func() {
		DescribeTable("should create skipped NotificationRequest with correct data",
			func(fieldName string, validateFunc func(*notificationv1.NotificationRequest)) {
				name, err := nrCreator.CreateSkippedNotification(ctx, rr, "ResourceBusy", "we-other-rr")
				Expect(err).NotTo(HaveOccurred())
				Expect(name).To(Equal("nr-skipped-test-rr"))

				nr := &notificationv1.NotificationRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: rr.Namespace,
				}, nr)).To(Succeed())

				validateFunc(nr)
			},
			Entry("Type is status-update for skipped",
				"Type",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Type).To(Equal(notificationv1.NotificationTypeStatusUpdate))
				}),
			Entry("Priority is low for skipped",
				"Priority",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Priority).To(Equal(notificationv1.NotificationPriorityLow))
				}),
			Entry("Subject indicates skipped",
				"Subject",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Subject).To(ContainSubstring("Skipped"))
					Expect(nr.Spec.Subject).To(ContainSubstring("⏭️"))
				}),
			Entry("Body contains skip reason",
				"Body:skipReason",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Body).To(ContainSubstring("ResourceBusy"))
				}),
			Entry("Body contains duplicateOf reference",
				"Body:duplicateOf",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Body).To(ContainSubstring("we-other-rr"))
				}),
			Entry("Metadata includes skipReason",
				"Metadata:skipReason",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Metadata).To(HaveKeyWithValue("skipReason", "ResourceBusy"))
				}),
			Entry("Metadata includes duplicateOf",
				"Metadata:duplicateOf",
				func(nr *notificationv1.NotificationRequest) {
					Expect(nr.Spec.Metadata).To(HaveKeyWithValue("duplicateOf", "we-other-rr"))
				}),
		)
	})

	// Priority mapping tests
	Describe("Priority Mapping", func() {
		DescribeTable("should map signal priority to notification priority correctly",
			func(signalPriority string, expectedNotifPriority notificationv1.NotificationPriority) {
				rrWithPriority := testutil.NewRemediationRequest("test-rr-priority", "default", testutil.RemediationRequestOpts{
					Priority: signalPriority,
				})
				Expect(fakeClient.Create(ctx, rrWithPriority)).To(Succeed())

				name, err := nrCreator.CreateApprovalNotification(ctx, rrWithPriority, ai)
				Expect(err).NotTo(HaveOccurred())

				nr := &notificationv1.NotificationRequest{}
				Expect(fakeClient.Get(ctx, client.ObjectKey{
					Name:      name,
					Namespace: rrWithPriority.Namespace,
				}, nr)).To(Succeed())

				Expect(nr.Spec.Priority).To(Equal(expectedNotifPriority))
			},
			Entry("P1 maps to critical", "P1", notificationv1.NotificationPriorityCritical),
			Entry("CRITICAL maps to critical", "CRITICAL", notificationv1.NotificationPriorityCritical),
			Entry("P2 maps to high", "P2", notificationv1.NotificationPriorityHigh),
			Entry("HIGH maps to high", "HIGH", notificationv1.NotificationPriorityHigh),
			Entry("P3 maps to medium", "P3", notificationv1.NotificationPriorityMedium),
			Entry("MEDIUM maps to medium", "MEDIUM", notificationv1.NotificationPriorityMedium),
			Entry("P4 maps to low", "P4", notificationv1.NotificationPriorityLow),
			Entry("LOW maps to low", "LOW", notificationv1.NotificationPriorityLow),
		)
	})
})
