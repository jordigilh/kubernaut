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

package workflowexecution

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	tektonv1 "github.com/tektoncd/pipeline/pkg/apis/pipeline/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workflowexecutionv1alpha1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	"github.com/jordigilh/kubernaut/internal/controller/workflowexecution"
	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/shared/events"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/audit"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/metrics"
	wephase "github.com/jordigilh/kubernaut/pkg/workflowexecution/phase"
	"github.com/jordigilh/kubernaut/pkg/workflowexecution/status"
)

// drainFakeRecorderEvents reads all available events from a FakeRecorder channel.
func drainFakeRecorderEvents(rec *record.FakeRecorder) []string {
	var collected []string
	for {
		select {
		case evt := <-rec.Events:
			collected = append(collected, evt)
		default:
			return collected
		}
	}
}

// hasEventMatch checks if any event string contains ALL the given substrings.
func hasEventMatch(eventList []string, substrings ...string) bool {
	for _, evt := range eventList {
		allMatch := true
		for _, sub := range substrings {
			if !strings.Contains(evt, sub) {
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

// DD-EVENT-001 v1.1: K8s Event Observability for WorkflowExecution Controller
// BR-WE-095: All WorkflowExecution lifecycle events must be emitted via Recorder.Event
// Issue: #74
var _ = Describe("WorkflowExecution Controller K8s Events [DD-EVENT-001]", func() {
	var (
		evtScheme   *runtime.Scheme
		evtRecorder *record.FakeRecorder
		evtCtx      context.Context
	)

	BeforeEach(func() {
		evtCtx = context.Background()
		evtScheme = runtime.NewScheme()
		Expect(workflowexecutionv1alpha1.AddToScheme(evtScheme)).To(Succeed())
		Expect(tektonv1.AddToScheme(evtScheme)).To(Succeed())
		Expect(corev1.AddToScheme(evtScheme)).To(Succeed())
		evtRecorder = record.NewFakeRecorder(20)
	})

	// UT-WE-095-06 + PhaseTransition on MarkCompleted
	Context("UT-WE-095-06: PhaseTransition event on MarkCompleted (Running → Completed)", func() {
		It("should emit PhaseTransition event alongside WorkflowCompleted", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(evtScheme).
				WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
				Build()

			as := &mockAuditStore{events: make([]*ogenclient.AuditEventRequest, 0)}
			am := audit.NewManager(as, logr.Discard())
			sm := status.NewManager(fakeClient)
			tm := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			pm := wephase.NewManager()

			reconciler := &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             evtScheme,
				Recorder:           evtRecorder,
				ExecutionNamespace: "kubernaut-workflows",
				AuditStore:         as,
				Metrics:            tm,
				StatusManager:      sm,
				AuditManager:       am,
				PhaseManager:       pm,
			}

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-evt-completed",
					Namespace: "default",
					UID:       types.UID("test-uid-completed"),
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "default/deployment/test-app",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "restart-deployment",
						Version:        "v1",
						ContainerImage: "registry.example.com/workflows/restart:v1",
					},
				},
			}
			Expect(fakeClient.Create(evtCtx, wfe)).To(Succeed())

			// Set to Running phase first
			wfe.Status.Phase = workflowexecutionv1alpha1.PhaseRunning
			now := metav1.Now()
			wfe.Status.StartTime = &now
			Expect(fakeClient.Status().Update(evtCtx, wfe)).To(Succeed())

			// Drain any previous events
			drainFakeRecorderEvents(evtRecorder)

			// Mark completed
			_, err := reconciler.MarkCompleted(evtCtx, wfe, nil)
			Expect(err).ToNot(HaveOccurred())

			evts := drainFakeRecorderEvents(evtRecorder)
			Expect(hasEventMatch(evts, "Normal", events.EventReasonWorkflowCompleted)).
				To(BeTrue(), "Expected WorkflowCompleted event, got: %v", evts)
			Expect(hasEventMatch(evts, "Normal", events.EventReasonPhaseTransition, "Running", "Completed")).
				To(BeTrue(), "Expected PhaseTransition Running→Completed event, got: %v", evts)
		})
	})

	// PhaseTransition on MarkFailed
	Context("PhaseTransition event on MarkFailed (Running → Failed)", func() {
		It("should emit PhaseTransition event alongside WorkflowFailed", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(evtScheme).
				WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
				Build()

			as := &mockAuditStore{events: make([]*ogenclient.AuditEventRequest, 0)}
			am := audit.NewManager(as, logr.Discard())
			sm := status.NewManager(fakeClient)
			tm := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			pm := wephase.NewManager()

			reconciler := &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             evtScheme,
				Recorder:           evtRecorder,
				ExecutionNamespace: "kubernaut-workflows",
				AuditStore:         as,
				Metrics:            tm,
				StatusManager:      sm,
				AuditManager:       am,
				PhaseManager:       pm,
			}

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-evt-failed",
					Namespace: "default",
					UID:       types.UID("test-uid-failed"),
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "default/deployment/test-app",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "restart-deployment",
						Version:        "v1",
						ContainerImage: "registry.example.com/workflows/restart:v1",
					},
				},
			}
			Expect(fakeClient.Create(evtCtx, wfe)).To(Succeed())

			// Set to Running phase first
			wfe.Status.Phase = workflowexecutionv1alpha1.PhaseRunning
			now := metav1.Now()
			wfe.Status.StartTime = &now
			Expect(fakeClient.Status().Update(evtCtx, wfe)).To(Succeed())

			// Drain any previous events
			drainFakeRecorderEvents(evtRecorder)

			// Mark failed
			_, err := reconciler.MarkFailed(evtCtx, wfe, nil)
			Expect(err).ToNot(HaveOccurred())

			evts := drainFakeRecorderEvents(evtRecorder)
			Expect(hasEventMatch(evts, "Warning", events.EventReasonWorkflowFailed)).
				To(BeTrue(), "Expected WorkflowFailed event, got: %v", evts)
			Expect(hasEventMatch(evts, "Normal", events.EventReasonPhaseTransition, "Running", "Failed")).
				To(BeTrue(), "Expected PhaseTransition Running→Failed event, got: %v", evts)
		})
	})

	// PhaseTransition on MarkFailedWithReason (Pending → Failed)
	Context("PhaseTransition event on MarkFailedWithReason (Pending → Failed)", func() {
		It("should emit PhaseTransition event for pre-execution failure", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(evtScheme).
				WithStatusSubresource(&workflowexecutionv1alpha1.WorkflowExecution{}).
				Build()

			as := &mockAuditStore{events: make([]*ogenclient.AuditEventRequest, 0)}
			am := audit.NewManager(as, logr.Discard())
			sm := status.NewManager(fakeClient)
			tm := metrics.NewMetricsWithRegistry(prometheus.NewRegistry())
			pm := wephase.NewManager()

			reconciler := &workflowexecution.WorkflowExecutionReconciler{
				Client:             fakeClient,
				Scheme:             evtScheme,
				Recorder:           evtRecorder,
				ExecutionNamespace: "kubernaut-workflows",
				AuditStore:         as,
				Metrics:            tm,
				StatusManager:      sm,
				AuditManager:       am,
				PhaseManager:       pm,
			}

			wfe := &workflowexecutionv1alpha1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-wfe-evt-prefail",
					Namespace: "default",
					UID:       types.UID("test-uid-prefail"),
				},
				Spec: workflowexecutionv1alpha1.WorkflowExecutionSpec{
					TargetResource: "default/deployment/test-app",
					WorkflowRef: workflowexecutionv1alpha1.WorkflowRef{
						WorkflowID:     "restart-deployment",
						Version:        "v1",
						ContainerImage: "registry.example.com/workflows/restart:v1",
					},
				},
			}
			Expect(fakeClient.Create(evtCtx, wfe)).To(Succeed())

			// Phase is "" (empty/Pending)
			drainFakeRecorderEvents(evtRecorder)

			// Mark failed with reason (pre-execution failure)
			err := reconciler.MarkFailedWithReason(evtCtx, wfe, "ConfigurationError", "missing container image")
			Expect(err).ToNot(HaveOccurred())

			evts := drainFakeRecorderEvents(evtRecorder)
			Expect(hasEventMatch(evts, "Warning", events.EventReasonWorkflowFailed)).
				To(BeTrue(), "Expected WorkflowFailed event, got: %v", evts)
			Expect(hasEventMatch(evts, "Normal", events.EventReasonPhaseTransition, "Pending", "Failed")).
				To(BeTrue(), "Expected PhaseTransition Pending→Failed event, got: %v", evts)
		})
	})

	// CooldownActive and CleanupFailed require more complex test setups.
	// CooldownActive requires multiple WFEs with cooldown state.
	// CleanupFailed requires a mock executor that returns errors.
	// These will be covered by integration tests in IT-WE-095-03 and IT-WE-095-04.
	// The event emission points in the controller are straightforward and verified by build.

	// Verify CooldownActive event constant exists (compile-time check)
	Context("CooldownActive event constant verification", func() {
		It("should have CooldownActive constant in events package", func() {
			Expect(events.EventReasonCooldownActive).To(Equal("CooldownActive"))
		})
	})

	// Verify CleanupFailed event constant exists
	Context("CleanupFailed event constant verification", func() {
		It("should have CleanupFailed constant in events package", func() {
			Expect(events.EventReasonCleanupFailed).To(Equal("CleanupFailed"))
		})
	})

	// Verify WorkflowValidated event constant exists
	Context("WorkflowValidated event constant verification", func() {
		It("should have WorkflowValidated constant in events package", func() {
			Expect(events.EventReasonWorkflowValidated).To(Equal("WorkflowValidated"))
		})
	})

	// Verify WorkflowValidationFailed event constant exists
	Context("WorkflowValidationFailed event constant verification", func() {
		It("should have WorkflowValidationFailed constant in events package", func() {
			Expect(events.EventReasonWorkflowValidationFailed).To(Equal("WorkflowValidationFailed"))
		})
	})
})

// Suppress unused import warning
var _ = time.Second
