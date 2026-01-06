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

package infrastructure

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// AuthWebhookInfrastructure manages the test infrastructure for auth webhook integration tests
// Per DD-TEST-001: PostgreSQL:15442, Redis:16386, DataStorage:18099
type AuthWebhookInfrastructure struct {
	PostgresHost string
	PostgresPort string
	RedisAddr    string
	DataStorageURL string
	
	networkName string
	postgresContainer string
	redisContainer string
	datastorageContainer string
}

// NewAuthWebhookInfrastructure creates infrastructure manager with DD-TEST-001 ports
func NewAuthWebhookInfrastructure() *AuthWebhookInfrastructure {
	return &AuthWebhookInfrastructure{
		PostgresHost: "localhost",
		PostgresPort: "15442", // DD-TEST-001
		RedisAddr:    "localhost:16386", // DD-TEST-001
		DataStorageURL: "http://localhost:18099", // DD-TEST-001
		
		networkName: "authwebhook-test-network",
		postgresContainer: "authwebhook_postgres_test",
		redisContainer: "authwebhook_redis_test",
		datastorageContainer: "authwebhook_datastorage_test",
	}
}

// Setup starts all infrastructure containers and waits for readiness
// Pattern: Programmatic podman commands (DD-INTEGRATION-001 v2.0)
func (i *AuthWebhookInfrastructure) Setup() {
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("🔧 Setting up Auth Webhook Integration Infrastructure")
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	By("Creating podman network")
	i.createNetwork()

	By("Starting PostgreSQL container")
	i.startPostgreSQL()

	By("Starting Redis container")
	i.startRedis()

	By("Applying database migrations")
	i.applyMigrations()

	By("Starting Data Storage service")
	i.startDataStorage()

	By("Waiting for Data Storage to be ready")
	i.waitForDataStorage()

	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	GinkgoWriter.Println("✅ Auth Webhook Infrastructure Ready")
	GinkgoWriter.Printf("   • PostgreSQL: %s:%s\n", i.PostgresHost, i.PostgresPort)
	GinkgoWriter.Printf("   • Redis: %s\n", i.RedisAddr)
	GinkgoWriter.Printf("   • Data Storage: %s\n", i.DataStorageURL)
	GinkgoWriter.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

// Teardown stops all infrastructure containers
func (i *AuthWebhookInfrastructure) Teardown() {
	GinkgoWriter.Println("🧹 Tearing down Auth Webhook infrastructure...")

	// Stop containers
	_ = exec.Command("podman", "stop", i.datastorageContainer).Run()
	_ = exec.Command("podman", "stop", i.redisContainer).Run()
	_ = exec.Command("podman", "stop", i.postgresContainer).Run()

	// Remove containers
	_ = exec.Command("podman", "rm", i.datastorageContainer).Run()
	_ = exec.Command("podman", "rm", i.redisContainer).Run()
	_ = exec.Command("podman", "rm", i.postgresContainer).Run()

	// Remove network
	_ = exec.Command("podman", "network", "rm", i.networkName).Run()

	GinkgoWriter.Println("✅ Teardown complete")
}

func (i *AuthWebhookInfrastructure) createNetwork() {
	// Remove existing network if it exists
	_ = exec.Command("podman", "network", "rm", i.networkName).Run()

	// Create new network
	cmd := exec.Command("podman", "network", "create", i.networkName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to create network: %s\n", output)
		Fail(fmt.Sprintf("Network creation failed: %v", err))
	}

	GinkgoWriter.Printf("✅ Network created: %s\n", i.networkName)
}

func (i *AuthWebhookInfrastructure) startPostgreSQL() {
	// Cleanup existing container
	_ = exec.Command("podman", "rm", "-f", i.postgresContainer).Run()

	// Wait for container to be removed
	Eventually(func() bool {
		cmd := exec.Command("podman", "ps", "-a", "--filter", fmt.Sprintf("name=%s", i.postgresContainer), "--format", "{{.Names}}")
		output, _ := cmd.CombinedOutput()
		return !strings.Contains(string(output), i.postgresContainer)
	}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(), "Container should be removed")

	// Start PostgreSQL
	cmd := exec.Command("podman", "run", "-d",
		"--name", i.postgresContainer,
		"--network", i.networkName,
		"-p", fmt.Sprintf("%s:5432", i.PostgresPort), // DD-TEST-001: 15442
		"-e", "POSTGRES_DB=action_history",
		"-e", "POSTGRES_USER=slm_user",
		"-e", "POSTGRES_PASSWORD=test_password",
		"postgres:16-alpine",
		"-c", "max_connections=200")

	output, err := cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to start PostgreSQL: %s\n", output)
		Fail(fmt.Sprintf("PostgreSQL container failed to start: %v", err))
	}

	// Wait for PostgreSQL ready
	GinkgoWriter.Println("⏳ Waiting for PostgreSQL to be ready...")
	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", i.postgresContainer, "pg_isready", "-U", "slm_user")
		return testCmd.Run()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready")

	GinkgoWriter.Println("✅ PostgreSQL started")
}

