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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

func TestFleetRegistry(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet Registry Suite")
}

var _ = Describe("extractClusterInfo (BR-INTEGRATION-065)", func() {

	It("UT-FLEET-003-001: populates all ClusterInfo fields including MCPEndpoint from status", func() {
		u := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "mcp.kuadrant.io/v1alpha1",
				"kind":       "MCPServerRegistration",
				"metadata": map[string]interface{}{
					"name":      "prod-east-1",
					"namespace": "kuadrant-system",
					"labels": map[string]interface{}{
						"kubernaut.ai/managed": "true",
						"env":                  "production",
					},
					"annotations": map[string]interface{}{
						"kubernaut.ai/cluster-name": "Production US-East",
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
		Expect(info.Name).To(Equal("Production US-East"))
		Expect(info.MCPEndpoint).To(Equal("https://mcp-gateway.example.com/clusters/prod-east-1/mcp"))
		Expect(info.Namespace).To(Equal("kuadrant-system"))
		Expect(info.Labels).To(HaveKeyWithValue("kubernaut.ai/managed", "true"))
		Expect(info.Labels).To(HaveKeyWithValue("env", "production"))
	})

	It("UT-FLEET-003-002: returns error if name is empty (schema drift protection)", func() {
		u := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "mcp.kuadrant.io/v1alpha1",
				"kind":       "MCPServerRegistration",
				"metadata": map[string]interface{}{
					"name":      "",
					"namespace": "kuadrant-system",
				},
			},
		}

		_, err := registry.ExtractClusterInfo(u)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("empty name"))
	})

	It("UT-FLEET-003-003: falls back to name as display name when annotation missing", func() {
		u := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "mcp.kuadrant.io/v1alpha1",
				"kind":       "MCPServerRegistration",
				"metadata": map[string]interface{}{
					"name":      "staging-west",
					"namespace": "kuadrant-system",
					"labels": map[string]interface{}{
						"kubernaut.ai/managed": "true",
					},
				},
			},
		}

		info, err := registry.ExtractClusterInfo(u)
		Expect(err).ToNot(HaveOccurred())
		Expect(info.Name).To(Equal("staging-west"))
	})

	It("UT-FLEET-003-004: extracts MCPEndpoint from spec.endpoint when status.endpoint absent", func() {
		u := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "mcp.kuadrant.io/v1alpha1",
				"kind":       "MCPServerRegistration",
				"metadata": map[string]interface{}{
					"name":      "dev-local",
					"namespace": "kuadrant-system",
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
