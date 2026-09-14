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

package tools_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/go-logr/logr"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	mcptools "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp/tools"
)

var _ = Describe("kubernaut_select_workflow MCP output schema", func() {
	It("IT-KA-2390-001: accepts serialized Kubernetes resource quantities", func() {
		workflowID := "wf-schema-2390"
		catalog := &mockWorkflowCatalog{
			workflow: &mcptools.CatalogWorkflow{
				WorkflowID: workflowID,
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
				},
			},
		}
		sessions := &mockSessionManager{
			isActive: true,
			getDriverResult: &mcpinternal.InteractiveSession{
				SessionID:       "sess-schema-2390",
				CorrelationID:   "rr-schema-2390",
				ActingUser:      mcpinternal.UserInfo{Username: "alice"},
				DiscoveryResult: discoveryWithWorkflow(workflowID),
			},
		}
		selectTool := mcptools.NewSelectWorkflowTool(catalog, sessions)

		handler, _ := mcpinternal.BootstrapMCP(mcpinternal.MCPDeps{
			AuthMiddleware: func(next http.Handler) http.Handler { return next },
			Tools: mcpinternal.ToolDeps{
				SelectWorkflow: mcptools.SelectWorkflowRegistration(selectTool, logr.Discard()),
			},
		})
		server := httptest.NewServer(handler)
		DeferCleanup(server.Close)

		client := mcpsdk.NewClient(&mcpsdk.Implementation{
			Name:    "select-workflow-schema-test-client",
			Version: "test",
		}, nil)
		session, err := client.Connect(context.Background(), &mcpsdk.StreamableClientTransport{Endpoint: server.URL}, nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = session.Close() })

		var selectSchema map[string]any
		for tool, toolErr := range session.Tools(context.Background(), nil) {
			Expect(toolErr).NotTo(HaveOccurred())
			if tool.Name == "kubernaut_select_workflow" {
				var ok bool
				selectSchema, ok = tool.OutputSchema.(map[string]any)
				Expect(ok).To(BeTrue(), "select_workflow must advertise an object output schema")
			}
		}
		Expect(selectSchema).NotTo(BeNil())

		workflowSchema := selectSchema["properties"].(map[string]any)["workflow"].(map[string]any)
		resourcesSchema := workflowSchema["properties"].(map[string]any)["resources"].(map[string]any)
		limitsSchema := resourcesSchema["properties"].(map[string]any)["limits"].(map[string]any)
		quantitySchema := limitsSchema["additionalProperties"].(map[string]any)
		Expect(quantitySchema["type"]).To(Equal("string"),
			"Kubernetes resource.Quantity marshals as a JSON string")

		result, err := session.CallTool(context.Background(), &mcpsdk.CallToolParams{
			Name: "kubernaut_select_workflow",
			Arguments: map[string]any{
				"rr_id":       "rr-schema-2390",
				"workflow_id": workflowID,
				"acting_user": "alice",
			},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.IsError).To(BeFalse())
		Expect(result.StructuredContent).NotTo(BeNil())
	})
})