func (i *AuthWebhookInfrastructure) startRedis() {
	// Cleanup existing container
	_ = exec.Command("podman", "rm", "-f", i.redisContainer).Run()

	// Wait for container to be removed
	Eventually(func() bool {
		cmd := exec.Command("podman", "ps", "-a", "--filter", fmt.Sprintf("name=%s", i.redisContainer), "--format", "{{.Names}}")
		output, _ := cmd.CombinedOutput()
		return !strings.Contains(string(output), i.redisContainer)
	}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(), "Container should be removed")

	// Start Redis
	hostPort := strings.Split(i.RedisAddr, ":")[1]
	cmd := exec.Command("podman", "run", "-d",
		"--name", i.redisContainer,
		"--network", i.networkName,
		"-p", fmt.Sprintf("%s:6379", hostPort), // DD-TEST-001: 16386
		"redis:7-alpine")

	output, err := cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to start Redis: %s\n", output)
		Fail(fmt.Sprintf("Redis container failed to start: %v", err))
	}

	// Wait for Redis ready
	GinkgoWriter.Println("⏳ Waiting for Redis to be ready...")
	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", i.redisContainer, "redis-cli", "ping")
		return testCmd.Run()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "Redis should be ready")

	GinkgoWriter.Println("✅ Redis started")
}

func (i *AuthWebhookInfrastructure) applyMigrations() {
	// Connect to PostgreSQL
	connStr := fmt.Sprintf("host=%s port=%s user=slm_user password=test_password dbname=action_history sslmode=disable",
		i.PostgresHost, i.PostgresPort)

	db, err := sql.Open("pgx", connStr)
	Expect(err).NotTo(HaveOccurred(), "Should connect to PostgreSQL")
	defer db.Close()

	// Test connection
	Eventually(func() error {
		return db.Ping()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL connection should work")

	// Apply minimal schema for audit events
	// Per ADR-034: Unified audit table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS audit_events (
			id BIGSERIAL PRIMARY KEY,
			event_timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			event_category VARCHAR(50) NOT NULL,
			event_type VARCHAR(100) NOT NULL,
			event_outcome VARCHAR(20) NOT NULL,
			correlation_id VARCHAR(255),
			actor_id VARCHAR(255),
			event_data JSONB,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_audit_correlation ON audit_events(correlation_id);
		CREATE INDEX IF NOT EXISTS idx_audit_category ON audit_events(event_category);
		CREATE INDEX IF NOT EXISTS idx_audit_type ON audit_events(event_type);
		CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_events(event_timestamp);
	`)
	Expect(err).NotTo(HaveOccurred(), "Should create audit_events table")

	GinkgoWriter.Println("✅ Database migrations applied")
}

func (i *AuthWebhookInfrastructure) startDataStorage() {
	// Cleanup existing container
	_ = exec.Command("podman", "rm", "-f", i.datastorageContainer).Run()

	// Wait for container to be removed
	Eventually(func() bool {
		cmd := exec.Command("podman", "ps", "-a", "--filter", fmt.Sprintf("name=%s", i.datastorageContainer), "--format", "{{.Names}}")
		output, _ := cmd.CombinedOutput()
		return !strings.Contains(string(output), i.datastorageContainer)
	}, 5*time.Second, 500*time.Millisecond).Should(BeTrue(), "Container should be removed")

	// Find the repository root (3 levels up from test/infrastructure/)
	repoRoot := os.Getenv("GITHUB_WORKSPACE")
	if repoRoot == "" {
		// Fallback for local execution
		cwd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		// Assume we're in test/infrastructure/ or test/integration/authwebhook/
		if strings.Contains(cwd, "test/infrastructure") {
			repoRoot = strings.Split(cwd, "/test/infrastructure")[0]
		} else if strings.Contains(cwd, "test/integration") {
			repoRoot = strings.Split(cwd, "/test/integration")[0]
		} else {
			// Assume we're already in the repo root
			repoRoot = cwd
		}
	}

	// Start Data Storage service
	cmd := exec.Command("podman", "run", "-d",
		"--name", i.datastorageContainer,
		"--network", i.networkName,
		"-p", "18099:8080", // DD-TEST-001
		"-e", "POSTGRES_HOST="+i.postgresContainer,
		"-e", "POSTGRES_PORT=5432",
		"-e", "POSTGRES_USER=slm_user",
		"-e", "POSTGRES_PASSWORD=test_password",
		"-e", "POSTGRES_DB=action_history",
		"-e", "REDIS_ADDR="+i.redisContainer+":6379",
		"-e", "PORT=8080",
		"-e", "DLQ_ENABLED=true",
		"-e", "DLQ_MAX_RETRIES=3",
		"-v", fmt.Sprintf("%s/cmd/datastorage:/app", repoRoot),
		"golang:1.22-alpine",
		"sh", "-c", "cd /app && go run main.go")

	output, err := cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("❌ Failed to start Data Storage: %s\n", output)
		Fail(fmt.Sprintf("Data Storage container failed to start: %v", err))
	}

	GinkgoWriter.Println("✅ Data Storage service started")
}

func (i *AuthWebhookInfrastructure) waitForDataStorage() {
	GinkgoWriter.Println("⏳ Waiting for Data Storage API to be ready...")

	// Wait for health endpoint
	Eventually(func() error {
		cmd := exec.Command("curl", "-sf", i.DataStorageURL+"/health")
		return cmd.Run()
	}, 60*time.Second, 2*time.Second).Should(Succeed(), "Data Storage should be ready")

	GinkgoWriter.Println("✅ Data Storage API is ready")
}

// GetDataStorageURL returns the Data Storage service URL
func (i *AuthWebhookInfrastructure) GetDataStorageURL() string {
	return i.DataStorageURL
}

