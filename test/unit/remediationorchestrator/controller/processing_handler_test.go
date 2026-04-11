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
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	prometheus "github.com/prometheus/client_golang/prometheus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/status"
)

var _ = Describe("Issue #666: ProcessingHandler (BR-ORCH-025)", func() {

	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(remediationv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(signalprocessingv1.AddToScheme(scheme)).To(Succeed())
		Expect(aianalysisv1.AddToScheme(scheme)).To(Succeed())
	})

	newHandler := func(c client.Client) *prodcontroller.ProcessingHandler {
		m := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		return prodcontroller.NewProcessingHandler(
			c,
			creator.NewAIAnalysisCreator(c, scheme, m),
			status.NewManager(c, c),
			m,
		)
	}

	// ========================================
	// Interface compliance
	// ========================================
	Describe("Interface compliance", func() {
		It("UT-PRC-H-001: implements PhaseHandler interface", func() {
			var _ phase.PhaseHandler = &prodcontroller.ProcessingHandler{}
		})

		It("UT-PRC-H-002: Phase() returns Processing", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			h := newHandler(c)
			Expect(h.Phase()).To(Equal(phase.Processing))
		})
	})

	// ========================================
	// Corrupted state
	// ========================================
	Describe("Corrupted state", func() {
		It("UT-PRC-H-003: no SignalProcessingRef returns Failed intent", func() {
			rr := newRemediationRequest("prc-no-ref", "default", remediationv1.PhaseProcessing)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr).Build()
			h := newHandler(c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionFailed))
			Expect(intent.FailurePhase).To(Equal(remediationv1.FailurePhaseSignalProcessing))
		})
	})

	// ========================================
	// SP fetch error
	// ========================================
	Describe("SP fetch error", func() {
		It("UT-PRC-H-004: SP CRD not found returns 5s requeue", func() {
			rr := newRemediationRequest("prc-sp-missing", "default", remediationv1.PhaseProcessing)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.SignalProcessingRef = &corev1.ObjectReference{
				Name:      "nonexistent-sp",
				Namespace: "default",
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr).Build()
			h := newHandler(c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})
	})

	// ========================================
	// SP Completed → AIAnalysis created → Advance(Analyzing)
	// ========================================
	Describe("SP Completed (happy path)", func() {
		It("UT-PRC-H-005: SP Completed creates AIAnalysis and returns Advance(Analyzing)", func() {
			rr := newRemediationRequest("prc-happy", "default", remediationv1.PhaseProcessing)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.SignalProcessingRef = &corev1.ObjectReference{
				Name:      "sp-completed",
				Namespace: "default",
			}
			sp := &signalprocessingv1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{Name: "sp-completed", Namespace: "default"},
				Status: signalprocessingv1.SignalProcessingStatus{
					Phase: signalprocessingv1.PhaseCompleted,
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &signalprocessingv1.SignalProcessing{}).
				WithObjects(rr, sp).Build()
			h := newHandler(c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionAdvance))
			Expect(intent.TargetPhase).To(Equal(phase.Analyzing))
		})
	})

	// ========================================
	// SP Completed + AI create error
	// ========================================
	Describe("SP Completed (AI create error)", func() {
		It("UT-PRC-H-006: AI creation error returns 5s requeue", func() {
			rr := newRemediationRequest("prc-ai-err", "default", remediationv1.PhaseProcessing)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.SignalProcessingRef = &corev1.ObjectReference{
				Name:      "sp-completed-err",
				Namespace: "default",
			}
			sp := &signalprocessingv1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{Name: "sp-completed-err", Namespace: "default"},
				Status: signalprocessingv1.SignalProcessingStatus{
					Phase: signalprocessingv1.PhaseCompleted,
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &signalprocessingv1.SignalProcessing{}).
				WithObjects(rr, sp).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if _, ok := obj.(*aianalysisv1.AIAnalysis); ok {
							return fmt.Errorf("simulated AI creation failure")
						}
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()
			h := newHandler(c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})
	})

	// ========================================
	// SP Failed
	// ========================================
	Describe("SP Failed", func() {
		It("UT-PRC-H-007: SP Failed returns Failed intent", func() {
			rr := newRemediationRequest("prc-sp-failed", "default", remediationv1.PhaseProcessing)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			rr.Status.SignalProcessingRef = &corev1.ObjectReference{
				Name:      "sp-failed",
				Namespace: "default",
			}
			sp := &signalprocessingv1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{Name: "sp-failed", Namespace: "default"},
				Status: signalprocessingv1.SignalProcessingStatus{
					Phase: signalprocessingv1.PhaseFailed,
				},
			}
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}, &signalprocessingv1.SignalProcessing{}).
				WithObjects(rr, sp).Build()
			h := newHandler(c)

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionFailed))
			Expect(intent.FailurePhase).To(Equal(remediationv1.FailurePhaseSignalProcessing))
		})
	})

	// ========================================
	// SP In Progress / Non-terminal phases
	// ========================================
	Describe("SP In Progress", func() {
		DescribeTable("UT-PRC-H-008: non-terminal SP phases return 10s requeue",
			func(spPhase string) {
				rr := newRemediationRequest("prc-sp-phase", "default", remediationv1.PhaseProcessing)
				rr.Status.StartTime = ptrMetaTime(time.Now())
				rr.Status.SignalProcessingRef = &corev1.ObjectReference{
					Name:      "sp-phase",
					Namespace: "default",
				}
				sp := &signalprocessingv1.SignalProcessing{
					ObjectMeta: metav1.ObjectMeta{Name: "sp-phase", Namespace: "default"},
					Status: signalprocessingv1.SignalProcessingStatus{
						Phase: signalprocessingv1.SignalProcessingPhase(spPhase),
					},
				}
				c := fake.NewClientBuilder().WithScheme(scheme).
					WithStatusSubresource(&remediationv1.RemediationRequest{}).
					WithObjects(rr, sp).Build()
				h := newHandler(c)

				intent, err := h.Handle(ctx, rr)
				Expect(err).ToNot(HaveOccurred())
				Expect(intent.Type).To(Equal(phase.TransitionNone))
				Expect(intent.RequeueAfter).To(Equal(10 * time.Second))
			},
			Entry("Pending", string(signalprocessingv1.PhasePending)),
			Entry("Processing", "Processing"),
			Entry("empty phase", ""),
			Entry("unknown phase", "SomeUnknownPhase"),
		)
	})
})
