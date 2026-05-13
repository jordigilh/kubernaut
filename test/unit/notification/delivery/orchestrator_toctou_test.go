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

package delivery_test

import (
	"context"
	"sync"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	ctrl "sigs.k8s.io/controller-runtime"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	"github.com/jordigilh/kubernaut/test/shared/helpers"
)

var _ = Describe("Orchestrator TOCTOU Race Prevention (DD-NOT-008)", func() {
	var (
		orchestrator *delivery.Orchestrator
		metrics      *notificationmetrics.Metrics
		logger       = ctrl.Log.WithName("test-toctou")
		ctx          = context.Background()
	)

	BeforeEach(func() {
		metrics = notificationmetrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		orchestrator = delivery.NewOrchestrator(nil, metrics, nil, logger)
	})

	// BR-NOT-052: MaxAttempts enforcement under concurrent reconciliation.
	//
	// The TOCTOU race: two concurrent DeliverToChannels calls both check
	// attemptCount < MaxAttempts before either increments the in-flight counter,
	// so both pass the gate and both deliver — exceeding MaxAttempts.
	Describe("Concurrent delivery must respect MaxAttempts", func() {
		It("UT-TOCTOU-001: concurrent reconciles must not exceed MaxAttempts total Deliver calls", func() {
			const maxAttempts = 1
			const concurrency = 10

			var deliverCalls atomic.Int32

			svc := &mockDeliveryService{
				deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					deliverCalls.Add(1)
					return nil
				},
			}
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), svc)

			notification := helpers.NewNotificationRequest("toctou-test", "default")
			policy := &notificationv1alpha1.RetryPolicy{MaxAttempts: maxAttempts}

			// Use the orchestrator's own success tracking — mirrors the
			// real controller's channelAlreadySucceeded callback which
			// calls HasChannelSucceeded.
			channelSucceeded := func(n *notificationv1alpha1.NotificationRequest, ch string) bool {
				return orchestrator.HasChannelSucceeded(n, ch, false)
			}
			noopPermanent := func(*notificationv1alpha1.NotificationRequest, string) bool { return false }
			noopAuditSent := func(context.Context, *notificationv1alpha1.NotificationRequest, string) error { return nil }
			noopAuditFail := func(context.Context, *notificationv1alpha1.NotificationRequest, string, error) error {
				return nil
			}

			// getChannelAttemptCount uses the orchestrator's own
			// GetTotalAttemptCount, which includes in-flight tracking —
			// this mirrors the real controller's callback.
			getAttemptCount := func(n *notificationv1alpha1.NotificationRequest, ch string) int {
				return orchestrator.GetTotalAttemptCount(n, ch, 0)
			}

			var wg sync.WaitGroup
			wg.Add(concurrency)
			for i := 0; i < concurrency; i++ {
				go func() {
					defer GinkgoRecover()
					defer wg.Done()
					_, _ = orchestrator.DeliverToChannels(
						ctx, notification,
						[]notificationv1alpha1.Channel{notificationv1alpha1.ChannelFile},
						policy,
						channelSucceeded, noopPermanent, getAttemptCount,
						noopAuditSent, noopAuditFail,
						nil,
					)
				}()
			}
			wg.Wait()

			// With MaxAttempts=1, we expect at most 1 real Deliver call
			// regardless of how many concurrent reconciles race.
			// singleflight may collapse truly concurrent calls, but
			// sequential arrivals each get their own Do() invocation.
			Expect(int(deliverCalls.Load())).To(BeNumerically("<=", maxAttempts),
				"BR-NOT-052: concurrent reconciles must not exceed MaxAttempts Deliver calls")
		})

		It("UT-TOCTOU-002: in-flight reservation is visible to getChannelAttemptCount", func() {
			// Verify the TOCTOU fix: incrementInFlightAttempts is called
			// BEFORE getChannelAttemptCount, so the callback always sees
			// at least 1 in-flight attempt (our own reservation).
			var minCountSeen atomic.Int32
			minCountSeen.Store(999)

			svc := &mockDeliveryService{
				deliverFunc: func(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) error {
					return nil
				},
			}
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelFile), svc)

			notification := helpers.NewNotificationRequest("toctou-ordering", "default")
			policy := &notificationv1alpha1.RetryPolicy{MaxAttempts: 1}

			channelSucceeded := func(n *notificationv1alpha1.NotificationRequest, ch string) bool {
				return orchestrator.HasChannelSucceeded(n, ch, false)
			}
			noopPermanent := func(*notificationv1alpha1.NotificationRequest, string) bool { return false }
			noopAuditSent := func(context.Context, *notificationv1alpha1.NotificationRequest, string) error { return nil }
			noopAuditFail := func(context.Context, *notificationv1alpha1.NotificationRequest, string, error) error {
				return nil
			}

			getAttemptCount := func(n *notificationv1alpha1.NotificationRequest, ch string) int {
				total := orchestrator.GetTotalAttemptCount(n, ch, 0)
				for {
					cur := minCountSeen.Load()
					if int32(total) >= cur || minCountSeen.CompareAndSwap(cur, int32(total)) {
						break
					}
				}
				return total
			}

			_, err := orchestrator.DeliverToChannels(
				ctx, notification,
				[]notificationv1alpha1.Channel{notificationv1alpha1.ChannelFile},
				policy,
				channelSucceeded, noopPermanent, getAttemptCount,
				noopAuditSent, noopAuditFail,
				nil,
			)
			Expect(err).ToNot(HaveOccurred())

			// The TOCTOU fix guarantees that when getChannelAttemptCount
			// is called, our in-flight reservation is already visible,
			// so the count must be >= 1. Before the fix, the count
			// could be 0 (check happened before increment).
			Expect(int(minCountSeen.Load())).To(BeNumerically(">=", 1),
				"DD-NOT-008: in-flight reservation must be visible to getChannelAttemptCount")
		})
	})
})
