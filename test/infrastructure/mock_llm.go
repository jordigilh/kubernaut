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

package infrastructure

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Mock LLM Service Integration Test Infrastructure
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Pattern: DD-INTEGRATION-001 v2.0 - Programmatic Podman Setup using Go
// Image Naming: DD-TEST-004 - Unique Resource Naming (GenerateInfraImageName)
// Port Allocation: DD-TEST-001 v2.5
//   HAPI Integration: 18140
//   AIAnalysis Integration: 18141
//   E2E (Kind ClusterIP): No external port (internal: http://mock-llm:8080)
//
// Purpose:
//   Provides standalone OpenAI-compatible mock LLM for integration and E2E tests
//   Replaces embedded mock logic in HolmesGPT-API business code
//
// Dependencies:
//   HAPI Integration Tests → Mock LLM (localhost:18140)
//   AIAnalysis Integration Tests → Mock LLM (localhost:18141)
//   HAPI E2E Tests → Mock LLM (Kind ClusterIP in kubernaut-system)
//   AIAnalysis E2E Tests → Mock LLM (Kind ClusterIP in kubernaut-system)
//
// Created: January 11, 2026
//   As part of Mock LLM Migration (docs/plans/MOCK_LLM_MIGRATION_PLAN.md v1.6.0)
//   Extracts test logic from HolmesGPT-API business code into standalone service
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// Port allocation per DD-TEST-001 v2.5 (Mock LLM Service)
// Integration Tests (Podman): Per-service isolation
//
//	HAPI: 18140
//	AIAnalysis: 18141
//
// E2E Tests (Kind): ClusterIP only (no NodePort)
const (
	MockLLMPortHAPI       = 18140 // HAPI integration tests (Podman)
	MockLLMPortAIAnalysis = 18141 // AIAnalysis integration tests (Podman)
	// E2E tests use ClusterIP in Kind (no external port needed)
)

// Container configuration (per-service naming)
const (
	MockLLMContainerNameHAPI       = "mock-llm-hapi"
	MockLLMContainerNameAIAnalysis = "mock-llm-aianalysis"
)

// MockLLMConfig specifies configuration for starting a Mock LLM container
type MockLLMConfig struct {
	ServiceName    string // "hapi" or "aianalysis" (for container naming)
	Port           int    // Host port to expose (per DD-TEST-001 v1.8)
	ContainerName  string // Unique container name per service
	ImageTag       string // Unique image tag per DD-TEST-004 (use GenerateInfraImageName)
	Network        string // Podman network for container-to-container communication (e.g., "aianalysis_test_network")
	ConfigFilePath string // Optional: Host path to scenarios.yaml file (DD-TEST-011 v2.0)
}

