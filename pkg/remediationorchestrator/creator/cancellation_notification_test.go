package creator_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	notificationv1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
)

var _ = Describe("Cancellation NotificationRequest creation", func() {
	It("UT-RO-2588-002: creates an idempotent retryable cancellation notification", func() {
		ctx := context.Background()
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		metrics := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-cancellation-notification",
				Namespace: "kubernaut-system",
				UID:       "rr-cancellation-notification-uid",
			},
			Spec: remediationv1.RemediationRequestSpec{
				SignalName: "HighCPU",
				Severity:   "critical",
				ClusterID:  "prod-east-1",
			},
			Status: remediationv1.RemediationRequestStatus{
				OverallPhase: remediationv1.PhaseCancelling,
				Cancellation: &remediationv1.CancellationStatus{
					RequestID: "kubernaut-system/rr-cancellation-notification:cancellation",
					Source:    remediationv1.CancellationSourceUser,
					Reason:    "Operator requested cancellation",
				},
			},
		}
		Expect(fakeClient.Create(ctx, rr)).To(Succeed())

		notificationCreator := creator.NewNotificationCreator(fakeClient, scheme, metrics)
		name, err := notificationCreator.CreateCancellationNotification(ctx, rr)
		Expect(err).NotTo(HaveOccurred())
		Expect(name).To(Equal("nr-cancellation-rr-cancellation-notification"))

		var notification notificationv1.NotificationRequest
		Expect(fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: rr.Namespace}, &notification)).To(Succeed())
		Expect(notification.Spec.Type).To(Equal(notificationv1.NotificationTypeCancellation))
		Expect(notification.Spec.Priority).To(Equal(notificationv1.NotificationPriorityHigh))
		Expect(notification.Spec.RetryPolicy).NotTo(BeNil())
		Expect(notification.Spec.RetryPolicy.MaxAttempts).To(Equal(5))
		Expect(notification.Spec.Extensions["cancellationRequestID"]).To(Equal("kubernaut-system/rr-cancellation-notification:cancellation"))
		Expect(notification.Spec.Body).To(ContainSubstring("Operator requested cancellation"))
		Expect(notification.OwnerReferences).To(HaveLen(1))
		Expect(notification.OwnerReferences[0].Name).To(Equal(rr.Name))

		secondName, err := notificationCreator.CreateCancellationNotification(ctx, rr)
		Expect(err).NotTo(HaveOccurred())
		Expect(secondName).To(Equal(name))
		var notifications notificationv1.NotificationRequestList
		Expect(fakeClient.List(ctx, &notifications, client.InNamespace(rr.Namespace))).To(Succeed())
		Expect(notifications.Items).To(HaveLen(1))
	})

	It("UT-RO-2588-006: rejects a stale same-name notification from another RR identity", func() {
		ctx := context.Background()
		scheme := runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(notificationv1.AddToScheme(scheme)).To(Succeed())
		metrics := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		rr := &remediationv1.RemediationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rr-reused-name",
				Namespace: "kubernaut-system",
				UID:       "new-rr-uid",
			},
			Status: remediationv1.RemediationRequestStatus{
				Cancellation: &remediationv1.CancellationStatus{
					RequestID: "kubernaut-system/rr-reused-name:new-cancellation",
				},
			},
		}
		stale := &notificationv1.NotificationRequest{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "nr-cancellation-rr-reused-name",
				Namespace: rr.Namespace,
			},
			Spec: notificationv1.NotificationRequestSpec{
				Type: notificationv1.NotificationTypeCancellation,
				RemediationRequestRef: &corev1.ObjectReference{
					APIVersion: remediationv1.GroupVersion.String(),
					Kind:       "RemediationRequest",
					Name:       rr.Name,
					Namespace:  rr.Namespace,
					UID:        "old-rr-uid",
				},
				Extensions: map[string]string{
					"cancellationRequestID": "old-cancellation",
				},
			},
		}
		controller := true
		stale.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: remediationv1.GroupVersion.String(),
			Kind:       "RemediationRequest",
			Name:       rr.Name,
			UID:        "old-rr-uid",
			Controller: &controller,
		}}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(rr, stale).Build()
		creator := creator.NewNotificationCreator(fakeClient, scheme, metrics)

		_, err := creator.CreateCancellationNotification(ctx, rr)
		Expect(err).To(MatchError(ContainSubstring("identity mismatch")))
	})
})
