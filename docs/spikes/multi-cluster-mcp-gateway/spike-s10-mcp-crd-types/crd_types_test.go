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

// Package spike_s10 validates that the real kubernetes-mcp-server binary can
// list and get CRD-based resources (non-core types) via envtest.
// This removes the risk that MCP only handles built-in K8s types.
// This code is NOT production code — it lives under docs/spikes/ per project convention.
package spike_s10

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
)

func TestSpikeS10(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spike S10 — K8s MCP Server with CRD Types")
}

var (
	testEnv    *envtest.Environment
	restCfg    *rest.Config
	dynClient  dynamic.Interface
	mcpCmd     *exec.Cmd
	mcpPort    string
	kubeconfig string
)

var _ = BeforeSuite(func() {
	By("Starting envtest with RemediationRequest CRD")
	crdPath := filepath.Join("..", "..", "..", "..", "config", "crd", "bases")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{crdPath},
	}
	var err error
	restCfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(restCfg).ToNot(BeNil())

	dynClient, err = dynamic.NewForConfig(restCfg)
	Expect(err).ToNot(HaveOccurred())

	By("Writing kubeconfig for envtest")
	kubeconfig = writeKubeconfig(restCfg)

	By("Creating test namespace with managed label")
	nsGVR := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	ns := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Namespace",
			"metadata": map[string]interface{}{
				"name": "spike-crd-test",
				"labels": map[string]interface{}{
					"kubernaut.ai/managed": "true",
				},
			},
		},
	}
	_, err = dynClient.Resource(nsGVR).Create(context.Background(), ns, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())

	By("Creating a RemediationRequest CRD instance")
	rrGVR := schema.GroupVersionResource{Group: "kubernaut.ai", Version: "v1alpha1", Resource: "remediationrequests"}
	rr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "RemediationRequest",
			"metadata": map[string]interface{}{
				"name":      "test-rr-001",
				"namespace": "spike-crd-test",
				"labels": map[string]interface{}{
					"kubernaut.ai/managed": "true",
					"app":                  "spike-test",
				},
			},
			"spec": map[string]interface{}{
				"signalName":        "HighCPU",
				"signalType":        "prometheus",
				"signalFingerprint": "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2",
				"severity":          "critical",
				"firingTime":        "2026-06-19T20:00:00Z",
				"receivedTime":      "2026-06-19T20:00:01Z",
				"targetType":        "kubernetes",
				"clusterID":         "prod-east",
				"targetResource": map[string]interface{}{
					"kind":      "Deployment",
					"name":      "payment-api",
					"namespace": "production",
				},
			},
		},
	}
	_, err = dynClient.Resource(rrGVR).Namespace("spike-crd-test").Create(context.Background(), rr, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())

	By("Creating a second RR without managed label")
	rr2 := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kubernaut.ai/v1alpha1",
			"kind":       "RemediationRequest",
			"metadata": map[string]interface{}{
				"name":      "test-rr-002",
				"namespace": "spike-crd-test",
				"labels": map[string]interface{}{
					"app": "unmanaged",
				},
			},
			"spec": map[string]interface{}{
				"signalName":        "LowMemory",
				"signalType":        "prometheus",
				"signalFingerprint": "f1e2d3c4b5a6f7e8d9c0b1a2f3e4d5c6b7a8f9e0d1c2b3a4f5e6d7c8b9a0f1e2",
				"severity":          "warning",
				"firingTime":        "2026-06-19T20:00:00Z",
				"receivedTime":      "2026-06-19T20:00:01Z",
				"targetType":        "kubernetes",
				"targetResource": map[string]interface{}{
					"kind":      "StatefulSet",
					"name":      "redis",
					"namespace": "production",
				},
			},
		},
	}
	_, err = dynClient.Resource(rrGVR).Namespace("spike-crd-test").Create(context.Background(), rr2, metav1.CreateOptions{})
	Expect(err).ToNot(HaveOccurred())

	By("Finding free port for K8s MCP Server")
	mcpPort = findFreePort()

	By("Starting kubernetes-mcp-server")
	mcpBinary, err := exec.LookPath("kubernetes-mcp-server")
	Expect(err).ToNot(HaveOccurred(), "kubernetes-mcp-server binary must be in PATH")

	mcpCmd = exec.Command(mcpBinary,
		"--kubeconfig", kubeconfig,
		"--port", mcpPort,
		"--read-only",
		"--stateless",
		"--list-output", "yaml",
		"--disable-multi-cluster",
	)
	mcpCmd.Stdout = GinkgoWriter
	mcpCmd.Stderr = GinkgoWriter
	err = mcpCmd.Start()
	Expect(err).ToNot(HaveOccurred())

	By("Waiting for K8s MCP Server to be ready")
	Eventually(func() error {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+mcpPort, 500*time.Millisecond)
		if err != nil {
			return err
		}
		conn.Close()
		return nil
	}, 10*time.Second, 200*time.Millisecond).Should(Succeed())
})

var _ = AfterSuite(func() {
	By("Stopping K8s MCP Server")
	if mcpCmd != nil && mcpCmd.Process != nil {
		_ = mcpCmd.Process.Kill()
		_ = mcpCmd.Wait()
	}

	By("Stopping envtest")
	if testEnv != nil {
		_ = testEnv.Stop()
	}

	By("Cleaning up kubeconfig")
	if kubeconfig != "" {
		_ = os.Remove(kubeconfig)
	}
})

