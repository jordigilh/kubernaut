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
	"fmt"
	"reflect"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

func TestFleetRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet Registry Suite")
}

var _ = Describe("extractClusterInfo (BR-INTEGRATION-065)", func() {

	// Issue #1651: ClusterInfo.Name (and the kubernaut.ai/cluster-name
	// annotation that populated it) were removed — non-unique, unsafe for
	// disambiguation. ClusterInfo.ID only.
	It("UT-FLEET-1651-001: Name field has been removed from ClusterInfo", func() {
		_, found := reflect.TypeOf(registry.ClusterInfo{}).FieldByName("Name")
		Expect(found).To(BeFalse(), "ClusterInfo.Name must not exist (issue #1651: non-unique, unsafe for disambiguation)")
	})

	It("UT-FLEET-003-001: populates all ClusterInfo fields including MCPEndpoint from status", func() {
		u := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.envoyproxy.io/v1alpha1",
				"kind":       "Backend",
				"metadata": map[string]interface{}{
					"name":      "prod-east-1",
					"namespace": "kubernaut-system",
					"labels": map[string]interface{}{
						"kubernaut.ai/managed": "true",
						"env":                  "production",
					},
				},
				"status": map[string]interface{}{
					"endpoint": "https://mcp-gateway.example.com/clusters/prod-east-1/mcp",
				},
			},
		}

		info, err := registry.ExtractClusterInfo(u)
		Expect(err).ToNot(HaveOccurred())
		Expect(info.ID).To(Equal("prod-east-1"))
		Expect(info.MCPEndpoint).To(Equal("https://mcp-gateway.example.com/clusters/prod-east-1/mcp"))
		Expect(info.Namespace).To(Equal("kubernaut-system"))
		Expect(info.Labels).To(HaveKeyWithValue("kubernaut.ai/managed", "true"))
		Expect(info.Labels).To(HaveKeyWithValue("env", "production"))
	})

	It("UT-FLEET-003-002: returns error if name is empty (schema drift protection)", func() {
		u := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.envoyproxy.io/v1alpha1",
				"kind":       "Backend",
				"metadata": map[string]interface{}{
					"name":      "",
					"namespace": "kubernaut-system",
				},
			},
		}

		_, err := registry.ExtractClusterInfo(u)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("empty name"))
	})

	It("UT-REG-EAIGW-001 [AC-3]: ExtractClusterInfo sets ToolPrefix to {name}__ for EAIGW Backend CRDs", func() {
		u := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.envoyproxy.io/v1alpha1",
				"kind":       "Backend",
				"metadata": map[string]interface{}{
					"name":      "prod-east-1",
					"namespace": "kubernaut-system",
					"labels": map[string]interface{}{
						"kubernaut.ai/managed": "true",
					},
				},
				"status": map[string]interface{}{
					"endpoint": "https://mcp.example.com/prod-east-1/mcp",
				},
			},
		}

		info, err := registry.ExtractClusterInfo(u)
		Expect(err).ToNot(HaveOccurred())
		Expect(info.ToolPrefix).To(Equal("prod-east-1__"),
			"EAIGW ToolPrefix must follow {name}__ convention")
	})

	It("UT-FLEET-003-004: extracts MCPEndpoint from spec.endpoint when status.endpoint absent", func() {
		u := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "gateway.envoyproxy.io/v1alpha1",
				"kind":       "Backend",
				"metadata": map[string]interface{}{
					"name":      "dev-local",
					"namespace": "kubernaut-system",
					"labels": map[string]interface{}{
						"kubernaut.ai/managed": "true",
					},
				},
				"spec": map[string]interface{}{
					"endpoint": "http://localhost:8080/mcp",
				},
			},
		}

		info, err := registry.ExtractClusterInfo(u)
		Expect(err).ToNot(HaveOccurred())
		Expect(info.MCPEndpoint).To(Equal("http://localhost:8080/mcp"))
	})
})

var _ = Describe("EAIGWRegistry.Start (BR-INTEGRATION-065)", func() {
	// UT-FLEET-003-006: guards against the data race between Start()'s read
	// of w.clusters (for the "started and synced" log line) and the
	// mutex-protected write in onAdd(). client-go's WaitForCacheSync only
	// guarantees the reflector's initial List has populated the informer's
	// internal store; it does NOT guarantee that the sharedProcessor has
	// finished dispatching AddFunc for every pre-existing object (handler
	// dispatch runs on separate processorListener goroutines -- see
	// k8s.io/client-go/tools/cache/shared_informer.go). Exercising Start()
	// with pre-existing Backends under `go test -race` is the regression
	// guard for that race (caught in CI: PR #1539 post-merge run).
	It("UT-FLEET-003-006: Start() succeeds and does not race when pre-existing Backends are synced", func() {
		scheme := runtime.NewScheme()
		fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
			map[schema.GroupVersionResource]string{
				registry.BackendGVR: "BackendList",
			},
		)

		for i := 0; i < 20; i++ {
			backend := &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "gateway.envoyproxy.io/v1alpha1",
					"kind":       "Backend",
					"metadata": map[string]interface{}{
						"name":      fmt.Sprintf("cluster-%d", i),
						"namespace": "kubernaut-system",
						"labels": map[string]interface{}{
							"kubernaut.ai/managed": "true",
						},
					},
					"status": map[string]interface{}{
						"endpoint": fmt.Sprintf("https://mcp.example.com/cluster-%d/mcp", i),
					},
				},
			}
			_, err := fakeClient.Resource(registry.BackendGVR).Namespace("kubernaut-system").Create(
				context.Background(), backend, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())
		}

		reg := registry.NewEAIGWRegistry(fakeClient, registry.EAIGWRegistryConfig{}, nil, logr.Discard())

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		Expect(reg.Start(ctx)).To(Succeed())
		reg.Stop()
	})
})

var _ = Describe("Metrics (BR-INTEGRATION-065)", func() {
	It("UT-FLEET-003-005: nil-safe methods do not panic", func() {
		var m *registry.Metrics
		Expect(func() {
			m.NilSafeIncReconcile()
			m.NilSafeIncReconcileError()
			m.NilSafeSetClusters(5)
			m.NilSafeIncEventDrop()
		}).ToNot(Panic())
	})
})
