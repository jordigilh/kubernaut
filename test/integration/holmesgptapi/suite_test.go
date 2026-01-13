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
	"os/exec"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/test/infrastructure"
)

func TestHolmesGPTAPIIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HolmesGPT API Integration Suite (Go Infrastructure)")
}

// SynchronizedBeforeSuite runs ONCE globally before all parallel processes start
// This follows the pattern established by Gateway, Notification, AIAnalysis, etc.
//
// Pattern: DD-INTEGRATION-001 v2.0 - Programmatic Go Infrastructure
// Migration: From Python pytest fixtures (subprocess.run docker-compose) to Go programmatic setup
var _ = SynchronizedBeforeSuite(func() []byte {
	// This runs ONCE on process 1 only - creates shared infrastructure
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("HolmesGPT API Integration Test Suite - Go Infrastructure")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("Migration: From Python subprocess → Go programmatic setup")
	GinkgoWriter.Println("Pattern:   DD-INTEGRATION-001 v2.0 (Programmatic Podman)")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Creating test infrastructure...")
	GinkgoWriter.Println("  • PostgreSQL (port 15439)")
	GinkgoWriter.Println("  • Redis (port 16387)")
	GinkgoWriter.Println("  • Data Storage API (port 18098)")
	GinkgoWriter.Println("  • Mock LLM Service (port 18140 - HAPI-specific)")
	GinkgoWriter.Println("")
	GinkgoWriter.Println("Benefits over Python subprocess approach:")
	GinkgoWriter.Println("  ✅ No subprocess.run() calls")
	GinkgoWriter.Println("  ✅ Reuses 720 lines of shared Go utilities")
	GinkgoWriter.Println("  ✅ Consistent with all other services")
	GinkgoWriter.Println("  ✅ Programmatic health checks")
	GinkgoWriter.Println("  ✅ Composite image tags (collision avoidance)")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	By("Starting HolmesGPT API integration infrastructure (Go programmatic setup)")
	// This starts: PostgreSQL, Redis, DataStorage
	// Per DD-TEST-001 v1.8: Ports 15439, 16387, 18098
	// Per DD-INTEGRATION-001 v2.0: Uses shared utilities from test/infrastructure/shared_integration_utils.go
	err := infrastructure.StartHolmesGPTAPIIntegrationInfrastructure(GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Infrastructure must start successfully (DD-INTEGRATION-001 v2.0)")
	GinkgoWriter.Println("✅ All services started and healthy (Go programmatic setup)")

	By("Building Mock LLM image (DD-TEST-004 unique tag)")
	// Per DD-TEST-004: Generate unique image tag to prevent collisions
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	mockLLMImageName, err := infrastructure.BuildMockLLMImage(ctx, "hapi", GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Mock LLM image must build successfully")
	Expect(mockLLMImageName).ToNot(BeEmpty(), "Mock LLM image name must be returned")
	GinkgoWriter.Printf("✅ Mock LLM image built: %s\n", mockLLMImageName)

	By("Starting Mock LLM service (replaces embedded mock logic)")
	// Per DD-TEST-001 v1.8: Port 18140 (HAPI-specific, unique)
	// Per MOCK_LLM_MIGRATION_PLAN.md v1.3.0: Standalone service for test isolation
	mockLLMConfig := infrastructure.GetMockLLMConfigForHAPI()
	mockLLMConfig.ImageTag = mockLLMImageName // Use the built image tag
	mockLLMContainerID, err := infrastructure.StartMockLLMContainer(ctx, mockLLMConfig, GinkgoWriter)
	Expect(err).ToNot(HaveOccurred(), "Mock LLM container must start successfully")
	Expect(mockLLMContainerID).ToNot(BeEmpty(), "Mock LLM container ID must be returned")
	GinkgoWriter.Printf("✅ Mock LLM service started and healthy (port %d)\n", mockLLMConfig.Port)

	return nil // No shared data needed between processes
}, func(data []byte) {
	// This runs on ALL parallel processes - setup per-process resources if needed
	GinkgoWriter.Printf("✅ Process setup complete (infrastructure shared across processes)\n")
})

// SynchronizedAfterSuite runs cleanup in two phases for parallel execution
var _ = SynchronizedAfterSuite(func() {
	// This runs on ALL processes - per-process cleanup (none needed for HAPI)
	GinkgoWriter.Printf("🧹 Process cleanup complete\n")
}, func() {
	// This runs ONCE on process 1 only - tears down shared infrastructure
	By("Tearing down HolmesGPT API integration infrastructure")

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
	if err := infrastructure.StopHolmesGPTAPIIntegrationInfrastructure(GinkgoWriter); err != nil {
		GinkgoWriter.Printf("⚠️  Failed to stop containers: %v\n", err)
	}

	// DD-INTEGRATION-001 v2.0: Clean up composite-tagged images
	By("Cleaning up infrastructure images to prevent disk space issues")
	pruneCmd := exec.Command("podman", "image", "prune", "-f")
	if pruneErr := pruneCmd.Run(); pruneErr != nil {
		GinkgoWriter.Printf("⚠️  Failed to prune images: %v\n", pruneErr)
	}

	GinkgoWriter.Println("✅ Infrastructure teardown complete (DD-INTEGRATION-001 v2.0)")
})
