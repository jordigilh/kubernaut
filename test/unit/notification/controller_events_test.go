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

// Package notification contains unit tests for Notification controller.
//
// DD-EVENT-001 v1.1: K8s Event Observability for Notification Controller
// BR-NT-095: All Notification lifecycle events must be emitted via Recorder.Event
package notification

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
	notificationcontroller "github.com/jordigilh/kubernaut/internal/controller/notification"
	"github.com/jordigilh/kubernaut/pkg/audit"
	notificationaudit "github.com/jordigilh/kubernaut/pkg/notification/audit"
	"github.com/jordigilh/kubernaut/pkg/notification/delivery"
	notificationmetrics "github.com/jordigilh/kubernaut/pkg/notification/metrics"
	notificationstatus "github.com/jordigilh/kubernaut/pkg/notification/status"
	"github.com/jordigilh/kubernaut/pkg/shared/circuitbreaker"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/pkg/shared/sanitization"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sony/gobreaker"
)

// drainEvents reads all available events from the FakeRecorder channel.
func drainEventsNT(recorder *record.FakeRecorder) []string {
	var collected []string
	for {
		select {
		case evt := <-recorder.Events:
			collected = append(collected, evt)
		default:
			return collected
		}
	}
}

// containsEvent checks if any event string contains ALL the given substrings.
func containsEventNT(eventList []string, substrings ...string) bool {
	for _, evt := range eventList {
		allMatch := true
		for _, sub := range substrings {
			if !containsSubstrNT(evt, sub) {
				allMatch = false
				break
			}
		}
		if allMatch {
			return true
		}
	}
	return false
}

func containsSubstrNT(s, substr string) bool {
	return len(s) >= len(substr) && findSubstrNT(s, substr)
}

func findSubstrNT(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// DD-EVENT-001 v1.1: K8s Event Observability for Notification Controller
var _ = Describe("Notification Controller K8s Events [DD-EVENT-001]", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(notificationv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	// UT-NT-EVT-00: Event reason constants verification
	Context("UT-NT-EVT-00: DD-EVENT-001 Event Constants", func() {
		It("should have correct Notification event reason constants", func() {
			Expect(events.EventReasonReconcileStarted).To(Equal("ReconcileStarted"))
			Expect(events.EventReasonPhaseTransition).To(Equal("PhaseTransition"))
			Expect(events.EventReasonNotificationSent).To(Equal("NotificationSent"))
			Expect(events.EventReasonNotificationFailed).To(Equal("NotificationFailed"))
			Expect(events.EventReasonNotificationPartiallySent).To(Equal("NotificationPartiallySent"))
			Expect(events.EventReasonNotificationRetrying).To(Equal("NotificationRetrying"))
			Expect(events.EventReasonCircuitBreakerOpen).To(Equal("CircuitBreakerOpen"))
		})
	})

	// UT-NT-EVT-01: ReconcileStarted event
	Context("UT-NT-EVT-01: ReconcileStarted event", func() {
		It("should emit ReconcileStarted Normal event when reconciliation begins", func() {
			recorder := record.NewFakeRecorder(20)
			nt := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-nt-evt-01",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "Test",
					Body:     "Test body",
					Priority: "critical",
					Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole},
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase: "", // Uninitialized
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(nt).
				WithStatusSubresource(nt).
				Build()

			metricsRecorder := notificationmetrics.NewPrometheusRecorderWithRegistry(prometheus.NewRegistry())
			statusManager := notificationstatus.NewManager(fakeClient, fakeClient)
			sanitizer := sanitization.NewSanitizer()
			auditManager := notificationaudit.NewManager("test")
			var store audit.AuditStore = &mockAuditStore{}

			orchestrator := delivery.NewOrchestrator(
				sanitizer,
				metricsRecorder,
				statusManager,
				ctrl.Log.WithName("delivery-orchestrator"),
			)
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), delivery.NewConsoleDeliveryService())

			cbManager := circuitbreaker.NewManager(gobreaker.Settings{
				ReadyToTrip: func(gobreaker.Counts) bool { return false },
			})

			reconciler := &notificationcontroller.NotificationRequestReconciler{
				Client:               fakeClient,
				APIReader:            fakeClient,
				Scheme:               scheme,
				Recorder:             recorder,
				Sanitizer:            sanitizer,
				CircuitBreaker:       cbManager,
				AuditStore:           store,
				AuditManager:         auditManager,
				Metrics:              metricsRecorder,
				StatusManager:        statusManager,
				DeliveryOrchestrator: orchestrator,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-nt-evt-01", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEventsNT(recorder)
			Expect(containsEventNT(evts, "Normal", events.EventReasonReconcileStarted)).
				To(BeTrue(), "Expected ReconcileStarted event, got: %v", evts)
		})
	})

	// UT-NT-EVT-02: PhaseTransition event on Pending → Sending
	Context("UT-NT-EVT-02: PhaseTransition event", func() {
		It("should emit PhaseTransition Normal event when transitioning Pending to Sending", func() {
			recorder := record.NewFakeRecorder(20)
			nt := &notificationv1alpha1.NotificationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-nt-evt-02",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: notificationv1alpha1.NotificationRequestSpec{
					Subject:  "Test",
					Body:     "Test body",
					Priority: "critical",
					Channels: []notificationv1alpha1.Channel{notificationv1alpha1.ChannelConsole},
				},
				Status: notificationv1alpha1.NotificationRequestStatus{
					Phase:                notificationv1alpha1.NotificationPhasePending,
					DeliveryAttempts:     []notificationv1alpha1.DeliveryAttempt{},
					TotalAttempts:       0,
					SuccessfulDeliveries: 0,
					FailedDeliveries:    0,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(nt).
				WithStatusSubresource(nt).
				Build()

			metricsRecorder := notificationmetrics.NewPrometheusRecorderWithRegistry(prometheus.NewRegistry())
			statusManager := notificationstatus.NewManager(fakeClient, fakeClient)
			sanitizer := sanitization.NewSanitizer()
			auditManager := notificationaudit.NewManager("test")
			var store audit.AuditStore = &mockAuditStore{}

			orchestrator := delivery.NewOrchestrator(
				sanitizer,
				metricsRecorder,
				statusManager,
				ctrl.Log.WithName("delivery-orchestrator"),
			)
			orchestrator.RegisterChannel(string(notificationv1alpha1.ChannelConsole), delivery.NewConsoleDeliveryService())

			cbManager := circuitbreaker.NewManager(gobreaker.Settings{
				ReadyToTrip: func(gobreaker.Counts) bool { return false },
			})

			reconciler := &notificationcontroller.NotificationRequestReconciler{
				Client:               fakeClient,
				APIReader:            fakeClient,
				Scheme:               scheme,
				Recorder:             recorder,
				Sanitizer:            sanitizer,
				CircuitBreaker:       cbManager,
				AuditStore:           store,
				AuditManager:         auditManager,
				Metrics:              metricsRecorder,
				StatusManager:        statusManager,
				DeliveryOrchestrator: orchestrator,
			}

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{Name: "test-nt-evt-02", Namespace: "default"},
			}

			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			evts := drainEventsNT(recorder)
			Expect(containsEventNT(evts, "Normal", events.EventReasonPhaseTransition, "Sending")).
				To(BeTrue(), "Expected PhaseTransition event, got: %v", evts)
		})
	})
})