var _ = Describe("Spike S10 — K8s MCP Server with CRD Types", func() {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	BeforeEach(func() {
		ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	})

	AfterEach(func() {
		cancel()
	})

	It("S10-001: resources_list returns CRD instances (RemediationRequest)", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_list",
			Arguments: map[string]any{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationRequest",
				"namespace":  "spike-crd-test",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Content).ToNot(BeEmpty())

		text := extractText(result)
		Expect(text).ToNot(BeEmpty())
		GinkgoWriter.Printf("resources_list CRD response:\n%s\n", text)

		Expect(text).To(ContainSubstring("test-rr-001"))
		Expect(text).To(ContainSubstring("test-rr-002"))
	})

	It("S10-002: resources_list with labelSelector filters CRD instances", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_list",
			Arguments: map[string]any{
				"apiVersion":    "kubernaut.ai/v1alpha1",
				"kind":          "RemediationRequest",
				"namespace":     "spike-crd-test",
				"labelSelector": "kubernaut.ai/managed=true",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Content).ToNot(BeEmpty())

		text := extractText(result)
		Expect(text).ToNot(BeEmpty())
		GinkgoWriter.Printf("resources_list CRD with labelSelector:\n%s\n", text)

		Expect(text).To(ContainSubstring("test-rr-001"))
		Expect(text).ToNot(ContainSubstring("test-rr-002"))
	})

	It("S10-003: resources_get retrieves specific CRD instance with full spec", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_get",
			Arguments: map[string]any{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationRequest",
				"namespace":  "spike-crd-test",
				"name":       "test-rr-001",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Content).ToNot(BeEmpty())

		text := extractText(result)
		Expect(text).ToNot(BeEmpty())
		GinkgoWriter.Printf("resources_get CRD response (first 800 chars):\n%s\n", truncate(text, 800))

		Expect(text).To(ContainSubstring("kubernaut.ai/managed"))
		Expect(text).To(ContainSubstring("test-rr-001"))
		Expect(text).To(ContainSubstring("prod-east"))
		Expect(text).To(ContainSubstring("HighCPU"))
	})

	It("S10-004: resources_get returns error for non-existent CRD instance", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_get",
			Arguments: map[string]any{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationRequest",
				"namespace":  "spike-crd-test",
				"name":       "does-not-exist",
			},
		})
		Expect(err).ToNot(HaveOccurred())

		text := extractText(result)
		GinkgoWriter.Printf("resources_get non-existent CRD:\n%s\n", text)
		// K8s MCP Server should return an error message or isError=true
		Expect(result.IsError).To(BeTrue(), "expected isError=true for non-existent resource")
	})

	It("S10-005: MCPServerRegistration-like CRD can be listed (unstructured cluster-scoped)", func() {
		// This tests whether cluster-scoped CRDs work too.
		// We don't have the Kuadrant CRD installed, but we can test with
		// a cluster-scoped built-in like ClusterRole to validate the pattern.
		session := mustConnectMCP(ctx)
		defer session.Close()

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_list",
			Arguments: map[string]any{
				"apiVersion": "rbac.authorization.k8s.io/v1",
				"kind":       "ClusterRole",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Content).ToNot(BeEmpty())

		text := extractText(result)
		Expect(text).ToNot(BeEmpty())
		// envtest creates system ClusterRoles
		Expect(text).To(ContainSubstring("ClusterRole"))
		GinkgoWriter.Printf("Cluster-scoped resource list works: %d chars returned\n", len(text))
	})

	It("S10-006: scope check pattern on CRD — verify managed label via MCP", func() {
		session := mustConnectMCP(ctx)
		defer session.Close()

		// Simulate FMC Writer checking if an RR is managed:
		// 1. List CRDs with labelSelector
		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_list",
			Arguments: map[string]any{
				"apiVersion":    "kubernaut.ai/v1alpha1",
				"kind":          "RemediationRequest",
				"namespace":     "spike-crd-test",
				"labelSelector": "kubernaut.ai/managed=true",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		text := extractText(result)
		Expect(text).To(ContainSubstring("test-rr-001"))
		Expect(text).ToNot(ContainSubstring("test-rr-002"))

		// 2. Get specific CRD instance and check labels
		getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_get",
			Arguments: map[string]any{
				"apiVersion": "kubernaut.ai/v1alpha1",
				"kind":       "RemediationRequest",
				"namespace":  "spike-crd-test",
				"name":       "test-rr-001",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		getText := extractText(getResult)
		Expect(getText).To(ContainSubstring("kubernaut.ai/managed"))

		GinkgoWriter.Println("CRD scope check via MCP: PASS")
	})
})

// --- Helpers ---

func writeKubeconfig(cfg *rest.Config) string {
	dir := os.TempDir()
	path := filepath.Join(dir, fmt.Sprintf("spike-s10-kubeconfig-%d", time.Now().UnixNano()))

	kubeConfig := clientcmdapi.NewConfig()
	kubeConfig.Clusters["envtest"] = &clientcmdapi.Cluster{
		Server:                   cfg.Host,
		CertificateAuthorityData: cfg.CAData,
	}
	kubeConfig.AuthInfos["envtest"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: cfg.CertData,
		ClientKeyData:         cfg.KeyData,
	}
	kubeConfig.Contexts["envtest"] = &clientcmdapi.Context{
		Cluster:  "envtest",
		AuthInfo: "envtest",
	}
	kubeConfig.CurrentContext = "envtest"

	err := clientcmd.WriteToFile(*kubeConfig, path)
	Expect(err).ToNot(HaveOccurred())
	return path
}

func findFreePort() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).ToNot(HaveOccurred())
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return fmt.Sprintf("%d", port)
}

func mustConnectMCP(ctx context.Context) *mcp.ClientSession {
	endpoint := fmt.Sprintf("http://127.0.0.1:%s/mcp", mcpPort)
	client := mcp.NewClient(&mcp.Implementation{Name: "spike-s10-test", Version: "v1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: endpoint}
	session, err := client.Connect(ctx, transport, nil)
	Expect(err).ToNot(HaveOccurred())
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