// BuildMockLLMImage builds the Mock LLM container image for integration tests
//
// Pattern: DD-INTEGRATION-001 v2.0 - Programmatic Podman Setup
// Image Naming: DD-TEST-004 - Unique Resource Naming
//
// CI/CD Optimization:
//   - If IMAGE_REGISTRY + IMAGE_TAG env vars are set: Pull from registry (ghcr.io)
//   - Otherwise: Build locally (existing behavior for local dev)
//   - Automatic fallback to local build if registry pull fails
//
// Returns: Full image name with tag (e.g., "localhost/mock-llm:hapi-abc123")
func BuildMockLLMImage(ctx context.Context, serviceName string, writer io.Writer) (string, error) {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Building Mock LLM Image (%s Integration Tests)\n", serviceName)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Build with stable base tag for Docker cache, then tag with unique name
	// Pattern: Build once with cache, tag multiple times for DD-TEST-004 compliance
	baseImageName := "localhost/mock-llm:latest"
	uniqueImageName := GenerateInfraImageName("mock-llm", serviceName)

	// DEBUG: Show environment variable status
	registry := os.Getenv("IMAGE_REGISTRY")
	tag := os.Getenv("IMAGE_TAG")
	_, _ = fmt.Fprintf(writer, "   🔍 Environment check: IMAGE_REGISTRY=%q IMAGE_TAG=%q\n", registry, tag)

	// CI/CD Optimization: Try to pull from registry if configured
	// Note: We try to pull with the unique image name, then tag as base for consistency
	if pulledImageName, pulled, err := tryPullFromRegistry(ctx, "mock-llm", uniqueImageName, writer); pulled {
		if err != nil {
			return "", err // Tag failed after successful pull
		}
		// Also tag as base image for cache consistency
		tagBaseCmd := exec.CommandContext(ctx, "podman", "tag", pulledImageName, baseImageName)
		_ = tagBaseCmd.Run()        // Ignore errors (not critical)
		return pulledImageName, nil // Use registry image
	}

	_, _ = fmt.Fprintf(writer, "🔨 Building Mock LLM image locally: %s (--no-cache for fresh code)\n", baseImageName)
	_, _ = fmt.Fprintf(writer, "   Will tag as: %s (DD-TEST-004 unique)\n", uniqueImageName)

	// Build context is test/services/mock-llm/
	projectRoot := getProjectRoot()
	buildContext := fmt.Sprintf("%s/test/services/mock-llm", projectRoot)

	// Build with --no-cache to ensure fresh code (addresses recurring cache issues)
	buildCmd := exec.CommandContext(ctx, "podman", "build",
		"--no-cache",
		"-t", baseImageName,
		"-f", fmt.Sprintf("%s/Dockerfile", buildContext),
		buildContext,
	)

	output, err := buildCmd.CombinedOutput()

	// ALWAYS show build output for debugging
	_, _ = fmt.Fprintf(writer, "\n📋 Podman Build Output:\n%s\n", string(output))

	if err != nil {
		return "", fmt.Errorf("failed to build Mock LLM image: %w\nOutput: %s", err, string(output))
	}

	_, _ = fmt.Fprintf(writer, "✅ Mock LLM image built (no cache): %s\n", baseImageName)

	// Tag with unique name for DD-TEST-004 compliance
	tagCmd := exec.CommandContext(ctx, "podman", "tag", baseImageName, uniqueImageName)
	if err := tagCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to tag Mock LLM image: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Tagged as unique image: %s\n\n", uniqueImageName)

	return uniqueImageName, nil
}

// GetMockLLMConfigForHAPI returns the Mock LLM configuration for HAPI integration tests
// Uses GenerateInfraImageName per DD-TEST-004 for unique image tags
func GetMockLLMConfigForHAPI() MockLLMConfig {
	return MockLLMConfig{
		ServiceName:   "hapi",
		Port:          MockLLMPortHAPI,
		ContainerName: MockLLMContainerNameHAPI,
		ImageTag:      GenerateInfraImageName("mock-llm", "hapi"),
	}
}

// GetMockLLMConfigForAIAnalysis returns the Mock LLM configuration for AIAnalysis integration tests
// Uses GenerateInfraImageName per DD-TEST-004 for unique image tags
func GetMockLLMConfigForAIAnalysis() MockLLMConfig {
	return MockLLMConfig{
		ServiceName:   "aianalysis",
		Port:          MockLLMPortAIAnalysis,
		ContainerName: MockLLMContainerNameAIAnalysis,
		ImageTag:      GenerateInfraImageName("mock-llm", "aianalysis"),
	}
}

