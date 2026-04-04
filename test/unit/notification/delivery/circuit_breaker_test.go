package delivery_test

import (
	"context"
	"fmt"
	"time"

	"github.com/sony/gobreaker"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	"github.com/jordigilh/kubernaut/pkg/shared/circuitbreaker"
)

type stubService struct {
	deliverFunc func(ctx context.Context, n *notificationv1alpha1.NotificationRequest) error
}

func (s *stubService) Deliver(ctx context.Context, n *notificationv1alpha1.NotificationRequest) error {
	return s.deliverFunc(ctx, n)
}

var _ = Describe("Generic CircuitBreakerService (BR-NOT-055)", func() {

	var (
		cbManager *circuitbreaker.Manager
		nr        *notificationv1alpha1.NotificationRequest
	)

	BeforeEach(func() {
		cbManager = circuitbreaker.NewManager(gobreaker.Settings{
			MaxRequests: 1,
			Interval:    60 * time.Second,
			Timeout:     60 * time.Second,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= 2
			},
		})
		nr = newTestNotification("cb-test", "ns", notificationv1alpha1.NotificationTypeSimple, notificationv1alpha1.NotificationPriorityMedium)
	})

	It("delegates to inner service on success", func() {
		inner := &stubService{deliverFunc: func(_ context.Context, _ *notificationv1alpha1.NotificationRequest) error {
			return nil
		}}
		svc := delivery.NewCircuitBreakerService(inner, cbManager, "slack")
		Expect(svc.Deliver(context.Background(), nr)).To(Succeed())
	})

	It("propagates inner service errors", func() {
		inner := &stubService{deliverFunc: func(_ context.Context, _ *notificationv1alpha1.NotificationRequest) error {
			return fmt.Errorf("upstream failure")
		}}
		svc := delivery.NewCircuitBreakerService(inner, cbManager, "pagerduty")
		err := svc.Deliver(context.Background(), nr)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("upstream failure"))
	})

	It("returns ErrOpenState after consecutive failures trip the breaker", func() {
		callCount := 0
		inner := &stubService{deliverFunc: func(_ context.Context, _ *notificationv1alpha1.NotificationRequest) error {
			callCount++
			return fmt.Errorf("fail %d", callCount)
		}}
		svc := delivery.NewCircuitBreakerService(inner, cbManager, "teams")

		_ = svc.Deliver(context.Background(), nr)
		_ = svc.Deliver(context.Background(), nr)

		err := svc.Deliver(context.Background(), nr)
		Expect(err).To(Equal(gobreaker.ErrOpenState))
		Expect(callCount).To(Equal(2), "inner should not be called when circuit is open")
	})

	It("isolates breaker state per channel name", func() {
		failingInner := &stubService{deliverFunc: func(_ context.Context, _ *notificationv1alpha1.NotificationRequest) error {
			return fmt.Errorf("fail")
		}}
		successInner := &stubService{deliverFunc: func(_ context.Context, _ *notificationv1alpha1.NotificationRequest) error {
			return nil
		}}

		pdSvc := delivery.NewCircuitBreakerService(failingInner, cbManager, "pagerduty")
		teamsSvc := delivery.NewCircuitBreakerService(successInner, cbManager, "teams")

		_ = pdSvc.Deliver(context.Background(), nr)
		_ = pdSvc.Deliver(context.Background(), nr)
		err := pdSvc.Deliver(context.Background(), nr)
		Expect(err).To(Equal(gobreaker.ErrOpenState), "pagerduty breaker should be open")

		Expect(teamsSvc.Deliver(context.Background(), nr)).To(Succeed(),
			"teams breaker should still be closed")
	})

	It("implements delivery.Service interface", func() {
		inner := &stubService{deliverFunc: func(_ context.Context, _ *notificationv1alpha1.NotificationRequest) error {
			return nil
		}}
		var svc delivery.Service = delivery.NewCircuitBreakerService(inner, cbManager, "slack")
		Expect(svc.Deliver(context.Background(), nr)).To(Succeed())
	})
})
