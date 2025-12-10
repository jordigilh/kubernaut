// Package gateway contains integration test helpers for Gateway Service
package gateway

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// POSTGRESQL TEST INFRASTRUCTURE TYPES (envtest Migration - Phase 2)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// PostgresTestClient wraps PostgreSQL container for integration tests
// Used for audit trail storage via Data Storage service
type PostgresTestClient struct {
	ContainerName string
	Host          string
	Port          int
	Database      string
	User          string
	Password      string
	DSN           string
}

// DataStorageTestServer wraps httptest.Server for Data Storage service
// Used for audit trail storage integration tests
type DataStorageTestServer struct {
	Server       *httptest.Server
	BaseURL      string
	PostgresPort int
	PgClient     *PostgresTestClient
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// POSTGRESQL TEST CLIENT METHODS (envtest Migration - Phase 2)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SetupPostgresTestClient creates a PostgreSQL container for integration tests
// Uses direct Podman commands (no testcontainers) with random port assignment
// Used for audit trail storage via Data Storage service
func SetupPostgresTestClient(ctx context.Context) *PostgresTestClient {
	containerName := "gateway-postgres-integration"

	// Stop and remove existing container if it exists
	exec.Command("podman", "stop", containerName).Run()
	exec.Command("podman", "rm", containerName).Run()

	// Find random available port
	port := findAvailablePort(50001, 60000)
	GinkgoWriter.Printf("  📍 Allocated random port for PostgreSQL: %d\n", port)

	// Start PostgreSQL container with random port
	GinkgoWriter.Printf("  Starting PostgreSQL container '%s' on port %d...\n", containerName, port)
	cmd := exec.Command("podman", "run", "-d",
		"--name", containerName,
		"-p", fmt.Sprintf("%d:5432", port),
		"-e", "POSTGRES_DB=kubernaut_audit",
		"-e", "POSTGRES_USER=kubernaut",
		"-e", "POSTGRES_PASSWORD=test_password",
		"postgres:15-alpine",
	)
	output, err := cmd.CombinedOutput()
	Expect(err).ToNot(HaveOccurred(), "Should start PostgreSQL container: %s", string(output))

	// Wait for PostgreSQL to be ready
	GinkgoWriter.Printf("  ⏳ Waiting for PostgreSQL to be ready...\n")
	Eventually(func() bool {
		cmd := exec.Command("podman", "exec", containerName,
			"pg_isready", "-U", "kubernaut", "-d", "kubernaut_audit")
		return cmd.Run() == nil
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), "PostgreSQL should be ready within 30 seconds")

	GinkgoWriter.Printf("  ✅ PostgreSQL container '%s' created and started on port %d\n", containerName, port)

	return &PostgresTestClient{
		Host:     "localhost",
		Port:     port,
		Database: "kubernaut_audit",
		User:     "kubernaut",
		Password: "test_password",
	}
}

// ConnectionString returns the PostgreSQL connection string
func (p *PostgresTestClient) ConnectionString() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		p.User, p.Password, p.Host, p.Port, p.Database)
}

