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

// SetupDataStorageTestServer creates a Data Storage service for integration tests
// Used for audit trail API (Gateway → Data Storage → PostgreSQL)
func SetupDataStorageTestServer(ctx context.Context, pgClient *PostgresTestClient) *DataStorageTestServer {
	// TODO: Initialize Data Storage service with PostgreSQL
	// This will be implemented when Data Storage service is ready
	// For now, create a mock server that accepts audit requests

	GinkgoWriter.Printf("  Creating mock Data Storage server (PostgreSQL: %s)...\n", pgClient.ConnectionString())

	// Create a simple mock server for audit trail endpoints
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock audit trail endpoint
		if r.URL.Path == "/api/v1/audit/events" {
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"status":"created","event_id":"test-123"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))

	GinkgoWriter.Printf("  ✅ Mock Data Storage server created (URL: %s)\n", mockServer.URL)

	return &DataStorageTestServer{
		Server:   mockServer,
		PgClient: pgClient,
	}
}

// Cleanup closes the Data Storage test server
func (d *DataStorageTestServer) Cleanup() {
	if d.Server != nil {
		d.Server.Close()
		GinkgoWriter.Printf("  ✅ Data Storage server closed\n")
	}
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
