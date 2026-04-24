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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	prodcontroller "github.com/jordigilh/kubernaut/internal/controller/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/config"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/creator"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/phase"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/routing"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/status"
)

type preAnalysisBlockEngine struct {
	MockRoutingEngine
	block *routing.BlockingCondition
}

func (e *preAnalysisBlockEngine) CheckPreAnalysisConditions(_ context.Context, _ *remediationv1.RemediationRequest) (*routing.BlockingCondition, error) {
	return e.block, nil
}

var _ = Describe("Issue #666: PendingHandler (BR-ORCH-025)", func() {

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
	})

	newHandler := func(c client.Client, re routing.Engine) *prodcontroller.PendingHandler {
		m := rometrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		return prodcontroller.NewPendingHandler(
			c,
			re,
			creator.NewSignalProcessingCreator(c, scheme, m),
			status.NewManager(c, c),
			m,
		)
	}

	// ========================================
	// Interface compliance
	// ========================================
	Describe("Interface compliance", func() {
		It("UT-PND-H-001: implements PhaseHandler interface", func() {
			var _ phase.PhaseHandler = &prodcontroller.PendingHandler{}
		})

		It("UT-PND-H-002: Phase() returns Pending", func() {
			c := fake.NewClientBuilder().WithScheme(scheme).Build()
			h := newHandler(c, &MockRoutingEngine{})
			Expect(h.Phase()).To(Equal(phase.Pending))
		})
	})

	// ========================================
	// Routing error
	// ========================================
	Describe("Routing error", func() {
		It("UT-PND-H-003: routing engine error returns 5s requeue", func() {
			rr := newRemediationRequest("pnd-route-err", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr).Build()
			h := newHandler(c, &ErrorRoutingEngine{
				PreAnalysisErr: fmt.Errorf("simulated routing failure"),
			})

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})
	})

	// ========================================
	// Routing blocked
	// ========================================
	Describe("Routing blocked", func() {
		It("UT-PND-H-004: routing blocked returns Block intent", func() {
			rr := newRemediationRequest("pnd-blocked", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr).Build()
			h := newHandler(c, &preAnalysisBlockEngine{
				block: &routing.BlockingCondition{
					Blocked:      true,
					Reason:       "DuplicateInProgress",
					Message:      "Duplicate RR active",
					RequeueAfter: 30 * time.Second,
				},
			})

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionBlocked))
			Expect(intent.Block.Reason).To(Equal("DuplicateInProgress"))
			Expect(intent.Block.FromPhase).To(Equal(phase.Pending))
		})
	})

	// ========================================
	// SP creation error: namespace terminating
	// ========================================
	Describe("Namespace terminating", func() {
		It("UT-PND-H-007: namespace terminating returns NoOp", func() {
			rr := newRemediationRequest("pnd-ns-term", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if _, ok := obj.(*signalprocessingv1.SignalProcessing); ok {
							return fmt.Errorf("namespace \"default\" is being terminated")
						}
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()
			h := newHandler(c, &MockRoutingEngine{})

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.IsNoOp()).To(BeTrue())
		})
	})

	// ========================================
	// SP creation error
	// ========================================
	Describe("SP creation error", func() {
		It("UT-PND-H-005: SP create error returns 5s requeue", func() {
			rr := newRemediationRequest("pnd-sp-err", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr).
				WithInterceptorFuncs(interceptor.Funcs{
					Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
						if _, ok := obj.(*signalprocessingv1.SignalProcessing); ok {
							return fmt.Errorf("simulated SP creation failure")
						}
						return c.Create(ctx, obj, opts...)
					},
				}).
				Build()
			h := newHandler(c, &MockRoutingEngine{})

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionNone))
			Expect(intent.RequeueAfter).To(Equal(config.RequeueGenericError))
		})
	})

	// ========================================
	// Happy path: SP created → Processing
	// ========================================
	Describe("Happy path", func() {
		It("UT-PND-H-006: SP created successfully returns Advance(Processing)", func() {
			rr := newRemediationRequest("pnd-happy", "default", remediationv1.PhasePending)
			rr.Status.StartTime = ptrMetaTime(time.Now())
			c := fake.NewClientBuilder().WithScheme(scheme).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				WithObjects(rr).Build()
			h := newHandler(c, &MockRoutingEngine{})

			intent, err := h.Handle(ctx, rr)
			Expect(err).ToNot(HaveOccurred())
			Expect(intent.Type).To(Equal(phase.TransitionAdvance))
			Expect(intent.TargetPhase).To(Equal(phase.Processing))
		})
	})
})
