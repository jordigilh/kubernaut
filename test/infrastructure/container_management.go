package infrastructure

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ============================================================================
// Generic Container Management (Reusable for Any Service)
// ============================================================================
//
// This file provides generic container start/stop abstractions that work with
// any container image (DataStorage, HAPI, Mock LLM, etc.). It implements the
// DD-TEST-002 sequential container orchestration pattern.
//
// Key Features:
// - Optional image building (if BuildContext + BuildDockerfile provided)
// - Health check support (HTTP endpoint polling)
// - Automatic cleanup of existing containers
// - Background log streaming for debugging
//
// Related:
// - datastorage_bootstrap.go: DataStorage-specific infrastructure
// - hapi_bootstrap.go: HAPI-specific image building
// - e2e_images.go: E2E/Kind image management
// ============================================================================

// GenericContainerConfig defines configuration for starting any container
// This abstraction allows services to bootstrap custom dependencies (e.g., HAPI for AIAnalysis)
// while reusing the proven sequential startup pattern from DD-TEST-002.
//
// Image Naming (DD-TEST-001 v1.3):
//
//	Use GenerateInfraImageName() helper for consistent tag generation:
//	Image: infrastructure.GenerateInfraImageName("holmesgpt-api", "aianalysis")
//	→ "holmesgpt-api:holmesgpt-api-aianalysis-1734278400-a1b2c3d4"
//
// Example Usage (AIAnalysis starting HAPI):
//
//	hapiConfig := infrastructure.GenericContainerConfig{
//	    Name:    "aianalysis_hapi_test",
//	    Image:   infrastructure.GenerateInfraImageName("holmesgpt-api", "aianalysis"), // DD-TEST-001 v1.3
//	    Network: "aianalysis_test_network",
//	    Ports:   map[int]int{8080: 18120}, // container:host
//	    Env: map[string]string{
//	        "LLM_PROVIDER": "mock",
//	        "MOCK_LLM":     "true",
//	    },
//	    BuildContext:    ".",                     // Optional: build if needed
//	    BuildDockerfile: "holmesgpt-api/Dockerfile.e2e", // Use E2E Dockerfile (minimal deps, faster builds)
//	    HealthCheck: &HealthCheckConfig{
//	        URL:     "http://127.0.0.1:18120/health",
//	        Timeout: 30 * time.Second,
//	    },
//	}
//	hapiContainer, err := infrastructure.StartGenericContainer(hapiConfig, writer)
type GenericContainerConfig struct {
	// Container Configuration
	Name    string            // Container name (e.g., "aianalysis_hapi_test")
	Image   string            // Container image (e.g., "robusta-dev/holmesgpt:latest")
	Network string            // Network to attach to (e.g., "aianalysis_test_network")
	Ports   map[int]int       // Port mappings: container_port -> host_port
	Env     map[string]string // Environment variables
	Volumes map[string]string // Volume mounts: host_path -> container_path

	// Build Configuration (optional, if image needs to be built)
	BuildContext    string            // Build context directory (e.g., project root)
	BuildDockerfile string            // Path to Dockerfile (relative to BuildContext)
	BuildArgs       map[string]string // Build arguments

	// Health Check Configuration (optional)
	HealthCheck *HealthCheckConfig
}

// HealthCheckConfig defines how to verify container health
type HealthCheckConfig struct {
	URL     string        // HTTP endpoint to check (e.g., "http://127.0.0.1:8080/health")
	Timeout time.Duration // Maximum time to wait for health check to pass
}

// ContainerInstance holds runtime information about a started container
type ContainerInstance struct {
	Name   string                 // Container name
	ID     string                 // Container ID from podman
	Ports  map[int]int            // Port mappings (container -> host)
	Config GenericContainerConfig // Original configuration
}

