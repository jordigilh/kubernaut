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

package enricher_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

// UT-SP-054-KINDS: Table-driven test for K8sEnricher local signal kind branches
// Authority: BR-SP-001 (K8s Context Enrichment)
// FedRAMP: SI-4 (Information System Monitoring) -- context collection for all resource types
var _ = Describe("UT-SP-054-KINDS: K8sEnricher local signal kind branches", func() {
	var (
		ctx     context.Context
		scheme  *runtime.Scheme
		testNS  *corev1.Namespace
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(appsv1.AddToScheme(scheme)).To(Succeed())

		testNS = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "default",
				Labels: map[string]string{"kubernaut.ai/managed": "true"},
			},
		}
	})

	DescribeTable("should enrich local signals for each resource kind",
		func(kind, name, namespace string, obj client.Object) {
			reg := prometheus.NewPedanticRegistry()
			m := spmetrics.NewMetricsWithRegistry(reg)
			logger := zap.New(zap.UseDevMode(true))

			builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(testNS)
			if obj != nil {
				builder = builder.WithObjects(obj)
			}
			fakeClient := builder.Build()

			e := enricher.NewK8sEnricher(fakeClient, fakeClient, logger, m,
				5*time.Second, 30*time.Second)

			signal := &signalprocessingv1.SignalData{
				TargetResource: signalprocessingv1.ResourceIdentifier{
					Kind:      kind,
					Name:      name,
					Namespace: namespace,
				},
			}

			result, err := e.Enrich(ctx, signal)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())

			if kind == "Node" {
				Expect(result.Namespace).To(BeNil(),
					"Node signals should not have namespace context")
			} else if kind == "CustomKind" {
				Expect(result.Namespace).ToNot(BeNil(),
					"Unknown kinds should still have namespace context")
				Expect(result.Workload).To(BeNil(),
					"Unknown kinds should not have workload context")
			} else {
				Expect(result.Namespace).ToNot(BeNil(),
					kind+" signals should include namespace context")
				Expect(result.Workload).ToNot(BeNil(),
					kind+" signals should include workload context")
				Expect(result.Workload.Kind).To(Equal(kind))
				Expect(result.Workload.Name).To(Equal(name))
			}
		},
		Entry("Pod kind", "Pod", "nginx", "default",
			&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
				Name: "nginx", Namespace: "default",
				Labels: map[string]string{"app": "web"},
			}},
		),
		Entry("Deployment kind", "Deployment", "api-server", "default",
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
				Name: "api-server", Namespace: "default",
				Labels: map[string]string{"app": "api"},
			}},
		),
		Entry("StatefulSet kind", "StatefulSet", "postgres", "default",
			&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{
				Name: "postgres", Namespace: "default",
				Labels: map[string]string{"app": "db"},
			}},
		),
		Entry("DaemonSet kind", "DaemonSet", "fluentd", "default",
			&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{
				Name: "fluentd", Namespace: "default",
				Labels: map[string]string{"app": "logging"},
			}},
		),
		Entry("ReplicaSet kind", "ReplicaSet", "nginx-rs", "default",
			&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{
				Name: "nginx-rs", Namespace: "default",
				Labels: map[string]string{"app": "web"},
			}},
		),
		Entry("Service kind", "Service", "api-svc", "default",
			&corev1.Service{ObjectMeta: metav1.ObjectMeta{
				Name: "api-svc", Namespace: "default",
				Labels: map[string]string{"app": "api"},
			}},
		),
		Entry("Node kind", "Node", "worker-1", "",
			&corev1.Node{ObjectMeta: metav1.ObjectMeta{
				Name:   "worker-1",
				Labels: map[string]string{"role": "worker"},
			}},
		),
		Entry("Unknown kind falls back to namespace only", "CustomKind", "custom-resource", "default",
			nil,
		),
	)
})
