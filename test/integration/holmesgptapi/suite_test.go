/*
Copyright 2025 Jordi Gil.

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

package holmesgptapi

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/jordigilh/kubernaut/test/infrastructure"
	"github.com/jordigilh/kubernaut/test/shared/integration"
)

var (
	// Shared infrastructure (created in Phase 1, cleaned up in AfterSuite)
	testEnv                *envtest.Environment
	hapiInfrastructure     *infrastructure.HAPIIntegrationInfra
	envtestKubeconfigIPv4  string // IPv4-rewritten kubeconfig for DataStorage auth
	serviceAccountToken    string // ServiceAccount token for Python tests (DD-AUTH-014)
)

func TestHolmesGPTAPIIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HolmesGPT API Integration Suite (Go Infrastructure)")
}

// SynchronizedBeforeSuite runs ONCE globally before all parallel processes start
// This follows the pattern established by Gateway, Notification, AIAnalysis, etc.
//
// Pattern: DD-INTEGRATION-001 v2.0 + DD-AUTH-014 (DataStorage auth)
// Migration: From Python pytest fixtures to Go programmatic setup with envtest
var _ = SynchronizedBeforeSuite(func() []byte {
	// This runs ONCE on process 1 only - creates shared infrastructure
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("HolmesGPT API Integration Test Suite - Go Infrastructure")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("Migration: From Python subprocess → Go programmatic setup")
	GinkgoWriter.Println("Pattern:   DD-INTEGRATION-001 v2.0 + DD-AUTH-014 (envtest + auth)")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Creating test infrastructure...")
	GinkgoWriter.Println("  • envtest (Kubernetes API)")
	GinkgoWriter.Println("  • PostgreSQL (port 15439)")
	GinkgoWriter.Println("  • Redis (port 16387)")
	GinkgoWriter.Println("  • Data Storage API (port 18098, with auth)")
	GinkgoWriter.Println("  • Mock LLM Service (port 18140)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	By("Starting envtest Kubernetes API server (DD-AUTH-014 requirement)")
	testEnv = &envtest.Environment{}
	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred(), "envtest must start successfully")
	Expect(cfg).NotTo(BeNil())
	GinkgoWriter.Println("✅ envtest started")

	By("Creating ServiceAccount with DataStorage RBAC in shared envtest")
	// DD-AUTH-014: Creates ServiceAccounts + RBAC for DataStorage access
	// This creates:
	// - holmesgptapi-ds-client ServiceAccount (for HAPI to call DataStorage)
	// - datastorage-service ServiceAccount (for DataStorage to validate tokens)
	// Also writes IPv4-rewritten kubeconfig for container compatibility
	authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
		cfg,
		"holmesgptapi-ds-client",
		"default",
		GinkgoWriter,
	)
	Expect(err).ToNot(HaveOccurred())
	envtestKubeconfigIPv4 = authConfig.KubeconfigPath // Store for Phase 2
	serviceAccountToken = authConfig.Token             // Store for Python container (DD-AUTH-014)
	GinkgoWriter.Printf("✅ ServiceAccount + RBAC created, kubeconfig written: %s\n", envtestKubeconfigIPv4)
	GinkgoWriter.Printf("✅ ServiceAccount token available for Python tests (DD-AUTH-014)\n\n")

	By("Starting HolmesGPT API integration infrastructure (Go programmatic setup)")
	// This starts: PostgreSQL, Redis, DataStorage (with auth)
	// Per DD-TEST-001 v1.8: Ports 15439, 16387, 18098
	// Per DD-AUTH-014: Uses envtest kubeconfig for DataStorage auth middleware
	var infraPtr *infrastructure.HAPIIntegrationInfra
	infraPtr, err = infrastructure.StartHolmesGPTAPIIntegrationInfrastructure(GinkgoWriter, authConfig.KubeconfigPath)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully (DD-INTEGRATION-001 v2.0)")
	hapiInfrastructure = infraPtr // Store in suite-level variable for cleanup
	GinkgoWriter.Println("✅ All services started and healthy (standardized StartDSBootstrap pattern)")

	// DD-TEST-011 v2.0: Seed workflows BEFORE Mock LLM starts (matches AIAnalysis pattern)
	// Must seed workflows first so Mock LLM can load UUIDs at startup
	// DD-AUTH-014: Use authenticated client
	By("Seeding test workflows into DataStorage (with authentication)")
	seedClient := integration.NewAuthenticatedDataStorageClients(
		"http://127.0.0.1:18098",
		authConfig.Token,
		5*time.Second,
	)
	workflowUUIDs, err := SeedTestWorkflowsInDataStorage(seedClient.OpenAPIClient, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Test workflows must be seeded successfully")

	// Write Mock LLM config file with workflow UUIDs
	// Pattern: DD-TEST-011 v2.0 - File-Based Configuration (matches AIAnalysis)
	// Mock LLM will read this file at startup (no HTTP calls required)
	By("Writing Mock LLM configuration file with workflow UUIDs")
	mockLLMConfigPath, err := filepath.Abs("mock-llm-hapi-config.yaml")
	Expect(err).ToNot(HaveOccurred(), "Must get absolute path for config file")
	err = WriteMockLLMConfigFile(mockLLMConfigPath, workflowUUIDs, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Mock LLM config file must be written successfully")

	By("Building Mock LLM image (DD-TEST-004 unique tag)")
	// Per DD-TEST-004: Generate unique image tag to prevent collisions
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	mockLLMImageName, err := infrastructure.BuildMockLLMImage(ctx, "hapi", GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Mock LLM image must build successfully")
	Expect(mockLLMImageName).ToNot(BeEmpty(), "Mock LLM image name must be returned")
	GinkgoWriter.Printf("✅ Mock LLM image built: %s\n", mockLLMImageName)

	By("Starting Mock LLM service with configuration file (DD-TEST-011 v2.0)")
	// Per DD-TEST-001 v1.8: Port 18140 (HAPI-specific, unique)
	// Per MOCK_LLM_MIGRATION_PLAN.md v1.3.0: Standalone service for test isolation
	mockLLMConfig := infrastructure.GetMockLLMConfigForHAPI()
	mockLLMConfig.ImageTag = mockLLMImageName          // Use the built image tag
	mockLLMConfig.ConfigFilePath = mockLLMConfigPath   // DD-TEST-011 v2.0: Mount config file
	mockLLMContainerID, err := infrastructure.StartMockLLMContainer(ctx, mockLLMConfig, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Mock LLM container must start successfully")
	Expect(mockLLMContainerID).ToNot(BeEmpty(), "Mock LLM container ID must be returned")
	GinkgoWriter.Printf("✅ Mock LLM service started with config file (port %d)\n", mockLLMConfig.Port)

	// FIX: HAPI-INT-CONFIG-001 - Share envtest kubeconfig and token between parallel processes
	// Pattern: Same as AIAnalysis, Gateway (JSON marshaling for robust data passing)
	// DD-AUTH-014: Share ServiceAccount token for Python test auth
	type Phase1Data struct {
		EnvtestKubeconfigIPv4 string `json:"envtest_kubeconfig_ipv4"`
		Token                 string `json:"token"`
	}
	data := Phase1Data{
		EnvtestKubeconfigIPv4: envtestKubeconfigIPv4,
		Token:                 authConfig.Token,
	}
	dataBytes, err := json.Marshal(data)
	Expect(err).NotTo(HaveOccurred(), "Phase 1 data marshaling must succeed")
	return dataBytes
}, func(data []byte) {
	// This runs on ALL parallel processes - setup per-process resources if needed
	// FIX: HAPI-INT-CONFIG-001 - Unmarshal shared data from Phase 1
	type Phase1Data struct {
		EnvtestKubeconfigIPv4 string `json:"envtest_kubeconfig_ipv4"`
		Token                 string `json:"token"`
	}
	var sharedData Phase1Data
	err := json.Unmarshal(data, &sharedData)
	Expect(err).NotTo(HaveOccurred(), "Phase 1 data unmarshaling must succeed")
	
	envtestKubeconfigIPv4 = sharedData.EnvtestKubeconfigIPv4
	serviceAccountToken = sharedData.Token
	GinkgoWriter.Printf("✅ Process setup complete (infrastructure shared, kubeconfig=%s)\n", envtestKubeconfigIPv4)
})

// SynchronizedAfterSuite runs cleanup in two phases for parallel execution
var _ = SynchronizedAfterSuite(func() {
	// This runs on ALL processes - per-process cleanup (none needed for HAPI)
	GinkgoWriter.Printf("🧹 Process cleanup complete\n")
}, func() {
	// This runs ONCE on process 1 only - tears down shared infrastructure
	By("Tearing down HolmesGPT API integration infrastructure")

	// DD-TEST-DIAGNOSTICS: Must-gather container logs for post-mortem analysis
	// ALWAYS collect logs - failures may have occurred on other parallel processes
	// The overhead is minimal (~2s) and logs are invaluable for debugging flaky tests
	GinkgoWriter.Println("📦 Collecting container logs for post-mortem analysis...")
	infrastructure.MustGatherContainerLogs("holmesgptapi", []string{
		infrastructure.HAPIIntegrationDataStorageContainer, // holmesgptapi_datastorage_1
		infrastructure.HAPIIntegrationPostgresContainer,    // holmesgptapi_postgres_1
		infrastructure.HAPIIntegrationRedisContainer,       // holmesgptapi_redis_1
		infrastructure.MockLLMContainerNameHAPI,            // mock-llm-hapi
	}, GinkgoWriter)

	// Stop Mock LLM service first
	By("Stopping Mock LLM service (HAPI-specific)")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	mockLLMConfig := infrastructure.GetMockLLMConfigForHAPI()
	if err := infrastructure.StopMockLLMContainer(ctx, mockLLMConfig, GinkgoWriter); err != nil {
		GinkgoWriter.Printf("⚠️  Failed to stop Mock LLM container: %v\n", err)
	}

	// DD-INTEGRATION-001 v2.0: Stop infrastructure using programmatic Go setup
	By("Stopping HolmesGPT API infrastructure (programmatic Podman)")
	if err := infrastructure.StopHolmesGPTAPIIntegrationInfrastructure(hapiInfrastructure, GinkgoWriter); err != nil {
		GinkgoWriter.Printf("⚠️  Failed to stop containers: %v\n", err)
	}

	// Stop envtest
	By("Stopping shared envtest")
	if testEnv != nil {
		if err := testEnv.Stop(); err != nil {
			GinkgoWriter.Printf("⚠️  Failed to stop envtest: %v\n", err)
		}
	}

	// Cleanup Mock LLM config file (DD-TEST-011 v2.0)
	By("Cleaning up Mock LLM configuration file")
	configPath, _ := filepath.Abs("mock-llm-hapi-config.yaml")
	if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
		GinkgoWriter.Printf("⚠️  Failed to remove Mock LLM config file: %v\n", err)
	}

	// DD-INTEGRATION-001 v2.0: Clean up composite-tagged images
	By("Cleaning up infrastructure images to prevent disk space issues")
	pruneCmd := exec.Command("podman", "image", "prune", "-f")
	if pruneErr := pruneCmd.Run(); pruneErr != nil {
		GinkgoWriter.Printf("⚠️  Failed to prune images: %v\n", pruneErr)
	}

	GinkgoWriter.Println("✅ Infrastructure teardown complete (DD-INTEGRATION-001 v2.0)")
})
