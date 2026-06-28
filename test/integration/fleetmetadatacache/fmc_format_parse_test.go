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

package fleetmetadatacache_test

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jordigilh/kubernaut/pkg/fleet/mcpclient"
)

// IT-FMC-PARSE: Integration tests for MCP format parsing with real kube-mcp-server.
// Authority: BR-FLEET-002 (Fleet Metadata Caching), ADR-068
// FedRAMP: AC-4 (Information Flow Enforcement) -- proves full-stack format wiring
//
// Pyramid Invariant: IT proves wiring.
// These tests exercise the complete production dispatch path:
//   mcpclient.Client.List() -> MCP protocol (SSE) -> real kube-mcp-server
//   -> K8s API (envtest) -> format response -> parse -> unstructured items
//
// Each test uses a real kube-mcp-server container deployed against envtest,
// configured with either table or yaml output format.
var _ = Describe("IT-FMC-PARSE: Format parsing with real kube-mcp-server (BR-FLEET-002)", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()

		if mcpTableURL == "" || mcpYAMLURL == "" {
			Skip("kube-mcp-server containers not available (mcpTableURL or mcpYAMLURL empty)")
		}
	})

	DescribeTable("IT-FMC-PARSE [AC-4]: List Deployments via real kube-mcp-server",
		func(format string, endpointFn func() string) {
			endpoint := endpointFn()
			nsName := fmt.Sprintf("it-fmc-parse-%s", format)

			By("Creating test namespace in envtest")
			ns := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"metadata":   map[string]any{"name": nsName},
			}}
			_, err := dynClient.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"}).
				Create(ctx, ns, metav1.CreateOptions{})
			if err != nil {
				GinkgoWriter.Printf("Namespace creation: %v (may already exist)\n", err)
			}

			By("Creating test Deployment in envtest")
			dep := &unstructured.Unstructured{Object: map[string]any{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]any{
					"name":      "it-parse-web",
					"namespace": nsName,
					"labels": map[string]any{
						"app":                  "web",
						"kubernaut.ai/managed": "true",
					},
				},
				"spec": map[string]any{
					"replicas": int64(1),
					"selector": map[string]any{
						"matchLabels": map[string]any{"app": "web"},
					},
					"template": map[string]any{
						"metadata": map[string]any{
							"labels": map[string]any{"app": "web"},
						},
						"spec": map[string]any{
							"containers": []any{
								map[string]any{
									"name":  "nginx",
									"image": "nginx:1.27",
								},
							},
						},
					},
				},
			}}
			depGVR := schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
			_, err = dynClient.Resource(depGVR).Namespace(nsName).Create(ctx, dep, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred(), "test Deployment should be created in envtest")

			By(fmt.Sprintf("Connecting mcpclient to kube-mcp-server (%s format)", format))
			c, err := mcpclient.New(ctx, endpoint)
			Expect(err).ToNot(HaveOccurred(), "mcpclient should connect to kube-mcp-server")
			defer c.Close()

			By("Listing Deployments via mcpclient.Client.List()")
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   appsv1.SchemeGroupVersion.Group,
				Version: appsv1.SchemeGroupVersion.Version,
				Kind:    "DeploymentList",
			})
			err = c.List(ctx, list,
				client.InNamespace(nsName),
				client.MatchingLabels{"kubernaut.ai/managed": "true"},
			)
			Expect(err).ToNot(HaveOccurred(), "Client.List() should succeed via real kube-mcp-server")

			By("Verifying parsed items")
			Expect(list.Items).To(HaveLen(1), "should return exactly 1 Deployment")
			item := list.Items[0]
			Expect(item.GetName()).To(Equal("it-parse-web"))
			Expect(item.GetNamespace()).To(Equal(nsName))

			if format == "yaml" {
				Expect(item.GetKind()).To(Equal("Deployment"))
				Expect(item.GetAPIVersion()).To(Equal("apps/v1"))
				Expect(item.GetLabels()).To(HaveKeyWithValue("kubernaut.ai/managed", "true"))
			}

			By("Cleanup: delete test Deployment")
			_ = dynClient.Resource(depGVR).Namespace(nsName).Delete(ctx, "it-parse-web", metav1.DeleteOptions{})
		},
		Entry("IT-FMC-PARSE-001 [AC-4]: table format", "table", func() string { return mcpTableURL + "/mcp" }),
		Entry("IT-FMC-PARSE-002 [AC-4]: yaml format", "yaml", func() string { return mcpYAMLURL + "/mcp" }),
	)
})

var _ = appsv1.SchemeGroupVersion
var _ = corev1.SchemeGroupVersion
