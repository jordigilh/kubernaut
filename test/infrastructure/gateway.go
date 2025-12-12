package infrastructure

import (
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"time"
)

// Gateway Integration Test Infrastructure Constants
// Port Allocation per DD-TEST-001: Port Allocation Strategy
const (
	// Gateway Integration Test Ports
	GatewayIntegrationPostgresPort    = 15437 // PostgreSQL (DataStorage backend)
	GatewayIntegrationRedisPort       = 16383 // Redis (DataStorage DLQ)
	GatewayIntegrationDataStoragePort = 18091 // DataStorage API (Audit + State)

	// Compose Configuration
	GatewayIntegrationComposeFile    = "test/integration/gateway/podman-compose.gateway.test.yml"
	GatewayIntegrationComposeProject = "gateway-integration-test"
)

// StartGatewayIntegrationInfrastructure starts the Gateway integration test infrastructure
// using podman-compose following the AIAnalysis pattern.
//
// Pattern: AIAnalysis Pattern (Programmatic podman-compose)
// - Declarative infrastructure (podman-compose.gateway.test.yml)
// - Programmatic lifecycle management (this function)
// - Health checks for robust startup
// - Parallel-safe with unique ports (DD-TEST-001)
//
// Infrastructure Components:
// - PostgreSQL (port 15437): DataStorage backend
// - Redis (port 16383): DataStorage DLQ
// - DataStorage API (port 18091): Audit events + State storage
//
// Returns:
// - error: Any errors during infrastructure startup
func StartGatewayIntegrationInfrastructure(writer io.Writer) error {
	projectRoot := getProjectRoot()
	composeFile := filepath.Join(projectRoot, GatewayIntegrationComposeFile)

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Starting Gateway Integration Test Infrastructure\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", GatewayIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:          localhost:%d\n", GatewayIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", GatewayIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  Compose File:   %s\n", GatewayIntegrationComposeFile)
	fmt.Fprintf(writer, "  Pattern:        AIAnalysis (Programmatic podman-compose)\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Check if podman-compose is available
	if err := exec.Command("podman-compose", "--version").Run(); err != nil {
		return fmt.Errorf("podman-compose not found: %w (install via: pip install podman-compose)", err)
	}

	// Start services
	cmd := exec.Command("podman-compose",
		"-f", composeFile,
		"-p", GatewayIntegrationComposeProject,
		"up", "-d", "--build",
	)
	cmd.Dir = projectRoot
	cmd.Stdout = writer
	cmd.Stderr = writer

	fmt.Fprintf(writer, "⏳ Starting containers (postgres, redis, datastorage)...\n")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start podman-compose stack: %w", err)
	}

	// Wait for services to be healthy
	fmt.Fprintf(writer, "⏳ Waiting for services to be healthy...\n")

	// Wait for DataStorage (this validates postgres + redis + migrations + datastorage)
	if err := waitForGatewayHTTPHealth(
		fmt.Sprintf("http://localhost:%d/health", GatewayIntegrationDataStoragePort),
		90*time.Second, // Longer timeout for migrations + build
		writer,
	); err != nil {
		return fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "✅ DataStorage is healthy (postgres + redis + migrations validated)\n")

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "✅ Gateway Integration Infrastructure Ready\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// StopGatewayIntegrationInfrastructure stops and cleans up the Gateway integration test infrastructure
//
// Pattern: AIAnalysis Pattern (per TRIAGE_INTEGRATION_TEST_INFRASTRUCTURE_PATTERNS.md)
// - Programmatic podman-compose cleanup
// - Removes volumes (-v flag)
// - Parallel-safe (called from SynchronizedAfterSuite)
//
// Returns:
// - error: Any errors during infrastructure cleanup
func StopGatewayIntegrationInfrastructure(writer io.Writer) error {
	projectRoot := getProjectRoot()
	composeFile := filepath.Join(projectRoot, GatewayIntegrationComposeFile)

	fmt.Fprintf(writer, "🛑 Stopping Gateway Integration Infrastructure...\n")

	// Stop and remove containers + volumes
	cmd := exec.Command("podman-compose",
		"-f", composeFile,
		"-p", GatewayIntegrationComposeProject,
		"down", "-v",
	)
	cmd.Dir = projectRoot
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(writer, "⚠️  Warning: Error stopping infrastructure: %v\n", err)
		return err
	}

	fmt.Fprintf(writer, "✅ Gateway Integration Infrastructure stopped and cleaned up\n")
	return nil
}

// waitForGatewayHTTPHealth waits for an HTTP health endpoint to respond with 200 OK
// Pattern: AIAnalysis health check with verbose logging
func waitForGatewayHTTPHealth(healthURL string, timeout time.Duration, writer io.Writer) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}

		// Log progress every 10 seconds
		if time.Now().Unix()%10 == 0 {
			fmt.Fprintf(writer, "   Still waiting for %s...\n", healthURL)
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for %s to become healthy after %v", healthURL, timeout)
}

// Note: getProjectRoot() is defined in aianalysis.go and shared across all infrastructure files
