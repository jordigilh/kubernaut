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

package mcp_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/integration"
)

// Port allocation per DD-TEST-001 v2.5 — MCP IT (Phase 7A)
const (
	mcpPostgresPort    = 13330
	mcpRedisPort       = 13331
	mcpDataStoragePort = 13332
	mcpMetricsPort     = 13333
)

var (
	sharedTestEnv   *envtest.Environment
	sharedK8sConfig *rest.Config
	sharedK8sClient client.Client

	// Mock LLM container (Podman, mode=interactive)
	sharedMockLLMConfig   infrastructure.MockLLMConfig
	sharedMockLLMEndpoint string

	// DataStorage infrastructure (Podman: PostgreSQL + Redis + DS)
	sharedDSInfra    *infrastructure.DSBootstrapInfra
	sharedDSEndpoint string
	sharedDSClient   *ogenclient.Client
)

func TestMCPIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Kubernaut Agent MCP Integration Suite")
}

var _ = SynchronizedBeforeSuite(
	func() []byte {
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("MCP IT - Phase 0: Infrastructure Bootstrap")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		ctx := context.Background()

		// ── Step 1: envtest (Leases are core K8s — no CRDs needed) ──
		By("Starting envtest")
		assetsDir := os.Getenv("KUBEBUILDER_ASSETS")
		if assetsDir == "" {
			out, err := exec.Command("setup-envtest", "use", "-p", "path").CombinedOutput()
			if err == nil {
				assetsDir = strings.TrimSpace(string(out))
			}
		}
		testEnv := &envtest.Environment{
			BinaryAssetsDirectory: assetsDir,
		}
		cfg, err := testEnv.Start()
		Expect(err).ToNot(HaveOccurred(), "envtest should start")
		GinkgoWriter.Printf("envtest API server: %s\n", cfg.Host)
		sharedTestEnv = testEnv

		scheme := runtime.NewScheme()
		Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())

		k8sClient, err := client.New(cfg, client.Options{Scheme: scheme})
		Expect(err).ToNot(HaveOccurred(), "controller-runtime client should build")
		sharedK8sConfig = cfg
		sharedK8sClient = k8sClient

		// ── Step 2: ServiceAccount + kubeconfig for DataStorage auth ──
		By("Creating ServiceAccount for DataStorage authentication")
		kubeconfigPath, err := infrastructure.WriteEnvtestKubeconfigToFile(cfg, "ka-mcp")
		Expect(err).ToNot(HaveOccurred())

		authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
			cfg, "ka-mcp-sa", "default", GinkgoWriter,
		)
		Expect(err).ToNot(HaveOccurred())

		// ── Step 3: DataStorage (PostgreSQL + Redis + DS via Podman) ──
		By("Starting DataStorage infrastructure (PostgreSQL, Redis, DataStorage)")
		dsCfg := infrastructure.NewDSBootstrapConfigWithAuth(
			"kamcp",
			mcpPostgresPort, mcpRedisPort, mcpDataStoragePort, mcpMetricsPort,
			"test/integration/kubernautagent/mcp/config",
			authConfig,
		)
		dsInfra, err := infrastructure.StartDSBootstrap(dsCfg, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "DS infrastructure must start")
		sharedDSInfra = dsInfra
		sharedDSEndpoint = fmt.Sprintf("http://127.0.0.1:%d", mcpDataStoragePort)
		GinkgoWriter.Printf("DataStorage endpoint: %s\n", sharedDSEndpoint)

		// ── Step 4: Mock LLM (Podman, mode=interactive) ──
		By("Building Mock LLM image")
		mockLLMCfg := infrastructure.GetMockLLMConfigForKA()
		builtImageTag, err := infrastructure.BuildMockLLMImage(ctx, mockLLMCfg.ServiceName, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Mock LLM image should build")
		mockLLMCfg.ImageTag = builtImageTag

		By("Starting Mock LLM container (mode=interactive)")
		_, err = infrastructure.StartMockLLMContainer(ctx, mockLLMCfg, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Mock LLM container should start")
		sharedMockLLMConfig = mockLLMCfg
		sharedMockLLMEndpoint = infrastructure.GetMockLLMEndpoint(mockLLMCfg)
		GinkgoWriter.Printf("Mock LLM endpoint: %s (mode=interactive)\n", sharedMockLLMEndpoint)

		GinkgoWriter.Println("Phase 0 complete — envtest + DataStorage + Mock LLM ready")

		// Pass token + kubeconfig to Phase 2 processes
		payload := authConfig.Token + "\n" + kubeconfigPath
		return []byte(payload)
	},
	func(data []byte) {
		if sharedK8sClient == nil {
			scheme := runtime.NewScheme()
			Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			var err error
			sharedK8sClient, err = client.New(sharedK8sConfig, client.Options{Scheme: scheme})
			Expect(err).ToNot(HaveOccurred())
		}

		// Build authenticated DS client for this Ginkgo process
		lines := strings.SplitN(string(data), "\n", 2)
		if len(lines) == 2 {
			dsToken := lines[0]
			dsURL := fmt.Sprintf("http://127.0.0.1:%d", mcpDataStoragePort)
			dsClients := integration.NewAuthenticatedDataStorageClients(dsURL, dsToken, 10*time.Second)
			sharedDSClient = dsClients.OpenAPIClient
		}
	},
)

var _ = SynchronizedAfterSuite(
	func() {},
	func() {
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		GinkgoWriter.Println("MCP IT - Infrastructure Cleanup")
		GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		ctx := context.Background()

		if sharedMockLLMConfig.ContainerName != "" {
			_ = infrastructure.StopMockLLMContainer(ctx, sharedMockLLMConfig, GinkgoWriter)
		}
		if sharedDSInfra != nil {
			infrastructure.MustGatherContainerLogs("kamcp", []string{
				sharedDSInfra.DataStorageContainer,
				sharedDSInfra.PostgresContainer,
				sharedDSInfra.RedisContainer,
			}, GinkgoWriter)
			_ = infrastructure.StopDSBootstrap(sharedDSInfra, GinkgoWriter)
		}
		if sharedTestEnv != nil {
			Expect(sharedTestEnv.Stop()).To(Succeed())
		}
		GinkgoWriter.Println("Suite complete")
	},
)