// StartMockLLMContainer starts the Mock LLM container for integration tests
//
// Pattern: DD-TEST-002 Sequential Startup Pattern
// - Programmatic `podman run` command
// - Explicit health check with retries
// - Parallel-safe (called from SynchronizedBeforeSuite)
//
// Prerequisites:
//   - Mock LLM image built with unique tag per DD-TEST-004
//     Example: localhost/mock-llm:hapi-a3b5c7d9 (generated via GenerateInfraImageName)
//   - Ports per DD-TEST-001 v2.5: HAPI=18140, AIAnalysis=18141
//
// Returns:
// - containerID: Container ID for cleanup
// - error: Any errors during startup
func StartMockLLMContainer(ctx context.Context, config MockLLMConfig, writer io.Writer) (string, error) {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Starting Mock LLM Container (%s Integration Tests)\n", config.ServiceName)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Service: %s\n", config.ServiceName)
	_, _ = fmt.Fprintf(writer, "Container: %s\n", config.ContainerName)
	_, _ = fmt.Fprintf(writer, "Image: %s (DD-TEST-004 unique tag)\n", config.ImageTag)
	_, _ = fmt.Fprintf(writer, "Port: %d (DD-TEST-001 v2.5)\n", config.Port)
	_, _ = fmt.Fprintf(writer, "\n")

	// Check if container already exists and remove it
	_, _ = fmt.Fprintf(writer, "🔍 Checking for existing Mock LLM container...\n")
	checkCmd := exec.CommandContext(ctx, "podman", "ps", "-a", "--filter", "name=^"+config.ContainerName+"$", "--format", "{{.Names}}")
	if output, err := checkCmd.Output(); err == nil && len(output) > 0 && string(output) != "\n" {
		_, _ = fmt.Fprintf(writer, "⚠️  Mock LLM container already exists, removing...\n")
		rmCmd := exec.CommandContext(ctx, "podman", "rm", "-f", config.ContainerName)
		if err := rmCmd.Run(); err != nil {
			return "", fmt.Errorf("failed to remove existing Mock LLM container: %w", err)
		}
	}

	// Start Mock LLM container
	_, _ = fmt.Fprintf(writer, "🚀 Starting Mock LLM container...\n")

	// DD-AUTH-014: Platform-specific port configuration
	// - Bridge network: Internal port 8080 with port mapping (e.g., 18085:8080)
	// - Host network: Internal port matches external (e.g., 18085) since no port mapping
	internalPort := 8080
	if config.Network == "host" {
		internalPort = config.Port // Host network: Bind directly to external port
		_, _ = fmt.Fprintf(writer, "   🌐 Host network mode: Mock LLM will bind to port %d directly\n", internalPort)
	}

	args := []string{"run", "-d", "--rm",
		"--name", config.ContainerName,
		"-p", fmt.Sprintf("%d:%d", config.Port, internalPort), // Port mapping (ignored on host network)
		"-e", "MOCK_LLM_HOST=0.0.0.0",
		"-e", fmt.Sprintf("MOCK_LLM_PORT=%d", internalPort),
		"-e", "MOCK_LLM_FORCE_TEXT=false",
	}

	// Mount config file if specified (DD-TEST-011 v2.0)
	if config.ConfigFilePath != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/config/scenarios.yaml:ro", config.ConfigFilePath))
		args = append(args, "-e", "MOCK_LLM_CONFIG_PATH=/config/scenarios.yaml")
		_, _ = fmt.Fprintf(writer, "📋 Mounting config file: %s → /config/scenarios.yaml\n", config.ConfigFilePath)
	}

	// Add network if specified (for container-to-container communication)
	if config.Network != "" {
		args = append(args, "--network", config.Network)
		_, _ = fmt.Fprintf(writer, "📡 Joining Podman network: %s\n", config.Network)
	}

	args = append(args, config.ImageTag) // Use unique image tag per DD-TEST-004

	cmd := exec.CommandContext(ctx, "podman", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to start Mock LLM container: %w\nOutput: %s", err, string(output))
	}

	containerID := string(output)
	_, _ = fmt.Fprintf(writer, "✅ Mock LLM container started: %s\n", containerID[:12])

	// Wait for Mock LLM to be healthy
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for Mock LLM to be healthy...\n")
	if err := WaitForMockLLMHealthy(ctx, config.Port, writer); err != nil {
		// Cleanup on failure
		rmCmd := exec.CommandContext(ctx, "podman", "rm", "-f", config.ContainerName)
		_ = rmCmd.Run() // Ignore cleanup errors
		return "", fmt.Errorf("Mock LLM health check failed: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Mock LLM container is healthy and ready\n")
	_, _ = fmt.Fprintf(writer, "🌐 Mock LLM URL: http://localhost:%d\n", config.Port)
	_, _ = fmt.Fprintf(writer, "\n")

	return containerID, nil
}

// WaitForMockLLMHealthy waits for the Mock LLM service to respond to health checks
//
// Pattern: DD-TEST-002 Health Check Pattern
// - HTTP GET to /health endpoint
// - 30-second timeout with 1-second retry interval
// - Returns error if service doesn't become healthy
// - Uses 127.0.0.1 (not localhost) to avoid IPv6 mapping issues in CI/CD
func WaitForMockLLMHealthy(ctx context.Context, port int, writer io.Writer) error {
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	maxRetries := 30
	retryInterval := 1 * time.Second

	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while waiting for Mock LLM")
		default:
		}

		resp, err := http.Get(healthURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			_, _ = fmt.Fprintf(writer, "✅ Mock LLM health check passed (attempt %d/%d)\n", i+1, maxRetries)
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}

		if i < maxRetries-1 {
			_, _ = fmt.Fprintf(writer, "⏳ Mock LLM not ready yet (attempt %d/%d), retrying in %v...\n",
				i+1, maxRetries, retryInterval)
			time.Sleep(retryInterval)
		}
	}

	return fmt.Errorf("Mock LLM did not become healthy after %d seconds", maxRetries)
}

