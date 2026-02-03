package infrastructure

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ============================================================================
// E2E Test Image Management (Build + Load to Kind + Cleanup)
// ============================================================================
//
// This file provides abstractions for building, loading, and cleaning up
// container images for E2E tests running in Kind clusters.
//
// Patterns Supported:
// 1. Standard Pattern: BuildAndLoadImageToKind() - build + load in one step
// 2. Hybrid Pattern: BuildImageForKind() → create cluster → LoadImageToKind()
//
// The hybrid pattern eliminates cluster idle time during image builds (~18% faster).
// See: docs/testing/e2e/E2E_PATTERN_PERFORMANCE_ANALYSIS_JAN07.md
//
// Related:
// - datastorage_bootstrap.go: DataStorage infrastructure bootstrap
// - container_management.go: Generic container start/stop operations
// ============================================================================

// E2EImageConfig configures image building and loading for E2E tests
type E2EImageConfig struct {
	ServiceName      string // Service name (e.g., "gateway", "aianalysis")
	ImageName        string // Base image name (e.g., "kubernaut/datastorage")
	DockerfilePath   string // Relative to project root (e.g., "docker/data-storage.Dockerfile")
	KindClusterName  string // Kind cluster name to load image into
	BuildContextPath string // Build context path, default: "." (project root)
	EnableCoverage   bool   // Enable Go coverage instrumentation (--build-arg GOFLAGS=-cover)
}

// PullImageFromRegistry pulls a container image from a registry (ghcr.io for CI/CD).
// This is used in GitHub Actions CI/CD to avoid building images locally (saves ~60% disk space).
//
// Registry Configuration (Environment Variables):
//   - IMAGE_REGISTRY: Registry URL (e.g., "ghcr.io/jordigilh/kubernaut")
//   - IMAGE_TAG: Image tag (e.g., "pr-123", "main-abc1234")
//
// Returns the full image name for later loading to Kind.
//
// Example (CI/CD):
//
//	IMAGE_REGISTRY=ghcr.io/jordigilh/kubernaut IMAGE_TAG=pr-123
//	imageName, err := PullImageFromRegistry("datastorage", writer)
//	// Returns: "ghcr.io/jordigilh/kubernaut/datastorage:pr-123"
func PullImageFromRegistry(serviceName string, writer io.Writer) (string, error) {
	registry := os.Getenv("IMAGE_REGISTRY")
	tag := os.Getenv("IMAGE_TAG")

	if registry == "" || tag == "" {
		return "", fmt.Errorf("IMAGE_REGISTRY or IMAGE_TAG not set (required for registry pull)")
	}

	fullImageName := fmt.Sprintf("%s/%s:%s", registry, serviceName, tag)
	_, _ = fmt.Fprintf(writer, "📥 Pulling image from registry: %s\n", fullImageName)

	// Pull image using podman (GitHub Actions uses podman for Kind)
	pullCmd := exec.Command("podman", "pull", fullImageName)
	pullCmd.Stdout = writer
	pullCmd.Stderr = writer
	pullStartTime := time.Now()
	_, _ = fmt.Fprintf(writer, "   ⏱️  Pull started: %s\n", pullStartTime.Format("15:04:05"))

	if err := pullCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to pull image from registry: %w", err)
	}

	pullDuration := time.Since(pullStartTime)
	_, _ = fmt.Fprintf(writer, "   ✅ Image pulled in %s: %s\n", pullDuration.Round(time.Second), fullImageName)

	return fullImageName, nil
}

