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

// BR-FLEET-003 (#1511): cluster classification is persisted to
// Status.ClusterClassification via the Classifying phase, and a Rego
// evaluation error for the `cluster` dimension MUST NOT transition
// SignalProcessing to PhaseFailed -- unlike severity, this is a non-fatal,
// optional targeting dimension (R2).
package signalprocessing_test

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	signalprocessingv1alpha1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	controller "github.com/jordigilh/kubernaut/internal/controller/signalprocessing"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
	spaudit "github.com/jordigilh/kubernaut/pkg/signalprocessing/audit"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/evaluator"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
	spstatus "github.com/jordigilh/kubernaut/pkg/signalprocessing/status"
)

var _ = Describe("UT-SP-1511: Cluster classification persistence (BR-FLEET-003, #1511)", func() {
	var (
		mockStore   *mockAuditStore
		auditClient *spaudit.AuditClient
		scheme      *runtime.Scheme
	)

	BeforeEach(func() {
		mockStore = &mockAuditStore{}
		auditClient = spaudit.NewAuditClient(mockStore, logr.Discard())

		scheme = runtime.NewScheme()
		_ = signalprocessingv1alpha1.AddToScheme(scheme)
		_ = corev1.AddToScheme(scheme)
	})

	newClassifyingSP := func(name string, cluster *sharedtypes.ClusterContext) *signalprocessingv1alpha1.SignalProcessing {
		return &signalprocessingv1alpha1.SignalProcessing{
			ObjectMeta: metav1.ObjectMeta{
				Name:       name,
				Namespace:  "default",
				Generation: 1,
			},
			Spec: signalprocessingv1alpha1.SignalProcessingSpec{
				Signal: signalprocessingv1alpha1.SignalData{
					Fingerprint: "test-fingerprint-" + name,
					Severity:    "critical",
					ClusterID:   "prod-east-1",
					TargetResource: signalprocessingv1alpha1.ResourceIdentifier{
						Kind:      "Deployment",
						Name:      "test-deploy",
						Namespace: "default",
					},
				},
			},
			Status: signalprocessingv1alpha1.SignalProcessingStatus{
				Phase:     signalprocessingv1alpha1.PhaseClassifying,
				StartTime: &metav1.Time{Time: metav1.Now().Time},
				KubernetesContext: &signalprocessingv1alpha1.KubernetesContext{
					Namespace: &signalprocessingv1alpha1.NamespaceContext{
						Name:   "default",
						Labels: map[string]string{"env": "production"},
					},
					Cluster: cluster,
				},
			},
		}
	}

	It("UT-SP-1511-CLASS-01: persists Status.ClusterClassification when EvaluateCluster returns a classification", func() {
		sp := newClassifyingSP("classify-cluster-ok", &sharedtypes.ClusterContext{
			Labels: map[string]string{"environment": "production"},
		})

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(sp).
			Build()

		mockEval := newDefaultMockPolicyEvaluator()
		mockEval.EvaluateClusterFunc = func(_ context.Context, input evaluator.PolicyInput) (*evaluator.ClusterResult, error) {
			Expect(input.Cluster.Labels).To(HaveKeyWithValue("environment", "production"))
			return &evaluator.ClusterResult{Classification: "production", Source: "rego-policy"}, nil
		}

		reconciler := &controller.SignalProcessingReconciler{
			Client:          fakeClient,
			Scheme:          scheme,
			StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
			Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			AuditManager:    spaudit.NewManager(auditClient),
			PolicyEvaluator: mockEval,
		}

		_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		updated := &signalprocessingv1alpha1.SignalProcessing{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updated)).To(Succeed())
		Expect(updated.Status.ClusterClassification).To(Equal("production"))
		Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCategorizing))
	})

	It("UT-SP-1511-CLASS-02: an EvaluateCluster error is non-fatal -- does NOT transition to PhaseFailed (R2)", func() {
		sp := newClassifyingSP("classify-cluster-err", &sharedtypes.ClusterContext{
			Labels: map[string]string{"environment": "trigger-error"},
		})

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(sp).
			Build()

		mockEval := newDefaultMockPolicyEvaluator()
		mockEval.EvaluateClusterFunc = func(_ context.Context, _ evaluator.PolicyInput) (*evaluator.ClusterResult, error) {
			return nil, fmt.Errorf("malformed cluster rule: non-string output")
		}

		reconciler := &controller.SignalProcessingReconciler{
			Client:          fakeClient,
			Scheme:          scheme,
			StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
			Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			AuditManager:    spaudit.NewManager(auditClient),
			PolicyEvaluator: mockEval,
		}

		_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		updated := &signalprocessingv1alpha1.SignalProcessing{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updated)).To(Succeed())
		Expect(updated.Status.Phase).ToNot(Equal(signalprocessingv1alpha1.PhaseFailed),
			"cluster classification errors must be non-fatal per BR-FLEET-003 R2 (unlike severity)")
		Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCategorizing))
		Expect(updated.Status.ClusterClassification).To(BeEmpty())
	})

	It("UT-SP-1511-CLASS-03: no KubernetesContext.Cluster (non-fleet) leaves Status.ClusterClassification empty", func() {
		sp := newClassifyingSP("classify-cluster-nonfleet", nil)

		fakeClient := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(sp).
			WithStatusSubresource(sp).
			Build()

		reconciler := &controller.SignalProcessingReconciler{
			Client:          fakeClient,
			Scheme:          scheme,
			StatusManager:   spstatus.NewManager(fakeClient, fakeClient),
			Metrics:         spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry()),
			AuditManager:    spaudit.NewManager(auditClient),
			PolicyEvaluator: newDefaultMockPolicyEvaluator(),
		}

		_, err := reconciler.Reconcile(context.Background(), reconcile.Request{
			NamespacedName: types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace},
		})
		Expect(err).ToNot(HaveOccurred())

		updated := &signalprocessingv1alpha1.SignalProcessing{}
		Expect(fakeClient.Get(context.Background(), types.NamespacedName{Name: sp.Name, Namespace: sp.Namespace}, updated)).To(Succeed())
		Expect(updated.Status.ClusterClassification).To(BeEmpty())
		Expect(updated.Status.Phase).To(Equal(signalprocessingv1alpha1.PhaseCategorizing))
	})
})
