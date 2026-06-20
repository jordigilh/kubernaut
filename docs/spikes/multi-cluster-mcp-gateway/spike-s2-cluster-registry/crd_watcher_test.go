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

package registry_test

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/jordigilh/kubernaut/docs/spikes/multi-cluster-mcp-gateway/spike-s2-cluster-registry"
)

func TestCRDWatcher(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CRD Watcher Suite — WS4 Spike S2 (Corrected)")
}

var _ = Describe("CRDWatcher — Spike S2 (Label-based discovery)", func() {

	var (
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	Describe("UT-FLEET-S2-001: discovers MCPServerRegistration with managed label", func() {
		It("should include clusters labeled kubernaut.ai/managed=true", func() {
			objs := []runtime.Object{
				newMCPServerReg("cluster-a", map[string]string{
					"kubernaut.ai/managed": "true",
				}, map[string]string{}, "cluster_a_", true),
			}
			reader := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
			watcher := registry.NewCRDWatcher(reader, "kubernaut-mcp", logr.Discard())

			Expect(watcher.Reconcile(ctx)).To(Succeed())
			clusters, err := watcher.ListClusters(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusters).To(HaveLen(1))
			Expect(clusters[0].ID).To(Equal("cluster-a"))
			Expect(clusters[0].ToolPrefix).To(Equal("cluster_a_"))
			Expect(clusters[0].Status).To(Equal(registry.ClusterStatusReady))
		})
	})

	Describe("UT-FLEET-S2-002: ignores MCPServerRegistration without managed label", func() {
		It("should exclude clusters without the label", func() {
			objs := []runtime.Object{
				newMCPServerReg("cluster-a", map[string]string{
					"kubernaut.ai/managed": "true",
				}, map[string]string{}, "cluster_a_", true),
				newMCPServerReg("cluster-b", map[string]string{
					"app": "other",
				}, map[string]string{}, "cluster_b_", true),
			}
			reader := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
			watcher := registry.NewCRDWatcher(reader, "kubernaut-mcp", logr.Discard())

			Expect(watcher.Reconcile(ctx)).To(Succeed())
			clusters, err := watcher.ListClusters(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(clusters).To(HaveLen(1))
			Expect(clusters[0].ID).To(Equal("cluster-a"))
		})
	})

	Describe("UT-FLEET-S2-003: extracts ClusterInfo fields correctly", func() {
		It("should populate prefix, status, and JWT audience from annotations", func() {
			objs := []runtime.Object{
				newMCPServerReg("prod-east", map[string]string{
					"kubernaut.ai/managed": "true",
					"env":                  "production",
				}, map[string]string{
					"kubernaut.ai/jwt-audience": "kubernaut-mcp-prod",
				}, "prod_east_", true),
			}
			reader := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
			watcher := registry.NewCRDWatcher(reader, "kubernaut-mcp", logr.Discard())

			Expect(watcher.Reconcile(ctx)).To(Succeed())
			cluster, err := watcher.GetCluster(ctx, "prod-east")
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.ToolPrefix).To(Equal("prod_east_"))
			Expect(cluster.JWTAudience).To(Equal("kubernaut-mcp-prod"))
			Expect(cluster.Labels).To(HaveKeyWithValue("env", "production"))
			Expect(cluster.Status).To(Equal(registry.ClusterStatusReady))
		})
	})

	Describe("UT-FLEET-S2-004: emits ClusterRemoved when label is removed", func() {
		It("should remove the cluster from registry on next reconcile", func() {
			managed := newMCPServerReg("cluster-a", map[string]string{
				"kubernaut.ai/managed": "true",
			}, map[string]string{}, "cluster_a_", true)

			reader := fake.NewClientBuilder().WithRuntimeObjects(managed).Build()
			watcher := registry.NewCRDWatcher(reader, "kubernaut-mcp", logr.Discard())

			eventCh, err := watcher.WatchClusters(ctx)
			Expect(err).NotTo(HaveOccurred())

			Expect(watcher.Reconcile(ctx)).To(Succeed())
			Expect(eventCh).To(Receive()) // ClusterAdded

			// Simulate label removal by reconciling with empty list
			emptyReader := fake.NewClientBuilder().Build()
			watcher2 := registry.NewCRDWatcher(emptyReader, "kubernaut-mcp", logr.Discard())
			// Copy state from first watcher to simulate same watcher seeing empty
			_ = watcher2

			// Directly test: reconcile watcher with no matching CRDs
			readerNoMatch := fake.NewClientBuilder().Build()
			watcherRecon := registry.NewCRDWatcher(readerNoMatch, "kubernaut-mcp", logr.Discard())
			// Pre-populate to simulate previous state
			watcherRecon.SetClustersForTest(map[string]registry.ClusterInfo{
				"cluster-a": {ID: "cluster-a", ToolPrefix: "cluster_a_"},
			})
			evCh2, _ := watcherRecon.WatchClusters(ctx)
			Expect(watcherRecon.Reconcile(ctx)).To(Succeed())

			var ev registry.ClusterEvent
			Eventually(evCh2).Should(Receive(&ev))
			Expect(ev.Type).To(Equal(registry.ClusterRemoved))
			Expect(ev.Cluster.ID).To(Equal("cluster-a"))
		})
	})

	Describe("UT-FLEET-S2-005: handles missing/degraded status conditions", func() {
		It("should report Offline when no status conditions exist", func() {
			objs := []runtime.Object{
				newMCPServerReg("cluster-new", map[string]string{
					"kubernaut.ai/managed": "true",
				}, map[string]string{}, "cluster_new_", false),
			}
			reader := fake.NewClientBuilder().WithRuntimeObjects(objs...).Build()
			watcher := registry.NewCRDWatcher(reader, "kubernaut-mcp", logr.Discard())

			Expect(watcher.Reconcile(ctx)).To(Succeed())
			cluster, err := watcher.GetCluster(ctx, "cluster-new")
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Status).To(Equal(registry.ClusterStatusOffline))
		})
	})
})

// newMCPServerReg creates an unstructured MCPServerRegistration for testing.
func newMCPServerReg(name string, labels, annotations map[string]string, prefix string, ready bool) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "mcp.kuadrant.io/v1alpha1",
			"kind":       "MCPServerRegistration",
			"metadata": map[string]any{
				"name":        name,
				"namespace":   "kubernaut-mcp",
				"labels":      toAnyMap(labels),
				"annotations": toAnyMap(annotations),
			},
			"spec": map[string]any{
				"prefix": prefix,
				"targetRef": map[string]any{
					"group":     "gateway.networking.k8s.io",
					"kind":      "HTTPRoute",
					"name":      name + "-route",
					"namespace": "kubernaut-mcp",
				},
			},
		},
	}

	if ready {
		obj.Object["status"] = map[string]any{
			"conditions": []any{
				map[string]any{
					"type":   "Ready",
					"status": "True",
				},
			},
		}
	}

	return obj
}

func toAnyMap(m map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
