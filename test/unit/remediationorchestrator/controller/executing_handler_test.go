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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheus "github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/aggregator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/status"
)

var _ = Describe("Issue #666: ExecutingHandler (BR-ORCH-025)", func() {

	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(workflowexecutionv1.AddToScheme(scheme)).To(Succeed())
	})

	newHandler := func(c client.Client, apiReader client.Reader) *prodcontroller.ExecutingHandler {
		return prodcontroller.NewExecutingHandler(
			c,
			apiReader,
			aggregator.NewStatusAggregator(c),
			status.NewManager(c, apiReader),
			rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			nil, nil,
		)
	}

	// ========================================
	// Interface compliance
	// ========================================
	Describe("Interface compliance", func() {
		It("UT-EXE-001: implements PhaseHandler interface", func() {
			var _ phase.PhaseHandler = &prodcontroller.ExecutingHandler{}
		})

		It("UT-EXE-002: Phase() returns Executing", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			h := newHandler(c, c)
			Expect(h.Phase()).To(Equal(phase.Executing))
		})
	})

	// ========================================
	// Corrupted state: no WE ref
	// ========================================
	Describe("Corrupted state", func() {
		It("UT-EXE-003: no WorkflowExecutionRef returns Failed intent", func() {
			rr := newRemediationRequest("exe-no-ref", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionFailed))
			Expect(intent.FailurePhase).To(Equal(remediationv1.FailurePhaseWorkflowExecution))
		})
	})

	// ========================================
	// Missing child CRD
	// ========================================
	Describe("Missing child CRD", func() {
		It("UT-EXE-004: WE CRD not found returns Failed intent", func() {
			rr := newRemediationRequest("exe-missing-we", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "nonexistent-we",
				Namespace: "default",
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionFailed))
			Expect(intent.FailurePhase).To(Equal(remediationv1.FailurePhaseWorkflowExecution))
		})
	})

	// ========================================
	// WE Completed → Verifying
	// ========================================
	Describe("WE Completed", func() {
		It("UT-EXE-005: WE Completed returns Verifying intent", func() {
			rr := newRemediationRequest("exe-completed", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "we-completed",
				Namespace: "default",
			}
			we := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "we-completed", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseCompleted,
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr, we).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionVerifying))
			Expect(intent.Outcome).To(Equal("Remediated"))
		})
	})

	// ========================================
	// WE Failed (normal)
	// ========================================
	Describe("WE Failed (normal)", func() {
		It("UT-EXE-006: WE Failed returns Failed intent", func() {
			rr := newRemediationRequest("exe-failed", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "we-failed",
				Namespace: "default",
			}
			we := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "we-failed", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseFailed,
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr, we).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionFailed))
			Expect(intent.FailurePhase).To(Equal(remediationv1.FailurePhaseWorkflowExecution))
		})
	})

	// ========================================
	// WE Failed + Deduplicated
	// ========================================
	Describe("WE Failed (Deduplicated)", func() {
		It("UT-EXE-007: WE Deduplicated sets DeduplicatedByWE and requeues", func() {
			rr := newRemediationRequest("exe-dedup", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "we-dedup",
				Namespace: "default",
			}
			we := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "we-dedup", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseFailed,
					FailureDetails: &workflowexecutionv1.FailureDetails{
						Reason: workflowexecutionv1.FailureReasonDeduplicated,
					},
					DeduplicatedBy: "original-we",
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &workflowexecutionv1.WorkflowExecution{}).
				WithObjects(rr, we).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(10 * time.Second))

			var updatedRR remediationv1.RemediationRequest
			Expect(c.Get(ctx, types.NamespacedName{Name: "exe-dedup", Namespace: "default"}, &updatedRR)).To(Succeed())
			Expect(updatedRR.Status.DeduplicatedByWE).To(Equal("original-we"))
		})
	})

	// ========================================
	// WE In Progress (Pending/Running)
	// ========================================
	Describe("WE In Progress", func() {
		It("UT-EXE-008: WE Pending returns 10s requeue", func() {
			rr := newRemediationRequest("exe-pending", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "we-pending",
				Namespace: "default",
			}
			we := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "we-pending", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhasePending,
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr, we).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(10 * time.Second))
		})
	})

	Describe("WE In Progress (Running)", func() {
		It("UT-EXE-013: WE Running returns 10s requeue", func() {
			rr := newRemediationRequest("exe-running", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "we-running",
				Namespace: "default",
			}
			we := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "we-running", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseRunning,
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr, we).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(10 * time.Second))
		})
	})

	Describe("WE Empty Phase", func() {
		It("UT-EXE-014: WE empty phase returns 10s requeue", func() {
			rr := newRemediationRequest("exe-empty", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "we-empty",
				Namespace: "default",
			}
			we := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "we-empty", Namespace: "default"},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr, we).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(10 * time.Second))
		})
	})

	Describe("WE Unknown Phase", func() {
		It("UT-EXE-015: WE unknown phase returns 10s requeue", func() {
			rr := newRemediationRequest("exe-unknown", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "we-unknown",
				Namespace: "default",
			}
			we := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "we-unknown", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: "SomeUnknownPhase",
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr, we).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(10 * time.Second))
		})
	})

	// ========================================
	// Dedup result propagation
	// ========================================
	Describe("Dedup result propagation", func() {
		It("UT-EXE-009: original WFE Completed returns InheritedCompleted", func() {
			rr := newRemediationRequest("exe-dedup-complete", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.DeduplicatedByWE = "original-we"
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "dedup-we",
				Namespace: "default",
			}
			dedupWE := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "dedup-we", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseFailed,
				},
			}
			originalWE := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "original-we", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseCompleted,
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr, dedupWE, originalWE).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionInheritedCompleted))
			Expect(intent.SourceRef).To(Equal("original-we"))
			Expect(intent.SourceKind).To(Equal("WorkflowExecution"))
		})

		It("UT-EXE-010: original WFE Failed returns InheritedFailed", func() {
			rr := newRemediationRequest("exe-dedup-fail", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.DeduplicatedByWE = "original-we-fail"
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "dedup-we-fail",
				Namespace: "default",
			}
			dedupWE := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "dedup-we-fail", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseFailed,
				},
			}
			originalWE := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "original-we-fail", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase:         workflowexecutionv1.PhaseFailed,
					FailureReason: "pipeline crashed",
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr, dedupWE, originalWE).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionInheritedFailed))
			Expect(intent.SourceRef).To(Equal("original-we-fail"))
			Expect(intent.SourceKind).To(Equal("WorkflowExecution"))
		})

		It("UT-EXE-011: original WFE deleted returns InheritedFailed", func() {
			rr := newRemediationRequest("exe-dedup-deleted", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.DeduplicatedByWE = "deleted-original-we"
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "dedup-we-deleted",
				Namespace: "default",
			}
			dedupWE := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "dedup-we-deleted", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseFailed,
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr, dedupWE).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionInheritedFailed))
			Expect(intent.SourceRef).To(Equal("deleted-original-we"))
			Expect(intent.SourceKind).To(Equal("WorkflowExecution"))
		})

		It("UT-EXE-012: original WFE still running returns 10s requeue", func() {
			rr := newRemediationRequest("exe-dedup-running", "default", remediationv1.PhaseExecuting)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.DeduplicatedByWE = "running-original-we"
			rr.Status.WorkflowExecutionRef = &corev1.ObjectReference{
				Name:      "dedup-we-running",
				Namespace: "default",
			}
			dedupWE := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "dedup-we-running", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseFailed,
				},
			}
			originalWE := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "running-original-we", Namespace: "default"},
				Status: workflowexecutionv1.WorkflowExecutionStatus{
					Phase: workflowexecutionv1.PhaseRunning,
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr, dedupWE, originalWE).Build()
			h := newHandler(c, c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(10 * time.Second))
		})
	})
})
