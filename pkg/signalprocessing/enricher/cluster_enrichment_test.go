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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/fleet/fleettest"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

// UT-SP-1511-002: K8sEnricher populates KubernetesContext.Cluster via
// ClusterRegistry, degrading gracefully when the cluster is not registered
// (BR-FLEET-003 R1, #1511, SI-10).
var _ = Describe("K8sEnricher Cluster Classification Labels (BR-FLEET-003, #1511)", func() {
	var (
		ctx         context.Context
		localScheme *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		localScheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(localScheme)).To(Succeed())
	})

	newMetrics := func() *spmetrics.Metrics {
		return spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry())
	}

	It("UT-SP-1511-002a: populates KubernetesContext.Cluster from a registered cluster's labels", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: "prod", Labels: map[string]string{"env": "production"}},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(localScheme).WithObjects(ns).Build()

		querier := &fleettest.StubClusterQuerier{
			Clusters: map[string]registry.ClusterInfo{
				"prod-east-1": {
					ID:     "prod-east-1",
					Labels: map[string]string{"environment": "production", "tier": "gold"},
				},
			},
		}

		e := enricher.NewK8sEnricher(fakeClient, nil, zap.New(zap.UseDevMode(true)), newMetrics(), 5*time.Second, 30*time.Second)
		e.SetClusterRegistry(querier)

		signal := &signalprocessingv1.SignalData{
			ClusterID: "prod-east-1",
			TargetResource: signalprocessingv1.ResourceIdentifier{
				Kind:      "Node",
				Name:      "node-1",
				Namespace: "",
			},
		}

		result, err := e.Enrich(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Cluster).ToNot(BeNil(), "Cluster context must be populated for a registered cluster")
		Expect(result.Cluster.Labels).To(HaveKeyWithValue("environment", "production"))
		Expect(result.Cluster.Labels).To(HaveKeyWithValue("tier", "gold"))
	})

	It("UT-SP-1511-002b: degrades gracefully (nil Cluster, no error) when the cluster is not registered", func() {
		querier := &fleettest.StubClusterQuerier{Clusters: map[string]registry.ClusterInfo{}}

		fakeClient := fake.NewClientBuilder().WithScheme(localScheme).Build()
		e := enricher.NewK8sEnricher(fakeClient, nil, zap.New(zap.UseDevMode(true)), newMetrics(), 5*time.Second, 30*time.Second)
		e.SetClusterRegistry(querier)

		signal := &signalprocessingv1.SignalData{
			ClusterID: "unregistered-cluster",
			TargetResource: signalprocessingv1.ResourceIdentifier{
				Kind: "Node",
				Name: "node-1",
			},
		}

		result, err := e.Enrich(ctx, signal)
		Expect(err).ToNot(HaveOccurred(), "an unregistered cluster must not fail enrichment (graceful degradation)")
		Expect(result).ToNot(BeNil())
		Expect(result.Cluster).To(BeNil(), "Cluster context stays nil when the cluster is not registered")
	})

	It("UT-SP-1511-002c: no ClusterRegistry configured (non-fleet) leaves Cluster nil", func() {
		fakeClient := fake.NewClientBuilder().WithScheme(localScheme).Build()
		e := enricher.NewK8sEnricher(fakeClient, nil, zap.New(zap.UseDevMode(true)), newMetrics(), 5*time.Second, 30*time.Second)
		// No SetClusterRegistry call -- simulates a non-fleet deployment.

		signal := &signalprocessingv1.SignalData{
			ClusterID: "",
			TargetResource: signalprocessingv1.ResourceIdentifier{
				Kind: "Node",
				Name: "node-1",
			},
		}

		result, err := e.Enrich(ctx, signal)
		Expect(err).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Cluster).To(BeNil(), "non-fleet deployments never populate Cluster context")
	})
})
