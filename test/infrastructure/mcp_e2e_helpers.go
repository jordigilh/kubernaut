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
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

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

// ConnectMCPClientWithRetry wraps ConnectMCPClient with exponential backoff on
// 429 (Too Many Requests). Matches KA's Retry-After: 1 header semantics.
func ConnectMCPClientWithRetry(ctx context.Context, cfg MCPClientConfig, writer io.Writer) (*mcpsdk.ClientSession, error) {
	backoff := 2 * time.Second
	const maxRetries = 5
	for attempt := 0; attempt <= maxRetries; attempt++ {
		connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		session, err := ConnectMCPClient(connectCtx, cfg)
		cancel()
		if err == nil {
			return session, nil
		}
		if !strings.Contains(err.Error(), "Too Many Requests") || attempt == maxRetries {
			return nil, err
		}
		if writer != nil {
			_, _ = fmt.Fprintf(writer, "  ⏳ MCP connect got 429, retrying in %v (attempt %d/%d)\n", backoff, attempt+1, maxRetries)
		}
		select {
		case <-time.After(backoff):
			backoff *= 2
		case <-ctx.Done():
			return nil, fmt.Errorf("context cancelled during 429 backoff: %w", ctx.Err())
		}
	}
	return nil, fmt.Errorf("unreachable")
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

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "--server-side", "--force-conflicts", "--kubeconfig", kubeconfigPath, "-f", "-")
	cmd.Stdin = strings.NewReader(roleYAML)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// CreateDirectRR creates a RemediationRequest CRD directly (bypassing the OOMKill
// pipeline) for tests that don't need the full event->Gateway->SP->RO flow.
// Returns the RR name. A managed namespace is created for the target resource so
// that the RO routing engine does not block the RR as UnmanagedResource.
func CreateDirectRR(ctx context.Context, namespace, testID string) (string, error) {
	rrName := fmt.Sprintf("rr-%s-%d", testID, time.Now().UnixMilli())
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte(testID+"-"+rrName)))
	now := metav1.Now()

	cfg, err := config.GetConfig()
	if err != nil {
		return "", fmt.Errorf("get kubeconfig: %w", err)
	}
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return "", fmt.Errorf("create dynamic client: %w", err)
	}

	targetNS := fmt.Sprintf("fp-%s-%d", testID, time.Now().UnixMilli())
	nsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	nsObj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": targetNS,
				"labels": map[string]interface{}{
					"kubernaut.ai/managed": "true",
				},
			},
		},
	}
	nsCtx, nsCancel := context.WithTimeout(ctx, 10*time.Second)
	defer nsCancel()
	if _, err := dynClient.Resource(nsGVR).Create(nsCtx, nsObj, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("create managed namespace %s: %w", targetNS, err)
	}

	rr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "RemediationRequest",
			"metadata": map[string]interface{}{
				"name":      rrName,
				"namespace": namespace,
			},
			"spec": map[string]interface{}{
				"signalFingerprint": fingerprint,
				"signalName":        fmt.Sprintf("e2e-%s-signal", testID),
				"signalType":        "alert",
				"severity":          "high",
				"targetType":        "kubernetes",
				"firingTime":        now.UTC().Format(time.RFC3339),
				"receivedTime":      now.UTC().Format(time.RFC3339),
				"targetResource": map[string]interface{}{
					"kind":      "Deployment",
					"name":      fmt.Sprintf("%s-target", testID),
					"namespace": targetNS,
				},
			},
		},
	}

	rrGVR := schema.GroupVersionResource{
		Group:    "kubernaut.ai",
		Version:  "v1alpha1",
		Resource: "remediationrequests",
	}

	createCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	_, err = dynClient.Resource(rrGVR).Namespace(namespace).Create(createCtx, rr, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("create RR CRD: %w", err)
	}
	return rrName, nil
}

// MCPSessionSetup holds the result of SetupMCPSession.
type MCPSessionSetup struct {
	Session  *mcpsdk.ClientSession
	SAToken  string
	Cleanup  func()
}

// SetupMCPSession creates a ServiceAccount with interactive RBAC, sets up TLS,
// and connects an MCP client session. Returns the session and a cleanup function.
// Retries on 429 (Too Many Requests) with exponential backoff, matching KA's
// per-IP (5 rps) and per-user (10 rps) rate limiters.
func SetupMCPSession(ctx context.Context, namespace, saName, kubeconfigPath string, writer io.Writer) (*MCPSessionSetup, error) {
	token, err := CreateInteractiveE2ESA(ctx, namespace, saName, kubeconfigPath, writer)
	if err != nil {
		return nil, fmt.Errorf("create interactive SA: %w", err)
	}

	tlsTransport, err := NewTLSAwareTransport(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("TLS transport: %w", err)
	}

	cfg := MCPClientConfig{
		Endpoint:     MCPEndpointForKAE2E(),
		SAToken:      token,
		TLSTransport: tlsTransport,
	}

	var session *mcpsdk.ClientSession
	backoff := 2 * time.Second
	const maxRetries = 5
	for attempt := 0; attempt <= maxRetries; attempt++ {
		connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		session, err = ConnectMCPClient(connectCtx, cfg)
		cancel()
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "Too Many Requests") || attempt == maxRetries {
			return nil, fmt.Errorf("MCP connect: %w", err)
		}
		_, _ = fmt.Fprintf(writer, "  ⏳ MCP connect got 429, retrying in %v (attempt %d/%d)\n", backoff, attempt+1, maxRetries)
		select {
		case <-time.After(backoff):
			backoff *= 2
		case <-ctx.Done():
			return nil, fmt.Errorf("MCP connect: context cancelled during 429 backoff: %w", ctx.Err())
		}
	}

	return &MCPSessionSetup{
		Session: session,
		SAToken: token,
		Cleanup: func() { _ = session.Close() },
	}, nil
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