// StopMockLLMContainer stops and removes the Mock LLM container
//
// Pattern: DD-TEST-002 Cleanup Pattern
// - Called from SynchronizedAfterSuite (only on Ginkgo process 1)
// - Graceful shutdown with timeout
// - Idempotent (safe to call multiple times)
//
// Note: This function is called ONLY by Ginkgo process 1 after all parallel
// processes have completed. See MOCK_LLM_MIGRATION_PLAN.md v1.2.0 for details.
func StopMockLLMContainer(ctx context.Context, config MockLLMConfig, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Stopping Mock LLM Container (%s)\n", config.ServiceName)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Check if container exists
	checkCmd := exec.CommandContext(ctx, "podman", "ps", "-a", "--filter", "name=^"+config.ContainerName+"$", "--format", "{{.Names}}")
	output, err := checkCmd.Output()
	if err != nil || len(output) == 0 || string(output) == "\n" {
		_, _ = fmt.Fprintf(writer, "ℹ️  Mock LLM container does not exist, nothing to stop\n")
		return nil
	}

	// Stop container
	_, _ = fmt.Fprintf(writer, "🛑 Stopping Mock LLM container...\n")
	stopCmd := exec.CommandContext(ctx, "podman", "stop", "--time=5", config.ContainerName)
	stopCmd.Stdout = writer
	stopCmd.Stderr = writer
	if err := stopCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  Warning: Failed to stop container: %v\n", err)
	}

	// Remove container
	_, _ = fmt.Fprintf(writer, "🗑️  Removing Mock LLM container...\n")
	rmCmd := exec.CommandContext(ctx, "podman", "rm", "-f", config.ContainerName)
	rmCmd.Stdout = writer
	rmCmd.Stderr = writer
	if err := rmCmd.Run(); err != nil {
		return fmt.Errorf("failed to remove Mock LLM container: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Mock LLM container stopped and removed\n")
	return nil
}

// GetMockLLMEndpoint returns the Mock LLM endpoint URL for tests
//
// Usage in tests:
//
//	config := infrastructure.GetMockLLMConfigForHAPI()
//	endpoint := infrastructure.GetMockLLMEndpoint(config)
//	os.Setenv("LLM_ENDPOINT", endpoint)
//
// Note: Uses 127.0.0.1 (not localhost) to avoid IPv6 mapping issues in GitHub Actions CI/CD
// GetMockLLMEndpoint returns the Mock LLM endpoint for host-to-container communication
// Use this when accessing Mock LLM from the test host (e.g., health checks, curl)
func GetMockLLMEndpoint(config MockLLMConfig) string {
	return fmt.Sprintf("http://127.0.0.1:%d", config.Port)
}

// GetMockLLMContainerEndpoint returns the Mock LLM endpoint for container-to-container communication
// Use this when configuring services running in containers (e.g., HAPI LLM_ENDPOINT)
// Example: "http://mock-llm-aianalysis:8080"
func GetMockLLMContainerEndpoint(config MockLLMConfig) string {
	return fmt.Sprintf("http://%s:8080", config.ContainerName)
}

// MockLLMContainerInfo represents information about the Mock LLM container
type MockLLMContainerInfo struct {
	ContainerID   string
	ContainerName string
	ServiceName   string
	Port          int
	Endpoint      string
	HealthURL     string
	MetricsURL    string
}

// GetMockLLMContainerInfo returns comprehensive information about the Mock LLM container
//
// Useful for debugging and test setup validation
func GetMockLLMContainerInfo(containerID string, config MockLLMConfig) MockLLMContainerInfo {
	return MockLLMContainerInfo{
		ContainerID:   containerID,
		ContainerName: config.ContainerName,
		ServiceName:   config.ServiceName,
		Port:          config.Port,
		Endpoint:      GetMockLLMEndpoint(config),
		HealthURL:     fmt.Sprintf("http://127.0.0.1:%d/health", config.Port),
		MetricsURL:    fmt.Sprintf("http://127.0.0.1:%d/metrics", config.Port),
	}
}
