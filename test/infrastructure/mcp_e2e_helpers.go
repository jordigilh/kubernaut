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

package infrastructure

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPClientConfig holds the configuration for creating an MCP client connection.
// Uses TLS-aware transport + SA Bearer token auth, matching the E2E suite pattern.
type MCPClientConfig struct {
	Endpoint       string
	SAToken        string
	TLSTransport   http.RoundTripper
	Implementation *mcpsdk.Implementation
}

// DefaultMCPImplementation returns the standard MCP client implementation metadata
// for E2E tests.
func DefaultMCPImplementation() *mcpsdk.Implementation {
	return &mcpsdk.Implementation{
		Name:    "kubernaut-e2e-client",
		Version: "1.5.0",
	}
}

// ConnectMCPClient creates an MCP SDK client session connected to a real KA endpoint
// in Kind via Streamable HTTP transport with TLS + SA token authentication.
//
// The transport chain is: TLS (inter-service CA) -> Bearer token injection -> HTTP.
// This mirrors the auth pattern in test/e2e/kubernautagent/suite_test.go.
func ConnectMCPClient(ctx context.Context, cfg MCPClientConfig) (*mcpsdk.ClientSession, error) {
	if cfg.Implementation == nil {
		cfg.Implementation = DefaultMCPImplementation()
	}

	client := mcpsdk.NewClient(cfg.Implementation, nil)

	saTransport := testauth.NewServiceAccountTransportWithBase(cfg.SAToken, cfg.TLSTransport)

	transport := &mcpsdk.StreamableClientTransport{
		Endpoint:   cfg.Endpoint,
		HTTPClient: &http.Client{Transport: saTransport},
	}

	return client.Connect(ctx, transport, nil)
}

// CallInvestigate calls the kubernaut_investigate MCP tool with the given arguments.
func CallInvestigate(ctx context.Context, session *mcpsdk.ClientSession, args map[string]any) (*mcpsdk.CallToolResult, error) {
	return session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "kubernaut_investigate",
		Arguments: args,
	})
}

// CallSelectWorkflow calls the kubernaut_select_workflow MCP tool with the given arguments.
func CallSelectWorkflow(ctx context.Context, session *mcpsdk.ClientSession, args map[string]any) (*mcpsdk.CallToolResult, error) {
	return session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "kubernaut_select_workflow",
		Arguments: args,
	})
}

// CallCompleteNoAction calls the kubernaut_complete_no_action MCP tool with the given arguments.
func CallCompleteNoAction(ctx context.Context, session *mcpsdk.ClientSession, args map[string]any) (*mcpsdk.CallToolResult, error) {
	return session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "kubernaut_complete_no_action",
		Arguments: args,
	})
}

// ExtractToolResultText extracts the text content from an MCP CallToolResult.
// Returns the concatenated text from all TextContent blocks.
func ExtractToolResultText(result *mcpsdk.CallToolResult) string {
	if result == nil {
		return ""
	}
	var text string
	for _, c := range result.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			text += tc.Text
		}
	}
	return text
}

// MCPEndpointForKAE2E returns the MCP endpoint URL for the KA E2E Kind cluster.
// KA is exposed on https://localhost:8088 via NodePort, MCP is under /api/v1/mcp.
func MCPEndpointForKAE2E() string {
	return "https://localhost:8088/api/v1/mcp"
}

// CreateInteractiveE2ESA creates a ServiceAccount with full interactive RBAC
// (impersonate, leases, pods access) and returns its token.
// Self-contained: ensures the prerequisite kubernaut-agent-e2e-client-access
// Role exists (idempotent) so this works in both KA E2E and FP E2E suites.
func CreateInteractiveE2ESA(ctx context.Context, namespace, saName, kubeconfigPath string, writer io.Writer) (string, error) {
	if err := CreateServiceAccount(ctx, namespace, kubeconfigPath, saName, writer); err != nil {
		return "", fmt.Errorf("create SA %s: %w", saName, err)
	}

	if err := ensureKAClientAccessRole(ctx, namespace, kubeconfigPath, writer); err != nil {
		return "", fmt.Errorf("ensure KA client access Role: %w", err)
	}

	if err := CreateKAE2EClientRBACForSA(ctx, namespace, kubeconfigPath, saName, writer); err != nil {
		return "", fmt.Errorf("bind KA client RBAC for %s: %w", saName, err)
	}

	token, err := GetServiceAccountToken(ctx, namespace, saName, kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("get token for %s: %w", saName, err)
	}
	return token, nil
}

// ensureKAClientAccessRole creates the kubernaut-agent-e2e-client-access Role
// if it doesn't already exist. Idempotent via kubectl apply.
func ensureKAClientAccessRole(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	roleYAML := fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: kubernaut-agent-e2e-client-access
  namespace: %s
  labels:
    app: kubernaut-agent
    component: e2e-testing
    authorization: dd-auth-014
rules:
  - apiGroups: [""]
    resources: ["services"]
    resourceNames: ["kubernaut-agent"]
    verbs: ["create", "get"]
`, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(roleYAML)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// CreateLimitedRBACSA creates a ServiceAccount with restricted RBAC for security tests.
// The SA can only read pods in the specified namespace, nothing else.
// It also has KA client access (to reach the MCP endpoint) but no impersonate/lease rights.
func CreateLimitedRBACSA(ctx context.Context, namespace, saName, kubeconfigPath, allowedPodNamespace string, writer io.Writer) (string, error) {
	if err := CreateServiceAccount(ctx, namespace, kubeconfigPath, saName, writer); err != nil {
		return "", fmt.Errorf("create SA %s: %w", saName, err)
	}

	if err := CreateKAE2EClientRBACForSA(ctx, namespace, kubeconfigPath, saName, writer); err != nil {
		return "", fmt.Errorf("bind KA client RBAC for %s: %w", saName, err)
	}

	manifest := fmt.Sprintf(`apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: %s-limited-pods
  namespace: %s
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: %s-limited-pods-binding
  namespace: %s
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: %s-limited-pods
subjects:
- kind: ServiceAccount
  name: %s
  namespace: %s
`, saName, allowedPodNamespace, saName, allowedPodNamespace, saName, saName, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("apply limited RBAC for %s: %w\n%s", saName, err, string(out))
	}

	token, err := GetServiceAccountToken(ctx, namespace, saName, kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("get token for %s: %w", saName, err)
	}
	return token, nil
}
