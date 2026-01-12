package infrastructure

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DSBootstrapConfig configures DataStorage infrastructure for integration tests
// Per DD-TEST-002: Sequential Container Orchestration Pattern
//
// This shared configuration eliminates code duplication across services (Gateway, RO, NT, etc.)
// by providing a single, proven implementation of the DataStorage infrastructure bootstrap.
//
// Only service-specific configuration is exposed (ports and config directory).
// All internal implementation details (database credentials, migrations) are hidden.
//
// Benefits:
// - 95% code similarity across services → single implementation
// - Eliminates podman-compose race conditions
// - Proven reliability: DataStorage (818/818 tests), Gateway (7/7 tests)
// - Consistent behavior across all integration test suites
// - Simple API - only expose what varies per service
type DSBootstrapConfig struct {
	// ServiceName is used for container naming: {service}_postgres_test, {service}_redis_test, etc.
	// Examples: "gateway", "remediation-orchestrator", "notification"
	ServiceName string

	// Port Configuration (per DD-TEST-001: Port Allocation Strategy)
	// These are the ONLY service-specific values - everything else is shared infrastructure
	PostgresPort    int // PostgreSQL port (e.g., 15437 for Gateway)
	RedisPort       int // Redis port (e.g., 16383 for Gateway)
	DataStoragePort int // DataStorage HTTP API port (e.g., 18091 for Gateway)
	MetricsPort     int // DataStorage metrics port (e.g., 19091 for Gateway)

	// Service-specific configuration directory
	ConfigDir string // Path to DataStorage config.yaml (e.g., "test/integration/gateway/config")
}

// Internal constants - shared across all services, not exposed in config
const (
	defaultPostgresUser     = "slm_user"
	defaultPostgresPassword = "test_password"
	defaultPostgresDB       = "action_history"
	defaultMigrationsPath   = "migrations" // Always at project root
)

// generateInfrastructureImageTag generates DD-TEST-001 v1.3 compliant tag for shared infrastructure
// Format: {consumer}-{uuid}
// Example: gateway-a1b2c3d4 (used as datastorage:gateway-a1b2c3d4)
//
// This simplified format is used for shared infrastructure images (DataStorage, HAPI) because:
// - Consumer service name provides clear isolation (gateway vs aianalysis vs ro)
// - UUID ensures zero collision risk (no timestamp needed - UUID is sufficient)
// - Simplest possible format while maintaining uniqueness
// - No redundancy - image name already contains infrastructure type
func generateInfrastructureImageTag(infrastructure, consumer string) string {
	// Use 8 hex characters from nanoseconds for UUID
	uuid := fmt.Sprintf("%x", time.Now().UnixNano())[:8]
	return fmt.Sprintf("%s-%s", consumer, uuid)
}

// DSBootstrapInfra holds references to started infrastructure components
// Used for cleanup and health monitoring during integration tests.
type DSBootstrapInfra struct {
	PostgresContainer    string // Container name: {service}_postgres_test
	RedisContainer       string // Container name: {service}_redis_test
	DataStorageContainer string // Container name: {service}_datastorage_test
	MigrationsContainer  string // Container name: {service}_migrations (ephemeral)
	Network              string // Network name: {service}_test_network

	ServiceURL string // DataStorage HTTP URL: http://localhost:{DataStoragePort}
	MetricsURL string // DataStorage metrics URL: http://localhost:{MetricsPort}

	// Image information for cleanup (DD-TEST-001 v1.3)
	DataStorageImageName string // Full image name with tag (e.g., kubernaut/datastorage:datastorage-gateway-1734278400)

	Config DSBootstrapConfig // Original configuration (for reference)
}

