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

package skip

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	workflowexecutionv1 "github.com/jordigilh/kubernaut/api/workflowexecution/v1alpha1"
	skiphandler "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/handler/skip"
	rometrics "github.com/jordigilh/kubernaut/pkg/remediationorchestrator/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

func TestSkipHandlers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Skip Handlers Suite")
}

func setupScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = remediationv1.AddToScheme(s)
	_ = signalprocessingv1.AddToScheme(s)
	_ = workflowexecutionv1.AddToScheme(s)
	return s
}

var _ = Describe("Issue #612: Skip handlers must set CompletedAt on PhaseSkipped", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = setupScheme()
	})

	Context("ResourceBusyHandler", func() {
		It("UT-RO-612-001: should set CompletedAt when transitioning to PhaseSkipped", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rr-busy",
					Namespace: "default",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhasePending,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			handlerCtx := &skiphandler.Context{
				Client:  fakeClient,
				Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			}

			handler := skiphandler.NewResourceBusyHandler(handlerCtx)
			we := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "we-test", Namespace: "default"},
			}
			sp := &signalprocessingv1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{Name: "sp-test", Namespace: "default"},
			}

			_, err := handler.Handle(ctx, rr, we, sp)
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr-busy", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseSkipped),
				"Precondition: RR must be in PhaseSkipped")
			Expect(updated.Status.CompletedAt).NotTo(BeNil(),
				"Behavior: CompletedAt must be set when RR transitions to PhaseSkipped via ResourceBusy")
		})
	})

	Context("RecentlyRemediatedHandler", func() {
		It("UT-RO-612-002: should set CompletedAt when transitioning to PhaseSkipped", func() {
			rr := &remediationv1.RemediationRequest{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-rr-recent",
					Namespace: "default",
				},
				Status: remediationv1.RemediationRequestStatus{
					OverallPhase: remediationv1.PhasePending,
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(rr).
				WithStatusSubresource(&remediationv1.RemediationRequest{}).
				Build()

			handlerCtx := &skiphandler.Context{
				Client:  fakeClient,
				Metrics: rometrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			}

			handler := skiphandler.NewRecentlyRemediatedHandler(handlerCtx)
			we := &workflowexecutionv1.WorkflowExecution{
				ObjectMeta: metav1.ObjectMeta{Name: "we-test", Namespace: "default"},
			}
			sp := &signalprocessingv1.SignalProcessing{
				ObjectMeta: metav1.ObjectMeta{Name: "sp-test", Namespace: "default"},
			}

			_, err := handler.Handle(ctx, rr, we, sp)
			Expect(err).NotTo(HaveOccurred())

			updated := &remediationv1.RemediationRequest{}
			Expect(fakeClient.Get(ctx, types.NamespacedName{Name: "test-rr-recent", Namespace: "default"}, updated)).To(Succeed())

			Expect(updated.Status.OverallPhase).To(Equal(remediationv1.PhaseSkipped),
				"Precondition: RR must be in PhaseSkipped")
			Expect(updated.Status.CompletedAt).NotTo(BeNil(),
				"Behavior: CompletedAt must be set when RR transitions to PhaseSkipped via RecentlyRemediated")
		})
	})
})