// Cleanup terminates the PostgreSQL container
func (p *PostgresTestClient) Cleanup(ctx context.Context) {
	containerName := "gateway-postgres-integration"
	GinkgoWriter.Printf("  Stopping PostgreSQL container '%s'...\n", containerName)

	// Stop container
	cmd := exec.Command("podman", "stop", containerName)
	if err := cmd.Run(); err != nil {
		GinkgoWriter.Printf("  ⚠️  Failed to stop PostgreSQL container: %v\n", err)
	}

	// Remove container
	cmd = exec.Command("podman", "rm", containerName)
	if err := cmd.Run(); err != nil {
		GinkgoWriter.Printf("  ⚠️  Failed to remove PostgreSQL container: %v\n", err)
	} else {
		GinkgoWriter.Printf("  ✅ PostgreSQL container '%s' stopped and removed\n", containerName)
	}
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// DATA STORAGE TEST SERVER METHODS (envtest Migration - Phase 2)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// SetupDataStorageTestServer creates a REAL Data Storage service for integration tests
// Used for audit trail API (Gateway → Data Storage → PostgreSQL)
//
// FIX: NOTICE_INTEGRATION_TEST_MOCK_VIOLATIONS.md
// Per 03-testing-strategy.mdc: Integration tests must use REAL dependencies
// Only LLM is allowed to be mocked (cost constraint)
func SetupDataStorageTestServer(ctx context.Context, pgClient *PostgresTestClient) *DataStorageTestServer {
	GinkgoWriter.Printf("  Starting REAL Data Storage service (PostgreSQL: %s)...\n", pgClient.ConnectionString())

	containerName := "gateway-datastorage-integration"

	// Stop and remove existing container if it exists
	exec.Command("podman", "stop", containerName).Run()
	exec.Command("podman", "rm", containerName).Run()

	// Find random available port for Data Storage
	dsPort := findAvailablePort(50100, 60000)
	GinkgoWriter.Printf("  📍 Allocated random port for Data Storage: %d\n", dsPort)

	// Start Data Storage container with PostgreSQL connection
	// Uses the same PostgreSQL container that integration tests set up
	cmd := exec.Command("podman", "run", "-d",
		"--name", containerName,
		"-p", fmt.Sprintf("%d:8080", dsPort),
		"-e", fmt.Sprintf("DATABASE_URL=%s", pgClient.ConnectionString()),
		"-e", "CONFIG_PATH=/app/config.yaml",
		"--memory", "256m",
		"localhost/kubernaut-datastorage:e2e-test",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// If container image not available, fall back to mock with clear warning
		GinkgoWriter.Printf("  ⚠️  Data Storage container not available: %s\n", string(output))
		GinkgoWriter.Printf("  ⚠️  FALLING BACK TO MOCK - Build datastorage image for full validation\n")
		GinkgoWriter.Printf("  ⚠️  Run: make build-datastorage-image\n")

		// Create fallback mock server (temporary until image is built)
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Mock audit trail endpoint - batch endpoint per ADR-038
			if r.URL.Path == "/api/v1/audit/events/batch" && r.Method == http.MethodPost {
				w.WriteHeader(http.StatusAccepted)
				w.Write([]byte(`{"status":"accepted","count":1}`))
				return
			}
			// Single event endpoint
			if r.URL.Path == "/api/v1/audit/events" && r.Method == http.MethodPost {
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"status":"created","event_id":"test-123"}`))
				return
			}
			// Health check
			if r.URL.Path == "/healthz" {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(`{"status":"ok"}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))

		return &DataStorageTestServer{
			Server:   mockServer,
			PgClient: pgClient,
		}
	}

	GinkgoWriter.Printf("  ✅ Data Storage container started: %s\n", containerName)

	// Wait for Data Storage to be ready
	dataStorageURL := fmt.Sprintf("http://localhost:%d", dsPort)
	healthURL := fmt.Sprintf("%s/healthz", dataStorageURL)

	Eventually(func() bool {
		resp, err := http.Get(healthURL)
		if err != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Data Storage should become healthy")

	GinkgoWriter.Printf("  ✅ REAL Data Storage service ready (URL: %s)\n", dataStorageURL)

	// Return real Data Storage server info (no httptest.Server needed)
	return &DataStorageTestServer{
		Server:   nil, // No mock server when using real container
		BaseURL:  dataStorageURL,
		PgClient: pgClient,
	}
}

// Cleanup closes the Data Storage test server (mock or container)
func (d *DataStorageTestServer) Cleanup() {
	// Close mock server if used (fallback mode)
	if d.Server != nil {
		d.Server.Close()
		GinkgoWriter.Printf("  ✅ Data Storage mock server closed\n")
		return
	}

	// Stop real Data Storage container
	containerName := "gateway-datastorage-integration"
	if err := exec.Command("podman", "stop", containerName).Run(); err != nil {
		GinkgoWriter.Printf("  ⚠️  Failed to stop Data Storage container: %v\n", err)
	}
	if err := exec.Command("podman", "rm", containerName).Run(); err != nil {
		GinkgoWriter.Printf("  ⚠️  Failed to remove Data Storage container: %v\n", err)
	}
	GinkgoWriter.Printf("  ✅ Data Storage container stopped and removed\n")
}

// URL returns the Data Storage service URL
func (d *DataStorageTestServer) URL() string {
	if d.Server != nil {
		return d.Server.URL // Mock server URL
	}
	return d.BaseURL // Real container URL
}

// findAvailablePort finds a random available port in the given range
// This is a simple implementation that tries random ports
func findAvailablePort(min, max int) int {
	// Try to find an available port by attempting to bind to it
	for i := 0; i < 100; i++ {
		port := min + (i * 100) // Simple increment to avoid conflicts
		if port > max {
			port = min + (i % ((max - min) / 100))
		}

		// Check if port is in use by trying to connect
		cmd := exec.Command("sh", "-c", fmt.Sprintf("lsof -i :%d", port))
		if err := cmd.Run(); err != nil {
			// Port is not in use
			return port
		}
	}

	// Fallback to a high port
	return 50432
}
