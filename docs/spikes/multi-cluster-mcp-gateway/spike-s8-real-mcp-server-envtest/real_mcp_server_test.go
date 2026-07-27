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

// Package spike_s8 validates that the real kubernetes-mcp-server binary can run
// against envtest and serve MCP tool calls for fleet IT testing.
// This code is NOT production code — it lives under docs/spikes/ per project convention.
package spike_s8

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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"k8s.io/apimachinery/pkg/runtime"
)

func TestSpikeS8(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Spike S8 — Real K8s MCP Server against envtest")
}

var (
	testEnv    *envtest.Environment
	k8sClient  client.Client
	restCfg    *rest.Config
	mcpCmd     *exec.Cmd
	mcpPort    string
	kubeconfig string
)

var _ = BeforeSuite(func() {
	By("Starting envtest")
	testEnv = &envtest.Environment{}
	var err error
	restCfg, err = testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(restCfg).ToNot(BeNil())

	scheme := runtime.NewScheme()
	Expect(corev1.AddToScheme(scheme)).To(Succeed())
	Expect(appsv1.AddToScheme(scheme)).To(Succeed())

	k8sClient, err = client.New(restCfg, client.Options{Scheme: scheme})
	Expect(err).ToNot(HaveOccurred())

	By("Writing kubeconfig for envtest")
	kubeconfig = writeKubeconfig(restCfg)

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

	By("Creating test namespace with managed label")
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "spike-test",
			Labels: map[string]string{
				"kubernaut.ai/managed": "true",
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), ns)).To(Succeed())

	By("Creating managed Deployment")
	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx",
			Namespace: "spike-test",
			Labels: map[string]string{
				"kubernaut.ai/managed": "true",
				"app":                  "nginx",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "nginx"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "nginx",
						Image: "nginx:latest",
					}},
				},
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), dep)).To(Succeed())

	By("Creating unmanaged Deployment")
	dep2 := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: "spike-test",
			Labels: map[string]string{
				"app": "redis",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "redis"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "redis"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "redis",
						Image: "redis:latest",
					}},
				},
			},
		},
	}
	Expect(k8sClient.Create(context.Background(), dep2)).To(Succeed())
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

var _ = Describe("Spike S8 — Real K8s MCP Server against envtest", func() {
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

	It("S8-001: connects to real K8s MCP Server and lists tools", func() {
		endpoint := fmt.Sprintf("http://127.0.0.1:%s", mcpPort)
		session := mustConnectMCP(ctx, endpoint)
		defer session.Close()

		tools, err := session.ListTools(ctx, nil)
		Expect(err).ToNot(HaveOccurred())
		Expect(tools.Tools).ToNot(BeEmpty())

		toolNames := make([]string, len(tools.Tools))
		for i, t := range tools.Tools {
			toolNames[i] = t.Name
		}
		Expect(toolNames).To(ContainElement("resources_list"))
		Expect(toolNames).To(ContainElement("resources_get"))
		GinkgoWriter.Printf("Available tools: %v\n", toolNames)
	})

	It("S8-002: resources_list with labelSelector returns only managed resources", func() {
		endpoint := fmt.Sprintf("http://127.0.0.1:%s", mcpPort)
		session := mustConnectMCP(ctx, endpoint)
		defer session.Close()

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_list",
			Arguments: map[string]any{
				"apiVersion":    "apps/v1",
				"kind":          "Deployment",
				"namespace":     "spike-test",
				"labelSelector": "kubernaut.ai/managed=true",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Content).ToNot(BeEmpty())

		text := extractText(result)
		Expect(text).ToNot(BeEmpty())
		GinkgoWriter.Printf("resources_list response:\n%s\n", text)

		Expect(text).To(ContainSubstring("nginx"))
		Expect(text).ToNot(ContainSubstring("redis"))
	})

	It("S8-003: resources_get retrieves specific resource with labels", func() {
		endpoint := fmt.Sprintf("http://127.0.0.1:%s", mcpPort)
		session := mustConnectMCP(ctx, endpoint)
		defer session.Close()

		result, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_get",
			Arguments: map[string]any{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"namespace":  "spike-test",
				"name":       "nginx",
			},
		})
		Expect(err).ToNot(HaveOccurred())
		Expect(result.Content).ToNot(BeEmpty())

		text := extractText(result)
		Expect(text).ToNot(BeEmpty())
		Expect(text).To(ContainSubstring("kubernaut.ai/managed"))
		Expect(text).To(ContainSubstring("nginx"))
		GinkgoWriter.Printf("resources_get response (first 500 chars):\n%s\n", truncate(text, 500))
	})

	It("S8-004: scope check pattern — verify managed label via MCP", func() {
		endpoint := fmt.Sprintf("http://127.0.0.1:%s", mcpPort)
		session := mustConnectMCP(ctx, endpoint)
		defer session.Close()

		// Simulate the federated scope check pattern:
		// 1. Get the namespace and check its labels
		nsResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_get",
			Arguments: map[string]any{
				"apiVersion": "v1",
				"kind":       "Namespace",
				"name":       "spike-test",
			},
		})
		Expect(err).ToNot(HaveOccurred())

		nsText := extractText(nsResult)
		Expect(nsText).To(ContainSubstring("kubernaut.ai/managed"))

		// 2. Get the resource and check its labels
		resResult, err := session.CallTool(ctx, &mcp.CallToolParams{
			Name: "resources_get",
			Arguments: map[string]any{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"namespace":  "spike-test",
				"name":       "nginx",
			},
		})
		Expect(err).ToNot(HaveOccurred())

		resText := extractText(resResult)
		Expect(resText).To(ContainSubstring("kubernaut.ai/managed"))

		GinkgoWriter.Println("Scope check via MCP: PASS (both namespace and resource have managed label)")
	})
})

// --- Helpers ---

func writeKubeconfig(cfg *rest.Config) string {
	dir := os.TempDir()
	path := filepath.Join(dir, fmt.Sprintf("spike-s8-kubeconfig-%d", time.Now().UnixNano()))

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

func mustConnectMCP(ctx context.Context, endpoint string) *mcp.ClientSession {
	client := mcp.NewClient(&mcp.Implementation{Name: "spike-s8-test", Version: "v1.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: endpoint + "/mcp"}
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
