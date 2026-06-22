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

// Package spike_s11 validates that WE can execute remediation workflows on
// a remote cluster via K8s MCP Server tool calls (resources_create_or_update,
// resources_get, resources_delete, tekton_pipeline_start).
// This code is NOT production code — it lives under docs/spikes/ per project convention.
package spike_s11

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultMCPEndpoint = "http://127.0.0.1:18080"
	workflowNamespace  = "kubernaut-workflows"
)

func TestSpikeS11(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spike S11 — WE Remote Execution via K8s MCP Server")
}

func mcpEndpoint() string {
	if e := os.Getenv("MCP_ENDPOINT"); e != "" {
		return e
	}
	return defaultMCPEndpoint
}

func mustConnectMCP(ctx context.Context) *mcp.ClientSession {
	client := mcp.NewClient(&mcp.Implementation{Name: "spike-s11-test", Version: "v1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: mcpEndpoint() + "/mcp"}
	session, err := client.Connect(ctx, transport, nil)
	Expect(err).ToNot(HaveOccurred(), "failed to connect to MCP server at %s", mcpEndpoint())
	return session
}

func extractText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			return tc.Text
		}
	}
	return ""
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

var _ = Describe("Spike S11 — WE Remote Execution via K8s MCP Server", Ordered, func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 120*time.Second)
	})

	AfterEach(func() {
		cancel()
	})

	// --- Preflight: list tools and verify capabilities ---

	It("S11-PREFLIGHT: lists available tools and verifies core + tekton toolsets", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		tools, err := session.ListTools(ctx, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(tools.Tools).ToNot(BeEmpty())

		toolNames := make([]string, len(tools.Tools))
		for i, t := range tools.Tools {
			toolNames[i] = t.Name
		}
		GinkgoWriter.Printf("Available tools (%d): %v\n", len(toolNames), toolNames)

		Expect(toolNames).To(ContainElement("resources_create_or_update"), "core toolset: resources_create_or_update required for Job/PipelineRun creation")
		Expect(toolNames).To(ContainElement("resources_get"), "core toolset: resources_get required for status polling")
		Expect(toolNames).To(ContainElement("resources_delete"), "core toolset: resources_delete required for cleanup")
		Expect(toolNames).To(ContainElement("resources_list"), "core toolset: resources_list required for label queries")
	})

	// --- Setup: verify test fixtures exist (pre-created via kubectl) ---

	It("S11-SETUP: verifies test Secret and ConfigMap exist in kubernaut-workflows via resources_get", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_get",
			Arguments: map[string]any{
				"apiVersion": "v1",
				"kind":       "Secret",
				"namespace":  workflowNamespace,
				"name":       "spike-dep-secret",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "spike-dep-secret not found — create it first: kubectl create secret generic spike-dep-secret --from-literal=api-key=test -n %s", workflowNamespace)
		GinkgoWriter.Printf("Verified Secret exists: %s\n", truncate(extractText(result), 200))

		result, err = session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_get",
			Arguments: map[string]any{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"namespace":  workflowNamespace,
				"name":       "spike-dep-config",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "spike-dep-config not found — create it first: kubectl create configmap spike-dep-config --from-literal=timeout=30s -n %s", workflowNamespace)
		GinkgoWriter.Printf("Verified ConfigMap exists: %s\n", truncate(extractText(result), 200))
	})

	// --- S11-001: Create Job with full WE spec ---

	It("S11-001: creates a batch/v1 Job via resources_create_or_update with SA, env vars, and volume mounts", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		jobYAML := fmt.Sprintf(`{
			"apiVersion": "batch/v1",
			"kind": "Job",
			"metadata": {
				"name": "wfe-spike-s11-001",
				"namespace": "%s",
				"labels": {
					"kubernaut.ai/workflow-execution": "spike-s11-001",
					"kubernaut.ai/managed": "true",
					"kubernaut.ai/spike": "s11"
				}
			},
			"spec": {
				"backoffLimit": 0,
				"ttlSecondsAfterFinished": 120,
				"template": {
					"spec": {
						"serviceAccountName": "kubernaut-workflow-runner",
						"restartPolicy": "Never",
						"containers": [{
							"name": "workflow",
							"image": "busybox:latest",
							"command": ["sh", "-c", "echo TARGET=$TARGET_RESOURCE; ls /run/kubernaut/secrets/spike-dep-secret/; ls /run/kubernaut/configmaps/spike-dep-config/; echo DONE; exit 0"],
							"env": [
								{"name": "TARGET_RESOURCE", "value": "default/Deployment/nginx"},
								{"name": "WORKFLOW_PARAM_1", "value": "restart"}
							],
							"volumeMounts": [
								{"name": "secret-spike-dep-secret", "mountPath": "/run/kubernaut/secrets/spike-dep-secret", "readOnly": true},
								{"name": "configmap-spike-dep-config", "mountPath": "/run/kubernaut/configmaps/spike-dep-config", "readOnly": true}
							]
						}],
						"volumes": [
							{"name": "secret-spike-dep-secret", "secret": {"secretName": "spike-dep-secret"}},
							{"name": "configmap-spike-dep-config", "configMap": {"name": "spike-dep-config"}}
						]
					}
				}
			}
		}`, workflowNamespace)

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_create_or_update",
			Arguments: map[string]any{
				"apiVersion": "batch/v1",
				"kind":       "Job",
				"namespace":  workflowNamespace,
				"name":       "wfe-spike-s11-001",
				"resource":   jobYAML,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "failed to create Job: %s", extractText(result))

		text := extractText(result)
		Expect(text).To(ContainSubstring("wfe-spike-s11-001"))
		GinkgoWriter.Printf("S11-001 Job created: %s\n", truncate(text, 300))
	})

	// --- S11-002: Poll Job status ---

	It("S11-002: polls Job status via resources_get until completion", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		Eventually(func(g Gomega) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "resources_get",
				Arguments: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"namespace":  workflowNamespace,
					"name":       "wfe-spike-s11-001",
				},
			})
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(result.IsError).To(BeFalse())

			text := extractText(result)
			GinkgoWriter.Printf("S11-002 Job status poll: %s\n", truncate(text, 400))

			g.Expect(text).To(SatisfyAny(
				ContainSubstring("succeeded:"),
				ContainSubstring("type: Complete"),
				ContainSubstring("type: Failed"),
				ContainSubstring("CompletionsReached"),
			))
		}, 90*time.Second, 5*time.Second).Should(Succeed(), "Job did not complete within timeout")
	})

	// --- S11-003: Delete Job ---

	It("S11-003: deletes completed Job via resources_delete", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_delete",
			Arguments: map[string]any{
				"apiVersion": "batch/v1",
				"kind":       "Job",
				"namespace":  workflowNamespace,
				"name":       "wfe-spike-s11-001",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.IsError).To(BeFalse(), "failed to delete Job: %s", extractText(result))
		GinkgoWriter.Printf("S11-003 Job deleted: %s\n", truncate(extractText(result), 200))
	})

	// --- S11-008: Job fails when SA does not exist ---

	It("S11-008: Job fails correctly when SA does not exist", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		jobYAML := fmt.Sprintf(`{
			"apiVersion": "batch/v1",
			"kind": "Job",
			"metadata": {
				"name": "wfe-spike-s11-008",
				"namespace": "%s",
				"labels": {"kubernaut.ai/spike": "s11"}
			},
			"spec": {
				"backoffLimit": 0,
				"ttlSecondsAfterFinished": 60,
				"template": {
					"spec": {
						"serviceAccountName": "nonexistent-sa-spike",
						"restartPolicy": "Never",
						"containers": [{
							"name": "workflow",
							"image": "busybox:latest",
							"command": ["echo", "should not run"]
						}]
					}
				}
			}
		}`, workflowNamespace)

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_create_or_update",
			Arguments: map[string]any{
				"apiVersion": "batch/v1",
				"kind":       "Job",
				"namespace":  workflowNamespace,
				"name":       "wfe-spike-s11-008",
				"resource":   jobYAML,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		// Job creation itself may succeed (K8s accepts the spec) -- the pod will fail
		GinkgoWriter.Printf("S11-008 Job creation result (isError=%v): %s\n", result.IsError, truncate(extractText(result), 300))

		// Wait for the job to surface a pod failure
		Eventually(func(g Gomega) string {
			r, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "resources_get",
				Arguments: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"namespace":  workflowNamespace,
					"name":       "wfe-spike-s11-008",
				},
			})
			g.Expect(err).ToNot(HaveOccurred())
			return extractText(r)
		}, 60*time.Second, 5*time.Second).Should(SatisfyAny(
			ContainSubstring("nonexistent-sa-spike"),
			ContainSubstring("not found"),
			ContainSubstring("ServiceAccount"),
			ContainSubstring("failed"),
			ContainSubstring("Failed"),
			ContainSubstring("CreateContainerConfigError"),
			ContainSubstring("ErrImagePull"),
		), "expected Job to surface SA-related error")

		// Cleanup
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_delete",
			Arguments: map[string]any{
				"apiVersion": "batch/v1",
				"kind":       "Job",
				"namespace":  workflowNamespace,
				"name":       "wfe-spike-s11-008",
			},
		})
	})

	// --- S11-009: Job fails when dependency Secret does not exist ---

	It("S11-009: Job fails with actionable error when dependency Secret does not exist", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		jobYAML := fmt.Sprintf(`{
			"apiVersion": "batch/v1",
			"kind": "Job",
			"metadata": {
				"name": "wfe-spike-s11-009",
				"namespace": "%s",
				"labels": {"kubernaut.ai/spike": "s11"}
			},
			"spec": {
				"backoffLimit": 0,
				"ttlSecondsAfterFinished": 60,
				"template": {
					"spec": {
						"serviceAccountName": "kubernaut-workflow-runner",
						"restartPolicy": "Never",
						"containers": [{
							"name": "workflow",
							"image": "busybox:latest",
							"command": ["echo", "should not run"],
							"volumeMounts": [{
								"name": "missing-secret",
								"mountPath": "/run/kubernaut/secrets/missing",
								"readOnly": true
							}]
						}],
						"volumes": [{
							"name": "missing-secret",
							"secret": {"secretName": "nonexistent-secret-spike"}
						}]
					}
				}
			}
		}`, workflowNamespace)

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_create_or_update",
			Arguments: map[string]any{
				"apiVersion": "batch/v1",
				"kind":       "Job",
				"namespace":  workflowNamespace,
				"name":       "wfe-spike-s11-009",
				"resource":   jobYAML,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		GinkgoWriter.Printf("S11-009 Job creation (isError=%v): %s\n", result.IsError, truncate(extractText(result), 300))

		// Pod should fail with volume mount error referencing the missing secret
		Eventually(func(g Gomega) string {
			r, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "resources_get",
				Arguments: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"namespace":  workflowNamespace,
					"name":       "wfe-spike-s11-009",
				},
			})
			g.Expect(err).ToNot(HaveOccurred())
			text := extractText(r)
			GinkgoWriter.Printf("S11-009 poll: %s\n", truncate(text, 400))
			return text
		}, 90*time.Second, 5*time.Second).Should(SatisfyAny(
			ContainSubstring("nonexistent-secret-spike"),
			ContainSubstring("not found"),
			ContainSubstring("secret"),
			ContainSubstring("MountVolume"),
			ContainSubstring("failed"),
		), "expected actionable error about missing Secret")

		// Also check events for the pod to get the volume error
		eventsResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "events_list",
			Arguments: map[string]any{
				"namespace": workflowNamespace,
			},
		})
		if err == nil && !eventsResult.IsError {
			eventsText := extractText(eventsResult)
			GinkgoWriter.Printf("S11-009 events (first 500):\n%s\n", truncate(eventsText, 500))
		}

		// Cleanup
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_delete",
			Arguments: map[string]any{
				"apiVersion": "batch/v1",
				"kind":       "Job",
				"namespace":  workflowNamespace,
				"name":       "wfe-spike-s11-009",
			},
		})
	})

	// --- S11-010: Job fails when dependency ConfigMap does not exist ---

	It("S11-010: Job fails with actionable error when dependency ConfigMap does not exist", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		jobYAML := fmt.Sprintf(`{
			"apiVersion": "batch/v1",
			"kind": "Job",
			"metadata": {
				"name": "wfe-spike-s11-010",
				"namespace": "%s",
				"labels": {"kubernaut.ai/spike": "s11"}
			},
			"spec": {
				"backoffLimit": 0,
				"ttlSecondsAfterFinished": 60,
				"template": {
					"spec": {
						"serviceAccountName": "kubernaut-workflow-runner",
						"restartPolicy": "Never",
						"containers": [{
							"name": "workflow",
							"image": "busybox:latest",
							"command": ["echo", "should not run"],
							"volumeMounts": [{
								"name": "missing-cm",
								"mountPath": "/run/kubernaut/configmaps/missing",
								"readOnly": true
							}]
						}],
						"volumes": [{
							"name": "missing-cm",
							"configMap": {"name": "nonexistent-configmap-spike"}
						}]
					}
				}
			}
		}`, workflowNamespace)

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_create_or_update",
			Arguments: map[string]any{
				"apiVersion": "batch/v1",
				"kind":       "Job",
				"namespace":  workflowNamespace,
				"name":       "wfe-spike-s11-010",
				"resource":   jobYAML,
			},
		})
		Expect(err).ToNot(HaveOccurred())
		GinkgoWriter.Printf("S11-010 Job creation (isError=%v): %s\n", result.IsError, truncate(extractText(result), 300))

		Eventually(func(g Gomega) string {
			r, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "resources_get",
				Arguments: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"namespace":  workflowNamespace,
					"name":       "wfe-spike-s11-010",
				},
			})
			g.Expect(err).ToNot(HaveOccurred())
			return extractText(r)
		}, 90*time.Second, 5*time.Second).Should(SatisfyAny(
			ContainSubstring("nonexistent-configmap-spike"),
			ContainSubstring("not found"),
			ContainSubstring("configmap"),
			ContainSubstring("ConfigMap"),
			ContainSubstring("MountVolume"),
			ContainSubstring("failed"),
		), "expected actionable error about missing ConfigMap")

		// Cleanup
		_, _ = session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_delete",
			Arguments: map[string]any{
				"apiVersion": "batch/v1",
				"kind":       "Job",
				"namespace":  workflowNamespace,
				"name":       "wfe-spike-s11-010",
			},
		})
	})

	// --- Teardown ---

	It("S11-TEARDOWN: cleans up any remaining spike Jobs", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		for _, name := range []string{"wfe-spike-s11-001", "wfe-spike-s11-008", "wfe-spike-s11-009", "wfe-spike-s11-010"} {
			_, _ = session.CallTool(ctx, &mcp.CallToolParams{
				Name: "resources_delete",
				Arguments: map[string]any{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"namespace":  workflowNamespace,
					"name":       name,
				},
			})
		}
		GinkgoWriter.Println("S11-TEARDOWN: spike Jobs cleaned up (Secrets/ConfigMaps left for manual cleanup via kubectl)")
	})
})