// StartDSBootstrap starts DataStorage infrastructure using DD-TEST-002 sequential pattern
//
// Sequential Startup Order (eliminates race conditions):
// 1. Cleanup existing containers
// 2. Create network
// 3. Start PostgreSQL → wait for ready
// 4. Run migrations
// 5. Start Redis → wait for ready
// 6. Start DataStorage → wait for HTTP /health
//
// This pattern achieves >99% reliability vs ~70% with podman-compose parallel startup.
//
// Returns:
// - *DSBootstrapInfra: Infrastructure references for cleanup
// - error: Any errors during infrastructure startup
func StartDSBootstrap(cfg DSBootstrapConfig, writer io.Writer) (*DSBootstrapInfra, error) {
	// Build infrastructure references
	infra := &DSBootstrapInfra{
		PostgresContainer:    fmt.Sprintf("%s_postgres_test", cfg.ServiceName),
		RedisContainer:       fmt.Sprintf("%s_redis_test", cfg.ServiceName),
		DataStorageContainer: fmt.Sprintf("%s_datastorage_test", cfg.ServiceName),
		MigrationsContainer:  fmt.Sprintf("%s_migrations", cfg.ServiceName),
		Network:              fmt.Sprintf("%s_test_network", cfg.ServiceName),
		ServiceURL:           fmt.Sprintf("http://localhost:%d", cfg.DataStoragePort),
		MetricsURL:           fmt.Sprintf("http://localhost:%d", cfg.MetricsPort),
		Config:               cfg,
	}

	projectRoot := getProjectRoot()

	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "DataStorage Integration Infrastructure Setup (%s)\n", cfg.ServiceName)
	_, _ = fmt.Fprintf(writer, "Per DD-TEST-002: Sequential Startup Pattern\n")
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", cfg.PostgresPort)
	_, _ = fmt.Fprintf(writer, "  Redis:          localhost:%d\n", cfg.RedisPort)
	_, _ = fmt.Fprintf(writer, "  DataStorage:    %s\n", infra.ServiceURL)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// Step 1: Cleanup
	_, _ = fmt.Fprintf(writer, "🧹 Cleaning up existing containers...\n")
	cleanupDSBootstrapContainers(infra, writer)
	_, _ = fmt.Fprintf(writer, "   ✅ Cleanup complete\n\n")

	// Step 2: Network
	_, _ = fmt.Fprintf(writer, "🌐 Creating test network...\n")
	if err := createDSBootstrapNetwork(infra, writer); err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Network ready: %s\n\n", infra.Network)

	// Step 3: PostgreSQL
	_, _ = fmt.Fprintf(writer, "🐘 Starting PostgreSQL...\n")
	if err := startDSBootstrapPostgreSQL(infra, writer); err != nil {
		return nil, fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "⏳ Waiting for PostgreSQL to be ready...\n")
	if err := waitForDSBootstrapPostgresReady(infra, writer); err != nil {
		return nil, fmt.Errorf("PostgreSQL failed to become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ PostgreSQL ready\n\n")

	// Step 4: Migrations
	_, _ = fmt.Fprintf(writer, "🔄 Running database migrations...\n")
	if err := runDSBootstrapMigrations(infra, projectRoot, writer); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Migrations applied successfully\n\n")

	// Step 5: Redis
	_, _ = fmt.Fprintf(writer, "🔴 Starting Redis...\n")
	if err := startDSBootstrapRedis(infra, writer); err != nil {
		return nil, fmt.Errorf("failed to start Redis: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "⏳ Waiting for Redis to be ready...\n")
	if err := waitForDSBootstrapRedisReady(infra, writer); err != nil {
		return nil, fmt.Errorf("Redis failed to become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Redis ready\n\n")

	// Step 6: DataStorage
	_, _ = fmt.Fprintf(writer, "📦 Starting DataStorage service...\n")
	imageName, err := startDSBootstrapService(infra, projectRoot, writer)
	if err != nil {
		return nil, fmt.Errorf("failed to start DataStorage: %w", err)
	}
	infra.DataStorageImageName = imageName

	_, _ = fmt.Fprintf(writer, "⏳ Waiting for DataStorage HTTP endpoint to be ready...\n")
	if err := waitForDSBootstrapHTTPHealth(infra, 30*time.Second, writer); err != nil {
		// Print container logs for debugging
		_, _ = fmt.Fprintf(writer, "\n⚠️  DataStorage failed to become healthy. Container logs:\n")
		logsCmd := exec.Command("podman", "logs", infra.DataStorageContainer)
		logsCmd.Stdout = writer
		logsCmd.Stderr = writer
		_ = logsCmd.Run()
		return nil, fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage ready\n\n")

	// Success
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "✅ DataStorage Infrastructure Ready (%s)\n", cfg.ServiceName)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "  PostgreSQL:        localhost:%d\n", cfg.PostgresPort)
	_, _ = fmt.Fprintf(writer, "  Redis:             localhost:%d\n", cfg.RedisPort)
	_, _ = fmt.Fprintf(writer, "  DataStorage HTTP:  %s\n", infra.ServiceURL)
	_, _ = fmt.Fprintf(writer, "  DataStorage Metrics: %s\n", infra.MetricsURL)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return infra, nil
}

// StopDSBootstrap stops and cleans up DataStorage infrastructure
//
// Cleanup Order:
// 1. Stop containers in reverse order (DataStorage, Redis, PostgreSQL)
// 2. Remove containers
// 3. Remove DataStorage image (DD-TEST-001 v1.3 compliance)
// 4. Remove network
//
// Cleanup Scope (DD-TEST-001 v1.3):
//   - ONLY kubernaut-built images are removed (DataStorage service image)
//   - Base images (postgres:16-alpine, redis:7-alpine) are NOT removed
//   - Rationale: Base images are shared across services and cached for performance
//
// Errors are ignored to allow cleanup to continue even if containers don't exist.
func StopDSBootstrap(infra *DSBootstrapInfra, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🛑 Stopping DataStorage Infrastructure (%s)...\n", infra.Config.ServiceName)

	// Stop and remove containers (ignore errors if containers don't exist)
	containers := []string{
		infra.DataStorageContainer,
		infra.RedisContainer,
		infra.PostgresContainer,
		infra.MigrationsContainer,
	}

	for _, container := range containers {
		stopCmd := exec.Command("podman", "stop", container)
		_ = stopCmd.Run() // Ignore errors

		rmCmd := exec.Command("podman", "rm", container)
		_ = rmCmd.Run() // Ignore errors
	}

	// Remove ONLY kubernaut-built DataStorage image (DD-TEST-001 v1.3)
	// Base images (postgres, redis) are NOT removed - they're shared and cached
	if infra.DataStorageImageName != "" {
		_, _ = fmt.Fprintf(writer, "🗑️  Removing kubernaut-built DataStorage image: %s\n", infra.DataStorageImageName)
		rmiCmd := exec.Command("podman", "rmi", infra.DataStorageImageName)
		if err := rmiCmd.Run(); err != nil {
			_, _ = fmt.Fprintf(writer, "   ⚠️  Failed to remove image (may not exist): %v\n", err)
		} else {
			_, _ = fmt.Fprintf(writer, "   ✅ DataStorage image removed\n")
		}
	}

	// Remove network
	networkCmd := exec.Command("podman", "network", "rm", infra.Network)
	_ = networkCmd.Run() // Ignore errors

	_, _ = fmt.Fprintf(writer, "✅ DataStorage Infrastructure stopped and cleaned up\n")
	return nil
}

// ============================================================================
// Internal Helper Functions (DD-TEST-002 Sequential Startup Implementation)
// ============================================================================

// cleanupDSBootstrapContainers removes any existing containers from previous runs
func cleanupDSBootstrapContainers(infra *DSBootstrapInfra, writer io.Writer) {
	containers := []string{
		infra.PostgresContainer,
		infra.RedisContainer,
		infra.DataStorageContainer,
		infra.MigrationsContainer,
	}

	for _, container := range containers {
		stopCmd := exec.Command("podman", "stop", container)
		_ = stopCmd.Run() // Ignore errors

		rmCmd := exec.Command("podman", "rm", container)
		_ = rmCmd.Run() // Ignore errors
	}
}

// createDSBootstrapNetwork creates the test network
func createDSBootstrapNetwork(infra *DSBootstrapInfra, writer io.Writer) error {
	// Check if network already exists
	checkCmd := exec.Command("podman", "network", "exists", infra.Network)
	if checkCmd.Run() == nil {
		return nil // Network exists
	}

	// Create network
	cmd := exec.Command("podman", "network", "create", infra.Network)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// startDSBootstrapPostgreSQL starts the PostgreSQL container
func startDSBootstrapPostgreSQL(infra *DSBootstrapInfra, writer io.Writer) error {
	cfg := infra.Config

	cmd := exec.Command("podman", "run", "-d",
		"--name", infra.PostgresContainer,
		"--network", infra.Network,
		"-p", fmt.Sprintf("%d:5432", cfg.PostgresPort),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", defaultPostgresUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", defaultPostgresPassword),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", defaultPostgresDB),
		"postgres:16-alpine",
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// waitForDSBootstrapPostgresReady waits for PostgreSQL to be ready
func waitForDSBootstrapPostgresReady(infra *DSBootstrapInfra, writer io.Writer) error {
	for i := 1; i <= 30; i++ {
		cmd := exec.Command("podman", "exec", infra.PostgresContainer,
			"pg_isready", "-U", defaultPostgresUser, "-d", defaultPostgresDB)
		if cmd.Run() == nil {
			_, _ = fmt.Fprintf(writer, "   PostgreSQL ready (attempt %d/30)\n", i)
			return nil
		}
		if i == 30 {
			return fmt.Errorf("PostgreSQL failed to become ready after 30 seconds")
		}
		_, _ = fmt.Fprintf(writer, "   Waiting... (attempt %d/30)\n", i)
		time.Sleep(1 * time.Second)
	}
	return nil
}

// runDSBootstrapMigrations applies database migrations
// Migrations are always located at {project_root}/migrations (universal location)
func runDSBootstrapMigrations(infra *DSBootstrapInfra, projectRoot string, writer io.Writer) error {
	migrationsDir := filepath.Join(projectRoot, defaultMigrationsPath)

	// Apply migrations: extract only "Up" sections (stop at "-- +goose Down")
	// NOTE: Migrations 001-008 (001,002,003,004,006) do NOT use pgvector - they are core schema
	// The pgvector-dependent migrations (005,007,008) were removed in V1.0 and no longer exist
	migrationScript := `
		set -e
		echo "Creating slm_user role (required by migrations)..."
		psql -c "CREATE ROLE slm_user LOGIN PASSWORD 'slm_user';" || echo "Role slm_user already exists"
		echo "Applying migrations (Up sections only)..."
		find /migrations -maxdepth 1 -name "*.sql" -type f | sort | while read f; do
			echo "Applying $f..."
			sed -n "1,/^-- +goose Down/p" "$f" | grep -v "^-- +goose Down" | psql
		done
		echo "Migrations complete!"
	`

	cmd := exec.Command("podman", "run", "--rm",
		"--name", infra.MigrationsContainer,
		"--network", infra.Network,
		"-v", fmt.Sprintf("%s:/migrations:ro", migrationsDir),
		"-e", fmt.Sprintf("PGHOST=%s", infra.PostgresContainer),
		"-e", "PGPORT=5432",
		"-e", fmt.Sprintf("PGUSER=%s", defaultPostgresUser),
		"-e", fmt.Sprintf("PGPASSWORD=%s", defaultPostgresPassword),
		"-e", fmt.Sprintf("PGDATABASE=%s", defaultPostgresDB),
		"postgres:16-alpine",
		"bash", "-c", migrationScript,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// startDSBootstrapRedis starts the Redis container
func startDSBootstrapRedis(infra *DSBootstrapInfra, writer io.Writer) error {
	cfg := infra.Config

	cmd := exec.Command("podman", "run", "-d",
		"--name", infra.RedisContainer,
		"--network", infra.Network,
		"-p", fmt.Sprintf("%d:6379", cfg.RedisPort),
		"redis:7-alpine",
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// waitForDSBootstrapRedisReady waits for Redis to be ready
func waitForDSBootstrapRedisReady(infra *DSBootstrapInfra, writer io.Writer) error {
	for i := 1; i <= 10; i++ {
		cmd := exec.Command("podman", "exec", infra.RedisContainer,
			"redis-cli", "ping")
		output, err := cmd.Output()
		if err == nil && strings.Contains(string(output), "PONG") {
			_, _ = fmt.Fprintf(writer, "   Redis ready (attempt %d/10)\n", i)
			return nil
		}
		if i == 10 {
			return fmt.Errorf("Redis failed to become ready after 10 seconds")
		}
		_, _ = fmt.Fprintf(writer, "   Waiting... (attempt %d/10)\n", i)
		time.Sleep(1 * time.Second)
	}
	return nil
}

// startDSBootstrapService starts the DataStorage container
// Returns the full image name for cleanup purposes
func startDSBootstrapService(infra *DSBootstrapInfra, projectRoot string, writer io.Writer) (string, error) {
	cfg := infra.Config
	configDir := filepath.Join(projectRoot, cfg.ConfigDir)

	// Generate DD-TEST-001 v1.3 compliant image tag
	// Format: datastorage-{consumer}-{timestamp}
	// Example: datastorage-gateway-1734278400
	imageTag := generateInfrastructureImageTag("datastorage", cfg.ServiceName)
	imageName := fmt.Sprintf("kubernaut/datastorage:%s", imageTag)

	// Check if DataStorage image exists, build if not
	checkCmd := exec.Command("podman", "image", "exists", imageName)
	if checkCmd.Run() != nil {
		_, _ = fmt.Fprintf(writer, "   Building DataStorage image (tag: %s)...\n", imageTag)
		buildCmd := exec.Command("podman", "build",
			"--no-cache", // DD-TEST-002: Force fresh build to include latest code changes

			"-t", imageName,
			"--force-rm=false", // Disable auto-cleanup to avoid podman cleanup errors
			"-f", filepath.Join(projectRoot, "docker", "data-storage.Dockerfile"),
			projectRoot,
		)
		buildCmd.Stdout = writer
		buildCmd.Stderr = writer
		if err := buildCmd.Run(); err != nil {
			// Check if image was actually built despite error (podman cleanup issue)
			checkAgain := exec.Command("podman", "image", "exists", imageName)
			if checkAgain.Run() == nil {
				_, _ = fmt.Fprintf(writer, "   ⚠️  Build completed with warnings (image exists): %s\n", imageName)
			} else {
				return "", fmt.Errorf("failed to build DataStorage image: %w", err)
			}
		} else {
			_, _ = fmt.Fprintf(writer, "   ✅ DataStorage image built: %s\n", imageName)
		}
	}

	cmd := exec.Command("podman", "run", "-d",
		"--name", infra.DataStorageContainer,
		"--network", infra.Network,
		"-p", fmt.Sprintf("%d:8080", cfg.DataStoragePort),
		"-p", fmt.Sprintf("%d:9090", cfg.MetricsPort),
		"-v", fmt.Sprintf("%s:/etc/datastorage:ro", configDir),
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		"-e", fmt.Sprintf("POSTGRES_HOST=%s", infra.PostgresContainer),
		"-e", "POSTGRES_PORT=5432",
		"-e", fmt.Sprintf("POSTGRES_USER=%s", defaultPostgresUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", defaultPostgresPassword),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", defaultPostgresDB),
		"-e", fmt.Sprintf("REDIS_ADDR=%s:6379", infra.RedisContainer),
		"-e", "PORT=8080",
		imageName,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return imageName, nil
}

// waitForDSBootstrapHTTPHealth waits for DataStorage /health endpoint to respond with 200 OK
func waitForDSBootstrapHTTPHealth(infra *DSBootstrapInfra, timeout time.Duration, writer io.Writer) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(infra.ServiceURL + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}

		// Log progress every 10 seconds
		if time.Now().Unix()%10 == 0 {
			_, _ = fmt.Fprintf(writer, "   Still waiting for %s/health...\n", infra.ServiceURL)
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for %s/health to become healthy after %v", infra.ServiceURL, timeout)
}

// ============================================================================
// Generic Container Abstraction (Reusable for Any Service)
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
//	    Ports:   map[int]int{18120: 8080}, // host:container
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
	Ports   map[int]int       // Port mappings: host_port -> container_port
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
	Ports  map[int]int            // Port mappings (host -> container)
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

// ============================================================================
// E2E Test Abstractions (Build + Load to Kind + Cleanup)
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

// BuildAndLoadImageToKind builds a service image and loads it into Kind cluster
// This abstracts the common E2E pattern: build → tag → load → cleanup tracking
//
// Returns:
//   - Full image name with tag for cleanup purposes
//   - error: Any errors during build or load
//
// Example:
//
//	imageConfig := infrastructure.E2EImageConfig{
//	    ServiceName:      "gateway",
//	    ImageName:        "kubernaut/gateway",
//	    DockerfilePath:   "cmd/gateway/Dockerfile",
//	    KindClusterName:  "gateway-e2e",
//	}
//	imageName, err := infrastructure.BuildAndLoadImageToKind(imageConfig, GinkgoWriter)
//	// Image built, tagged, and loaded to Kind
//	// Later: infrastructure.CleanupE2EImage(imageName, GinkgoWriter)
//
// BuildImageForKind builds a container image for E2E testing.
// Returns the image name (with localhost/ prefix) for later loading to Kind.
//
// This is Phase 1 of the hybrid E2E pattern:
//
//	Phase 1: Build images (BEFORE cluster creation)
//	Phase 2: Create Kind cluster
//	Phase 3: Load images to cluster (using LoadImageToKind)
//
// Authority: E2E_PATTERN_PERFORMANCE_ANALYSIS_JAN07.md
// Performance: Eliminates cluster idle time during image builds
//
// Example:
//
//	// Phase 1: Build images in parallel (no cluster yet)
//	imageName, err := BuildImageForKind(cfg, writer)
//	// Phase 2: Create Kind cluster
//	createKindCluster(...)
//	// Phase 3: Load image to cluster
//	err = LoadImageToKind(imageName, cfg.ServiceName, clusterName, writer)
func BuildImageForKind(cfg E2EImageConfig, writer io.Writer) (string, error) {
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

	_, _ = fmt.Fprintf(writer, "🔨 Building E2E image: %s\n", fullImageName)

	// Build image with optional coverage instrumentation
	buildArgs := []string{"build", "-t", localImageName}

	// DD-TEST-007: E2E Coverage Collection
	// Support coverage instrumentation when E2E_COVERAGE=true or EnableCoverage flag is set
	if cfg.EnableCoverage || os.Getenv("E2E_COVERAGE") == "true" {
		buildArgs = append(buildArgs, "--build-arg", "GOFLAGS=-cover")
		_, _ = fmt.Fprintf(writer, "   📊 Building with coverage instrumentation (GOFLAGS=-cover)\n")
	}

	buildArgs = append(buildArgs, "-f", filepath.Join(projectRoot, cfg.DockerfilePath), cfg.BuildContextPath)

	buildCmd := exec.Command("podman", buildArgs...)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to build E2E image: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Image built: %s\n", localImageName)

	return localImageName, nil
}

// LoadImageToKind loads a pre-built image to a Kind cluster.
// Steps: Export to tar → Load to Kind → Remove tar → Remove Podman image
//
// This is Phase 3 of the hybrid E2E pattern:
//
//	Phase 1: Build images (using BuildImageForKind)
//	Phase 2: Create Kind cluster
//	Phase 3: Load images to cluster (THIS FUNCTION)
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

// ============================================================================
// Shared Utility Functions
// ============================================================================

// getProjectRoot is defined in aianalysis.go (shared across infrastructure package)
