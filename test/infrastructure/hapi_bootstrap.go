package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// ============================================================================
// HAPI (HolmesGPT API) Image Building
// ============================================================================
//
// This file contains HolmesGPT-API specific infrastructure helpers for
// integration tests. HAPI is used by AIAnalysis for incident analysis via OpenAPI client.
//
// Related:
// - container_management.go: Generic container start/stop operations
// - datastorage_bootstrap.go: DataStorage infrastructure bootstrap
// ============================================================================

// BuildHAPIImage builds the HolmesGPT-API image for integration tests
// Returns the full image name with tag for use in StartGenericContainer
//
// Image Naming (DD-TEST-001 v1.3):
//   - Generated tag includes: infrastructure + serviceName + timestamp + hash
//   - Example: "localhost/holmesgpt-api:holmesgpt-api-aianalysis-1734278400-a1b2c3d4"
//
// Usage:
//
//	hapiImageName, err := infrastructure.BuildHAPIImage(ctx, "aianalysis", writer)
//	hapiConfig := infrastructure.GenericContainerConfig{
//	    Image: hapiImageName,
//	    // ... other config
//	}
//	container, err := infrastructure.StartGenericContainer(hapiConfig, writer)
func BuildHAPIImage(ctx context.Context, serviceName string, writer io.Writer) (string, error) {
	projectRoot := getProjectRoot()

	// Generate DD-TEST-001 v1.3 compliant image tag
	imageTag := generateInfrastructureImageTag("holmesgpt-api", serviceName)
	localImageName := fmt.Sprintf("localhost/holmesgpt-api:%s", imageTag) // Podman auto-prefixes with localhost/

	// DEBUG: Show environment variable status
	registry := os.Getenv("IMAGE_REGISTRY")
	tag := os.Getenv("IMAGE_TAG")
	_, _ = fmt.Fprintf(writer, "   🔍 Environment check: IMAGE_REGISTRY=%q IMAGE_TAG=%q\n", registry, tag)

	// CI/CD Optimization: Try to pull from registry if IMAGE_REGISTRY + IMAGE_TAG are set
	registryImage, pulled, err := tryPullFromRegistry(ctx, "holmesgpt-api", localImageName, writer)
	if err != nil {
		return "", fmt.Errorf("failed during registry pull attempt: %w", err)
	}
	if pulled {
		// Success! Return the registry-pulled image (already tagged as localImageName)
		return registryImage, nil
	}

	// Registry pull not available or failed - proceed with local build
	// Check if image already exists (cache hit)
	checkCmd := exec.CommandContext(ctx, "podman", "image", "exists", localImageName)
	if checkCmd.Run() == nil {
		_, _ = fmt.Fprintf(writer, "   ✅ HAPI image already exists: %s\n", localImageName)
		return localImageName, nil
	}

	// Build the image
	_, _ = fmt.Fprintf(writer, "   🔨 Building HAPI image (tag: %s)...\n", imageTag)
	buildCmd := exec.CommandContext(ctx, "podman", "build",
		"-t", localImageName,
		"--force-rm=false",                                                  // Disable auto-cleanup to avoid podman cleanup errors
		"-f", filepath.Join(projectRoot, "holmesgpt-api", "Dockerfile.e2e"), // E2E Dockerfile: minimal dependencies, no lib64 issues
		projectRoot,
	)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		// Check if image was actually built despite error (podman cleanup issue)
		checkAgain := exec.Command("podman", "image", "exists", localImageName)
		if checkAgain.Run() == nil {
			_, _ = fmt.Fprintf(writer, "   ⚠️  Build completed with warnings (image exists): %s\n", localImageName)
			return localImageName, nil // Image exists, treat as success
		}
		return "", fmt.Errorf("failed to build HAPI image: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ HAPI image built: %s\n", localImageName)
	return localImageName, nil
}

// BuildKubernautAgentImage builds the Go Kubernaut Agent image for integration tests.
// Uses the same tag generation and caching pattern as BuildHAPIImage.
func BuildKubernautAgentImage(ctx context.Context, serviceName string, writer io.Writer) (string, error) {
	projectRoot := getProjectRoot()

	imageTag := generateInfrastructureImageTag("kubernautagent", serviceName)
	localImageName := fmt.Sprintf("localhost/kubernautagent:%s", imageTag)

	registry := os.Getenv("IMAGE_REGISTRY")
	tag := os.Getenv("IMAGE_TAG")
	_, _ = fmt.Fprintf(writer, "   🔍 Environment check: IMAGE_REGISTRY=%q IMAGE_TAG=%q\n", registry, tag)

	registryImage, pulled, err := tryPullFromRegistry(ctx, "kubernautagent", localImageName, writer)
	if err != nil {
		return "", fmt.Errorf("failed during registry pull attempt: %w", err)
	}
	if pulled {
		return registryImage, nil
	}

	checkCmd := exec.CommandContext(ctx, "podman", "image", "exists", localImageName)
	if checkCmd.Run() == nil {
		_, _ = fmt.Fprintf(writer, "   ✅ KA image already exists: %s\n", localImageName)
		return localImageName, nil
	}

	_, _ = fmt.Fprintf(writer, "   🔨 Building KA image (tag: %s)...\n", imageTag)
	buildCmd := exec.CommandContext(ctx, "podman", "build",
		"-t", localImageName,
		"--force-rm=false",
		"-f", filepath.Join(projectRoot, "docker", "kubernautagent.Dockerfile"),
		projectRoot,
	)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		checkAgain := exec.Command("podman", "image", "exists", localImageName)
		if checkAgain.Run() == nil {
			_, _ = fmt.Fprintf(writer, "   ⚠️  Build completed with warnings (image exists): %s\n", localImageName)
			return localImageName, nil
		}
		return "", fmt.Errorf("failed to build KA image: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ KA image built: %s\n", localImageName)
	return localImageName, nil
}