// StartGenericContainer starts a container using DD-TEST-002 sequential pattern
//
// Process:
// 1. Check if image exists, build if necessary (and BuildContext provided)
// 2. Stop and remove existing container with same name
// 3. Start container with specified configuration
// 4. Wait for health check to pass (if HealthCheck provided)
//
// Returns:
// - *ContainerInstance: Runtime information about started container
// - error: Any errors during container startup
func StartGenericContainer(cfg GenericContainerConfig, writer io.Writer) (*ContainerInstance, error) {
	_, _ = fmt.Fprintf(writer, "🚀 Starting container: %s\n", cfg.Name)

	// Step 1: Build image if needed
	if cfg.BuildContext != "" && cfg.BuildDockerfile != "" {
		checkCmd := exec.Command("podman", "image", "exists", cfg.Image)
		if checkCmd.Run() != nil {
			_, _ = fmt.Fprintf(writer, "   📦 Building image: %s\n", cfg.Image)
			if err := buildContainerImage(cfg, writer); err != nil {
				return nil, fmt.Errorf("failed to build image: %w", err)
			}
			_, _ = fmt.Fprintf(writer, "   ✅ Image built: %s\n", cfg.Image)
		}
	}

	// Step 2: Cleanup existing container
	_, _ = fmt.Fprintf(writer, "   🧹 Cleaning up existing container (if any)...\n")
	stopCmd := exec.Command("podman", "stop", cfg.Name)
	_ = stopCmd.Run() // Ignore errors

	rmCmd := exec.Command("podman", "rm", cfg.Name)
	_ = rmCmd.Run() // Ignore errors

	// Step 3: Build podman run command
	args := []string{"run", "-d", "--name", cfg.Name}

	// Add network
	if cfg.Network != "" {
		args = append(args, "--network", cfg.Network)
	}

	// Add port mappings
	// cfg.Ports format: map[containerPort]hostPort (e.g., 8080: 18120)
	// Podman format: hostPort:containerPort (e.g., 18120:8080)
	for containerPort, hostPort := range cfg.Ports {
		args = append(args, "-p", fmt.Sprintf("%d:%d", hostPort, containerPort))
	}

	// Add environment variables
	for key, value := range cfg.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", key, value))
	}

	// Add volumes
	for hostPath, containerPath := range cfg.Volumes {
		args = append(args, "-v", fmt.Sprintf("%s:%s", hostPath, containerPath))
	}

	// Add image
	args = append(args, cfg.Image)

	// Start container
	_, _ = fmt.Fprintf(writer, "   🐳 Starting container with image: %s\n", cfg.Image)
	cmd := exec.Command("podman", args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	// Get container ID
	inspectCmd := exec.Command("podman", "inspect", "--format", "{{.Id}}", cfg.Name)
	idBytes, err := inspectCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get container ID: %w", err)
	}
	containerID := strings.TrimSpace(string(idBytes))

	instance := &ContainerInstance{
		Name:   cfg.Name,
		ID:     containerID,
		Ports:  cfg.Ports,
		Config: cfg,
	}

	// Step 4: Health check
	if cfg.HealthCheck != nil {
		_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for health check: %s\n", cfg.HealthCheck.URL)
		if err := waitForContainerHealth(cfg.HealthCheck, writer); err != nil {
			// Print container logs for debugging
			_, _ = fmt.Fprintf(writer, "\n⚠️  Container failed health check. Logs:\n")
			logsCmd := exec.Command("podman", "logs", cfg.Name)
			logsCmd.Stdout = writer
			logsCmd.Stderr = writer
			_ = logsCmd.Run()
			return nil, fmt.Errorf("container health check failed: %w", err)
		}
		_, _ = fmt.Fprintf(writer, "   ✅ Health check passed\n")
	}

	// Step 5: Start streaming container logs in background (for runtime debugging)
	// This is critical for debugging HAPI audit events, Python exceptions, etc.
	go func() {
		logsCmd := exec.Command("podman", "logs", "-f", cfg.Name)
		logsCmd.Stdout = writer
		logsCmd.Stderr = writer
		_ = logsCmd.Run() // Will run until container stops
	}()
	_, _ = fmt.Fprintf(writer, "   📋 Container logs streaming to test output\n")

	_, _ = fmt.Fprintf(writer, "✅ Container ready: %s (ID: %s)\n\n", cfg.Name, containerID[:12])
	return instance, nil
}

// StopGenericContainer stops and removes a container
func StopGenericContainer(instance *ContainerInstance, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🛑 Stopping container: %s\n", instance.Name)

	stopCmd := exec.Command("podman", "stop", instance.Name)
	stopCmd.Stdout = writer
	stopCmd.Stderr = writer
	_ = stopCmd.Run() // Ignore errors

	rmCmd := exec.Command("podman", "rm", instance.Name)
	rmCmd.Stdout = writer
	rmCmd.Stderr = writer
	_ = rmCmd.Run() // Ignore errors

	_, _ = fmt.Fprintf(writer, "✅ Container stopped: %s\n", instance.Name)
	return nil
}

// buildContainerImage builds a container image using podman build
func buildContainerImage(cfg GenericContainerConfig, writer io.Writer) error {
	args := []string{"build", "-t", cfg.Image, "--force-rm=false"}

	// Add build args
	for key, value := range cfg.BuildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", key, value))
	}

	// Add dockerfile and context
	// BuildDockerfile can be relative to BuildContext or absolute
	dockerfilePath := cfg.BuildDockerfile
	if !filepath.IsAbs(dockerfilePath) {
		// Make it absolute by joining with BuildContext
		dockerfilePath = filepath.Join(cfg.BuildContext, dockerfilePath)
	}
	args = append(args, "-f", dockerfilePath, cfg.BuildContext)

	cmd := exec.Command("podman", args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		// Check if image was actually built despite error (podman cleanup issue)
		checkCmd := exec.Command("podman", "image", "exists", cfg.Image)
		if checkCmd.Run() == nil {
			_, _ = fmt.Fprintf(writer, "   ⚠️  Build completed with warnings (image exists): %s\n", cfg.Image)
			return nil // Image exists, treat as success
		}
		return err // Image doesn't exist, real failure
	}
	return nil
}

// waitForContainerHealth waits for HTTP health check to pass
func waitForContainerHealth(check *HealthCheckConfig, writer io.Writer) error {
	deadline := time.Now().Add(check.Timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(check.URL)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}

		// Log progress every 5 seconds
		elapsed := check.Timeout - time.Until(deadline)
		if elapsed.Seconds() > 0 && int(elapsed.Seconds())%5 == 0 {
			_, _ = fmt.Fprintf(writer, "   Still waiting for %s... (%.0fs elapsed)\n", check.URL, elapsed.Seconds())
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for %s after %v", check.URL, check.Timeout)
}