// BuildImageForKind builds a container image for E2E testing.
// Returns the image name (with localhost/ prefix) for later loading to Kind.
//
// This is Phase 1 of the hybrid E2E pattern:
//
//	Phase 1: Build images (BEFORE cluster creation) ← THIS FUNCTION
//	Phase 2: Create Kind cluster
//	Phase 3: Load images to cluster (using LoadImageToKind)
//
// CI/CD Optimization (Fallback Strategy):
//   - If IMAGE_REGISTRY + IMAGE_TAG are set: Pull from registry (ghcr.io)
//   - Otherwise: Build locally (existing behavior for local dev)
//
// Authority: E2E_PATTERN_PERFORMANCE_ANALYSIS_JAN07.md
// Performance: Eliminates cluster idle time during image builds
//
// Example (CI/CD with registry):
//
//	IMAGE_REGISTRY=ghcr.io/jordigilh/kubernaut IMAGE_TAG=pr-123
//	imageName, err := BuildImageForKind(cfg, writer)
//	// Pulls from registry instead of building
//
// Example (Local dev):
//
//	// No IMAGE_REGISTRY/IMAGE_TAG set
//	imageName, err := BuildImageForKind(cfg, writer)
//	// Builds locally as before
func BuildImageForKind(cfg E2EImageConfig, writer io.Writer) (string, error) {
	// CI/CD Optimization: Pull from registry if configured
	if os.Getenv("IMAGE_REGISTRY") != "" && os.Getenv("IMAGE_TAG") != "" {
		_, _ = fmt.Fprintf(writer, "🔄 Registry mode detected (IMAGE_REGISTRY + IMAGE_TAG set)\n")
		_, _ = fmt.Fprintf(writer, "   Skipping local build, will pull from registry\n")

		// Extract service name from ImageName (remove repo prefix if present)
		// e.g., "kubernaut/datastorage" → "datastorage"
		parts := strings.Split(cfg.ImageName, "/")
		serviceName := parts[len(parts)-1]

		imageName, err := PullImageFromRegistry(serviceName, writer)
		if err != nil {
			_, _ = fmt.Fprintf(writer, "   ⚠️  Registry pull failed: %v\n", err)
			_, _ = fmt.Fprintf(writer, "   ⚠️  Falling back to local build...\n")
			// Continue with local build below
		} else {
			return imageName, nil
		}
	}
	projectRoot := getProjectRoot()

	if cfg.BuildContextPath == "" {
		cfg.BuildContextPath = projectRoot
	}

	// Generate DD-TEST-001 v1.3 compliant tag
	// Use ServiceName for infrastructure field (not full ImageName with repo prefix)
	// to avoid "/" in tags which Docker/Podman rejects
	imageTag := generateInfrastructureImageTag(cfg.ServiceName, cfg.ServiceName)
	fullImageName := fmt.Sprintf("%s:%s", cfg.ImageName, imageTag)

	// Podman automatically prefixes images with "localhost/" if no registry is specified
	// We need to use the same name for both build and load operations
	localImageName := fmt.Sprintf("localhost/%s", fullImageName)

	// Check if image already exists (cache hit) - DD-TEST-002 optimization
	checkCmd := exec.Command("podman", "image", "exists", localImageName)
	if checkCmd.Run() == nil {
		_, _ = fmt.Fprintf(writer, "   ✅ Image already exists (using cache): %s\n", fullImageName)
		return localImageName, nil
	}

	_, _ = fmt.Fprintf(writer, "🔨 Building E2E image: %s\n", fullImageName)

	// Build image with optional coverage instrumentation
	buildArgs := []string{"build", "-t", localImageName, "--no-cache"}

	// DD-TEST-007: E2E Coverage Collection
	// Support coverage instrumentation when E2E_COVERAGE=true or EnableCoverage flag is set
	if cfg.EnableCoverage || os.Getenv("E2E_COVERAGE") == "true" {
		buildArgs = append(buildArgs, "--build-arg", "GOFLAGS=-cover")
		_, _ = fmt.Fprintf(writer, "   📊 Building with coverage instrumentation (GOFLAGS=-cover)\n")
	}

	_, _ = fmt.Fprintf(writer, "   🚫 Building with --no-cache to ensure fresh code\n")
	buildArgs = append(buildArgs, "-f", filepath.Join(projectRoot, cfg.DockerfilePath), cfg.BuildContextPath)

	// DD-TEST-009: Add 15-minute timeout to prevent infinite hangs
	// Context: E2E tests were hanging indefinitely when Podman build processes stalled
	// during dependency downloads (especially Python packages in HAPI)
	buildCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	buildCmd := exec.CommandContext(buildCtx, "podman", buildArgs...)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	buildStartTime := time.Now()
	_, _ = fmt.Fprintf(writer, "   ⏱️  Build started: %s (15min timeout)\n", buildStartTime.Format("15:04:05"))

	if err := buildCmd.Run(); err != nil {
		if buildCtx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("build timed out after 15 minutes for %s", cfg.ServiceName)
		}
		return "", fmt.Errorf("failed to build E2E image: %w", err)
	}

	buildDuration := time.Since(buildStartTime)
	_, _ = fmt.Fprintf(writer, "   ✅ Image built in %s: %s\n", buildDuration.Round(time.Second), localImageName)

	return localImageName, nil
}

