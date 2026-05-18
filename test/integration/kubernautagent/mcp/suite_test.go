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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"gopkg.in/yaml.v3"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/integration"
)

// phase1Payload is the JSON struct passed between Ginkgo processes via
// SynchronizedBeforeSuite. Uses the same pattern as the AIAnalysis IT suite.
type phase1Payload struct {
	Token            string            `json:"token"`
	KubeconfigPath   string            `json:"kubeconfig_path"`
	MockLLMEndpoint  string            `json:"mock_llm_endpoint"`
	WorkflowUUIDs    map[string]string `json:"workflow_uuids"`
}

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

	// Workflow UUIDs assigned by DataStorage during seeding (#1174).
	// Key format: "workflowID:environment" (e.g., "oomkill-increase-memory-v1:production").
	sharedWorkflowUUIDs map[string]string

	// Temp file for Mock LLM scenario overrides; cleaned up in AfterSuite.
	sharedOverrideFilePath string
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

		// Build the DS client for seeding (primary process).
		dsClients := integration.NewAuthenticatedDataStorageClients(
			sharedDSEndpoint, authConfig.Token, 10*time.Second,
		)
		sharedDSClient = dsClients.OpenAPIClient

		// ── Step 3b: Seed workflows into DataStorage (#1174) ──
		By("Seeding discovery workflows into DataStorage")
		discoveryWorkflows := []infrastructure.TestWorkflow{
			{WorkflowID: "oomkill-increase-memory-v1", Name: "OOMKill Recovery", ActionType: "IncreaseMemoryLimits", Environment: "production"},
			{WorkflowID: "generic-restart-v1", Name: "Generic Pod Restart", ActionType: "RestartPod", Environment: "production"},
		}
		workflowUUIDs, seedErr := infrastructure.SeedWorkflowsInDataStorage(
			sharedDSClient, discoveryWorkflows, "KA MCP IT", GinkgoWriter,
		)
		Expect(seedErr).ToNot(HaveOccurred(), "discovery workflows must seed in DataStorage")
		sharedWorkflowUUIDs = workflowUUIDs
		GinkgoWriter.Printf("Seeded %d workflows: %v\n", len(workflowUUIDs), workflowUUIDs)

		// ── Step 3c: Write scenarios override YAML for Mock LLM (#1174) ──
		By("Writing Mock LLM scenario overrides with DS-assigned UUIDs")
		type scenarioEntry struct {
			WorkflowID string `yaml:"workflow_id"`
		}
		scenarios := make(map[string]scenarioEntry, len(workflowUUIDs))
		for key, id := range workflowUUIDs {
			scenarios[key] = scenarioEntry{WorkflowID: id}
		}
		overrideYAML, yamlErr := yaml.Marshal(struct {
			Scenarios map[string]scenarioEntry `yaml:"scenarios"`
		}{Scenarios: scenarios})
		Expect(yamlErr).ToNot(HaveOccurred(), "scenario override YAML must serialize")

		overrideFile, writeErr := os.CreateTemp("", "kamcp-scenarios-*.yaml")
		Expect(writeErr).ToNot(HaveOccurred())
		_, writeErr = overrideFile.Write(overrideYAML)
		Expect(writeErr).ToNot(HaveOccurred())
		Expect(overrideFile.Close()).To(Succeed())
		sharedOverrideFilePath = overrideFile.Name()
		GinkgoWriter.Printf("Mock LLM override file: %s\n", sharedOverrideFilePath)

		// ── Step 4: Mock LLM (Podman, mode=interactive) ──
		By("Building Mock LLM image")
		mockLLMCfg := infrastructure.GetMockLLMConfigForKA()
		builtImageTag, err := infrastructure.BuildMockLLMImage(ctx, mockLLMCfg.ServiceName, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Mock LLM image should build")
		mockLLMCfg.ImageTag = builtImageTag
		mockLLMCfg.ConfigFilePath = overrideFile.Name()
		// DD-AUTH-014: Platform-specific network (matches AIAnalysis IT pattern)
		if runtime.GOOS == "linux" {
			mockLLMCfg.Network = "host"
		}

		By("Starting Mock LLM container (mode=interactive, with DS UUID overrides)")
		_, err = infrastructure.StartMockLLMContainer(ctx, mockLLMCfg, GinkgoWriter)
		Expect(err).ToNot(HaveOccurred(), "Mock LLM container should start")
		sharedMockLLMConfig = mockLLMCfg
		sharedMockLLMEndpoint = infrastructure.GetMockLLMEndpoint(mockLLMCfg)
		GinkgoWriter.Printf("Mock LLM endpoint: %s (mode=interactive)\n", sharedMockLLMEndpoint)

		GinkgoWriter.Println("Phase 0 complete — envtest + DataStorage + Mock LLM ready")

		// JSON payload for all Ginkgo processes (pattern: AIAnalysis IT suite).
		payload, marshalErr := json.Marshal(phase1Payload{
			Token:           authConfig.Token,
			KubeconfigPath:  kubeconfigPath,
			MockLLMEndpoint: sharedMockLLMEndpoint,
			WorkflowUUIDs:   workflowUUIDs,
		})
		Expect(marshalErr).ToNot(HaveOccurred())
		return payload
	},
	func(data []byte) {
		var p phase1Payload
		Expect(json.Unmarshal(data, &p)).To(Succeed(), "phase 1 JSON payload must deserialize")

		if sharedK8sConfig == nil {
			cfg, err := clientcmd.BuildConfigFromFlags("", p.KubeconfigPath)
			Expect(err).ToNot(HaveOccurred(), "secondary process should load rest.Config from kubeconfig")
			sharedK8sConfig = cfg
		}

		if sharedK8sClient == nil {
			scheme := runtime.NewScheme()
			Expect(coordinationv1.AddToScheme(scheme)).To(Succeed())
			Expect(corev1.AddToScheme(scheme)).To(Succeed())

			var err error
			sharedK8sClient, err = client.New(sharedK8sConfig, client.Options{Scheme: scheme})
			Expect(err).ToNot(HaveOccurred())
		}

		if sharedMockLLMEndpoint == "" {
			sharedMockLLMEndpoint = p.MockLLMEndpoint
		}

		dsURL := fmt.Sprintf("http://127.0.0.1:%d", mcpDataStoragePort)
		dsClients := integration.NewAuthenticatedDataStorageClients(dsURL, p.Token, 10*time.Second)
		sharedDSClient = dsClients.OpenAPIClient

		sharedWorkflowUUIDs = p.WorkflowUUIDs
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
		if sharedOverrideFilePath != "" {
			_ = os.Remove(sharedOverrideFilePath)
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
