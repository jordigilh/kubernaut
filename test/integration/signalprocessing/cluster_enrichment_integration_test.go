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

package signalprocessing

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	signalprocessingv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
	"github.com/jordigilh/kubernaut/pkg/signalprocessing/enricher"
	spmetrics "github.com/jordigilh/kubernaut/pkg/signalprocessing/metrics"
)

// IT-SP-1511-001: ClusterRegistry wired at SP startup and reachable through
// enrichment (BR-FLEET-003, #1511, SI-10).
//
// Authority: docs/tests/1511/TEST_PLAN.md, docs/requirements/BR-FLEET-003-cluster-scoped-workflow-targeting.md
//
// Scope: this test constructs a ClusterRegistry via the exact same production
// factory (registry.NewClusterRegistry) and lifecycle (Start/Stop) used by
// cmd/signalprocessing/main.go, backed by a real dynamic-informer watch (not a
// stub) against a fake dynamic client seeded with a Backend CRD -- proving the
// registry's List/Watch machinery is reachable and correctly wired into
// K8sEnricher.Enrich() via the real, production K8sEnricher.SetClusterRegistry
// entry point. Status.ClusterClassification persistence via the full
// SignalProcessing reconcile loop is additionally proven end-to-end by
// IT-SP-1511-002 (Phase 3), which exercises this exact same enrichment
// machinery as a sub-step of the real reconcile.
var _ = Describe("IT-SP-1511-001: ClusterRegistry wiring reachable through K8sEnricher (BR-FLEET-003)", Label("integration", "signalprocessing", "fleet"), func() {

	newBackend := func(name string, labels map[string]interface{}) *unstructured.Unstructured {
		metadataLabels := map[string]interface{}{registry.ManagedLabel: "true"}
		for k, v := range labels {
			metadataLabels[k] = v
		}
		return &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.envoyproxy.io/v1alpha1",
				"kind":       "Backend",
				"metadata": map[string]interface{}{
					"name":      name,
					"namespace": "kubernaut-system",
					"labels":    metadataLabels,
				},
			},
		}
	}

	It("IT-SP-1511-001a: real ClusterRegistry watch resolves cluster labels through K8sEnricher.Enrich()", func() {
		scheme := runtime.NewScheme()
		gvrToListKind := map[schema.GroupVersionResource]string{
			registry.BackendGVR: "BackendList",
		}
		realDynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme, gvrToListKind,
			newBackend("prod-east-1", map[string]interface{}{"environment": "production", "tier": "gold"}),
		)

		logger := zap.New(zap.UseDevMode(true))
		clusterRegistry, err := registry.NewClusterRegistry(
			registry.GatewayEAIGW,
			realDynClient,
			registry.RegistryConfig{Namespace: "kubernaut-system"},
			registry.NewMetricsWithRegistry(prometheus.NewRegistry()),
			logger,
		)
		Expect(err).ToNot(HaveOccurred())
		Expect(clusterRegistry.Start(ctx)).To(Succeed())
		defer clusterRegistry.Stop()

		Eventually(clusterRegistry.Ready, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

		sharedMetrics := spmetrics.NewMetricsWithRegistry(prometheus.NewRegistry())
		e := enricher.NewK8sEnricher(k8sClient, k8sClient, logger, sharedMetrics, 5*time.Second, 30*time.Second)
		e.SetClusterRegistry(clusterRegistry)

		signal := &signalprocessingv1.SignalData{
			ClusterID: "prod-east-1",
			TargetResource: signalprocessingv1.ResourceIdentifier{
				Kind: "Node",
				Name: "node-1",
			},
		}

		result, enrichErr := e.Enrich(ctx, signal)
		Expect(enrichErr).ToNot(HaveOccurred())
		Expect(result).ToNot(BeNil())
		Expect(result.Cluster).ToNot(BeNil(), "ClusterRegistry wiring must reach K8sEnricher.Enrich()")
		Expect(result.Cluster.Labels).To(HaveKeyWithValue("environment", "production"))
		Expect(result.Cluster.Labels).To(HaveKeyWithValue("tier", "gold"))
	})
})