// LoadImageToKind loads a pre-built image to a Kind cluster.
// Steps: Export to tar → Load to Kind → Remove tar → Remove Podman image
//
// This is Phase 3 of the hybrid E2E pattern:
//
//	Phase 1: Build images (using BuildImageForKind)
//	Phase 2: Create Kind cluster
//	Phase 3: Load images to cluster ← THIS FUNCTION
//
// Authority: E2E_PATTERN_PERFORMANCE_ANALYSIS_JAN07.md
// Performance: Explicit load step after cluster creation eliminates idle time
//
// Parameters:
//   - imageName: Full image name with localhost/ prefix (from BuildImageForKind)
//   - serviceName: Service name for tar file naming (e.g., "datastorage")
//   - clusterName: Kind cluster name to load image into
//   - writer: Output writer for logging
//
// Example:
//
//	imageName, _ := BuildImageForKind(cfg, writer)
//	err := LoadImageToKind(imageName, "datastorage", "gateway-e2e", writer)
func LoadImageToKind(imageName, serviceName, clusterName string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "📦 Loading image to Kind cluster: %s\n", clusterName)

	// Extract tag from image name for tar filename
	// imageName format: "localhost/kubernaut/datastorage:tag-abc123"
	parts := strings.Split(imageName, ":")
	imageTag := "latest"
	if len(parts) > 1 {
		imageTag = parts[1]
	}

	// Create temporary tar file
	tmpFile := fmt.Sprintf("/tmp/%s-%s.tar", serviceName, imageTag)
	_, _ = fmt.Fprintf(writer, "   📦 Exporting image to: %s\n", tmpFile)
	saveCmd := exec.Command("podman", "save", "-o", tmpFile, imageName)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to export image: %w", err)
	}

	// Load tar file into Kind
	_, _ = fmt.Fprintf(writer, "   📦 Importing archive into Kind cluster...\n")
	loadCmd := exec.Command("kind", "load", "image-archive", tmpFile, "--name", clusterName)
	loadCmd.Env = append(os.Environ(), "KIND_EXPERIMENTAL_PROVIDER=podman")
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		// Clean up tar file on error
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to load image to Kind: %w", err)
	}

	// Clean up tar file
	if err := os.Remove(tmpFile); err != nil {
		_, _ = fmt.Fprintf(writer, "   ⚠️  Failed to remove temp file %s: %v\n", tmpFile, err)
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ Removed tar file: %s\n", tmpFile)
	}

	// CRITICAL: Delete Podman image immediately after Kind load to free disk space
	// Problem: Image exists in both Podman storage AND Kind = 2x disk usage
	// Solution: Once in Kind, we don't need the Podman copy anymore
	_, _ = fmt.Fprintf(writer, "   🗑️  Removing Podman image to free disk space...\n")
	rmiCmd := exec.Command("podman", "rmi", "-f", imageName)
	rmiCmd.Stdout = writer
	rmiCmd.Stderr = writer
	if err := rmiCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ⚠️  Failed to remove Podman image (non-fatal): %v\n", err)
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ Podman image removed: %s\n", imageName)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Image loaded to Kind\n")

	return nil
}

// BuildAndLoadImageToKind builds and loads an image to Kind in one step.
// This is a convenience wrapper for the standard (non-hybrid) E2E pattern.
//
// For hybrid pattern (build-before-cluster), use BuildImageForKind() and LoadImageToKind() separately.
//
// Authority: E2E_PATTERN_PERFORMANCE_ANALYSIS_JAN07.md
// Pattern: Standard (cluster-first, images build while cluster idles)
// Performance: 18% slower than hybrid pattern, but simpler for small services
//
// Example (Standard Pattern):
//
//	imageName, err := BuildAndLoadImageToKind(cfg, writer)
//
// Example (Hybrid Pattern - RECOMMENDED):
//
//	imageName, err := BuildImageForKind(cfg, writer)
//	createKindCluster(...)
//	err = LoadImageToKind(imageName, cfg.ServiceName, cfg.KindClusterName, writer)
func BuildAndLoadImageToKind(cfg E2EImageConfig, writer io.Writer) (string, error) {
	imageName, err := BuildImageForKind(cfg, writer)
	if err != nil {
		return "", err
	}

	if err := LoadImageToKind(imageName, cfg.ServiceName, cfg.KindClusterName, writer); err != nil {
		return "", err
	}

	return imageName, nil
}

// CleanupE2EImage removes a service image built for E2E tests
// Per DD-TEST-001 v1.3: Only kubernaut-built images are cleaned, not base images
//
// This should be called in AfterSuite to prevent disk space exhaustion.
//
// Example:
//
//	var _ = AfterSuite(func() {
//	    if e2eImageName != "" {
//	        _ = infrastructure.CleanupE2EImage(e2eImageName, GinkgoWriter)
//	    }
//	})
func CleanupE2EImage(imageName string, writer io.Writer) error {
	if imageName == "" {
		return nil
	}

	_, _ = fmt.Fprintf(writer, "🗑️  Removing E2E image: %s\n", imageName)
	rmiCmd := exec.Command("podman", "rmi", "-f", imageName)
	if err := rmiCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ⚠️  Failed to remove image (may not exist): %v\n", err)
		return err
	}
	_, _ = fmt.Fprintf(writer, "   ✅ E2E image removed\n")
	return nil
}

// CleanupE2EImages removes multiple service images (batch cleanup)
// Useful when multiple images were built for a test run.
//
// Example:
//
//	var _ = AfterSuite(func() {
//	    images := []string{gatewayImage, dataStorageImage, hapiImage}
//	    _ = infrastructure.CleanupE2EImages(images, GinkgoWriter)
//	})
func CleanupE2EImages(imageNames []string, writer io.Writer) error {
	var errs []error
	for _, imageName := range imageNames {
		if err := CleanupE2EImage(imageName, writer); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to cleanup %d images", len(errs))
	}
	return nil
}
