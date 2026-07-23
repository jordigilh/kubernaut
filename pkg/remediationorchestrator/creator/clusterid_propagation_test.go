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

package creator_test

import (
	"context"
	"testing"
	"time"

	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

func TestClusterIDPropagation(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ClusterID Propagation Suite")
}

var _ = Describe("SignalProcessingCreator ClusterID Propagation (BR-INTEGRATION-054)", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
		m         *rometrics.Metrics
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
		reg := prometheus.NewRegistry()
		m = rometrics.NewMetricsWithRegistry(reg)
	})

	It("UT-SP-054-001 [AC-4]: propagates ClusterID from RemediationRequest to SignalData", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-test-cluster",
				Namespace: "kubernaut-system",
				UID:       "test-uid-123",
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
				SignalName:        "HighCPU",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "api-server",
					Namespace: "prod",
				},
				FiringTime:   metav1.Now(),
				ReceivedTime: metav1.Now(),
				ClusterID:    "prod-east-1",
			},
		}

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()

		spCreator := creator.NewSignalProcessingCreator(k8sClient, scheme, m)
		name, err := spCreator.Create(ctx, rr)
		Expect(err).ToNot(HaveOccurred())
		Expect(name).ToNot(BeEmpty())

		sp := &signalprocessingv1.SignalProcessing{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "kubernaut-system"}, sp)
		Expect(err).ToNot(HaveOccurred())

		// RED: This field does not exist yet -- compilation will fail
		Expect(sp.Spec.Signal.ClusterID).To(Equal("prod-east-1"),
			"AC-4: ClusterID must be propagated from RemediationRequest to SignalData")
	})

	It("UT-SP-054-001 [AC-4]: leaves ClusterID empty for local hub cluster signals", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-test-local",
				Namespace: "kubernaut-system",
				UID:       "test-uid-456",
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
				SignalName:        "HighMemory",
				Severity:          "warning",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Pod",
					Name:      "web-pod",
					Namespace: "default",
				},
				FiringTime:   metav1.Now(),
				ReceivedTime: metav1.Now(),
			},
		}

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()

		spCreator := creator.NewSignalProcessingCreator(k8sClient, scheme, m)
		name, err := spCreator.Create(ctx, rr)
		Expect(err).ToNot(HaveOccurred())

		sp := &signalprocessingv1.SignalProcessing{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "kubernaut-system"}, sp)
		Expect(err).ToNot(HaveOccurred())

		// RED: This field does not exist yet -- compilation will fail
		Expect(sp.Spec.Signal.ClusterID).To(BeEmpty(),
			"AC-4: empty ClusterID indicates local hub cluster")
	})

	It("UT-NT-054-001 [AC-4]: propagates ClusterID from RemediationRequest to NotificationRequest", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-test-notification-cluster",
				Namespace: "kubernaut-system",
				UID:       "test-uid-nr-001",
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "aabbccdd1234567890abcdef1234567890abcdef1234567890abcdef12345678",
				SignalName:        "HighDiskUsage",
				Severity:          "warning",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "StatefulSet",
					Name:      "db-primary",
					Namespace: "database",
				},
				FiringTime:   metav1.Now(),
				ReceivedTime: metav1.Now(),
				ClusterID:    "prod-east-1",
			},
			Status: remediationv1.RemediationRequestStatus{
				DuplicateCount: 3,
			},
		}

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr).
			Build()

		ntCreator := creator.NewNotificationCreator(k8sClient, scheme, m)
		name, err := ntCreator.CreateBulkDuplicateNotification(ctx, rr)
		Expect(err).ToNot(HaveOccurred())
		Expect(name).ToNot(BeEmpty())

		nr := &notificationv1.NotificationRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "kubernaut-system"}, nr)
		Expect(err).ToNot(HaveOccurred())

		Expect(nr.Spec.ClusterID).To(Equal("prod-east-1"),
			"AC-4: ClusterID must be propagated from RemediationRequest to NotificationRequest")
	})

	It("UT-RAR-054-001 [AC-4]: propagates ClusterID from RemediationRequest to RemediationApprovalRequest", func() {
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-test-approval-cluster",
				Namespace: "kubernaut-system",
				UID:       "test-uid-rar-001",
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalFingerprint: "ddccbbaa1234567890abcdef1234567890abcdef1234567890abcdef12345678",
				SignalName:        "HighLatency",
				Severity:          "critical",
				SignalType:        "alert",
				TargetType:        "kubernetes",
				TargetResource: remediationv1.ResourceIdentifier{
					Kind:      "Deployment",
					Name:      "payment-service",
					Namespace: "payments",
				},
				FiringTime:   metav1.Now(),
				ReceivedTime: metav1.Now(),
				ClusterID:    "prod-east-1",
			},
		}

		ai := &aianalysisv1.AIAnalysis{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aa-test-approval-cluster",
				Namespace: "kubernaut-system",
			},
			Status: aianalysisv1.AIAnalysisStatus{
				SelectedWorkflow: &aianalysisv1.SelectedWorkflow{
					WorkflowSnapshot: sharedtypes.WorkflowSnapshot{
						WorkflowID:      "restart-pod",
						WorkflowName:    "restart-pod",
						ActionType:      "RestartPod",
						Version:         "1.0.0",
						ExecutionBundle: "oci://registry/workflows/restart-pod:v1.0.0",
					},
					Confidence: 0.85,
					Rationale:  "Pod restart recommended for high latency",
				},
				ApprovalReason: "High severity requires approval",
				RootCause:      "Memory leak detected",
			},
		}

		k8sClient = fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(rr).
			WithStatusSubresource(rr, &remediationv1.RemediationApprovalRequest{}).
			Build()

		approvalCreator := creator.NewApprovalCreator(k8sClient, scheme, m, 15*time.Minute)
		name, err := approvalCreator.Create(ctx, rr, ai)
		Expect(err).ToNot(HaveOccurred())
		Expect(name).ToNot(BeEmpty())

		rar := &remediationv1.RemediationApprovalRequest{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "kubernaut-system"}, rar)
		Expect(err).ToNot(HaveOccurred())

		Expect(rar.Spec.ClusterID).To(Equal("prod-east-1"),
			"AC-4: ClusterID must be propagated from RemediationRequest to RemediationApprovalRequest")
	})
})
