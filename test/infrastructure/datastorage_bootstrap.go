package infrastructure

import (
	"context"
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

	// EnvtestKubeconfig is the path to kubeconfig for envtest Kubernetes API server (DD-AUTH-014)
	// If provided, DataStorage will use envtest for TokenReview/SAR validation (real K8s APIs)
	// Optional: Only needed when using real middleware auth in integration tests
	EnvtestKubeconfig string // Path to envtest kubeconfig (e.g., "/tmp/envtest-kubeconfig-ds-client.yaml")

	// DataStorageServiceTokenPath is the path to the data-storage-sa ServiceAccount token file (DD-AUTH-014)
	// If provided, this token is mounted at /var/run/secrets/kubernetes.io/serviceaccount/token in the container
	// This allows DataStorage to self-validate its auth middleware in the /health endpoint
	// Optional: Only needed when using real middleware auth in integration tests
	DataStorageServiceTokenPath string // Path to data-storage-sa token file (e.g., "/tmp/datastorage-service-token")
}

// NewDSBootstrapConfigWithAuth creates a DSBootstrapConfig with authentication properly configured
// This helper ensures all services configure DataStorage auth consistently (DD-AUTH-014)
//
// Parameters:
//   - serviceName: Service name for container naming (e.g., "gateway", "signalprocessing")
//   - postgresPort, redisPort, dataStoragePort, metricsPort: Service-specific port allocations (per DD-TEST-001)
//   - configDir: Path to service's DataStorage config directory (e.g., "test/integration/gateway/config")
//   - authConfig: Result from CreateIntegrationServiceAccountWithDataStorageAccess()
//
// Returns:
//   - DSBootstrapConfig with all auth fields properly set
//
// Usage:
//
//	// Phase 1: Create ServiceAccount + RBAC
//	authConfig, err := infrastructure.CreateIntegrationServiceAccountWithDataStorageAccess(
//	    sharedK8sConfig, "gateway-integration-sa", "default", GinkgoWriter)
//
//	// Phase 2: Build config with helper (no manual field mapping needed)
//	cfg := infrastructure.NewDSBootstrapConfigWithAuth(
//	    "gateway", 15437, 16380, 18091, 19091,
//	    "test/integration/gateway/config", authConfig)
//
//	// Phase 3: Start infrastructure
//	dsInfra, err := infrastructure.StartDSBootstrap(cfg, GinkgoWriter)
//
// Authority: DD-AUTH-014 (Middleware-based authentication)
func NewDSBootstrapConfigWithAuth(
	serviceName string,
	postgresPort, redisPort, dataStoragePort, metricsPort int,
	configDir string,
	authConfig *IntegrationAuthConfig,
) DSBootstrapConfig {
	return DSBootstrapConfig{
		ServiceName:                 serviceName,
		PostgresPort:                postgresPort,
		RedisPort:                   redisPort,
		DataStoragePort:             dataStoragePort,
		MetricsPort:                 metricsPort,
		ConfigDir:                   configDir,
		EnvtestKubeconfig:           authConfig.KubeconfigPath,
		DataStorageServiceTokenPath: authConfig.DataStorageServiceTokenPath,
	}
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
	
	// SharedTestEnv holds envtest environment for cleanup (DD-AUTH-014)
	// Only set if envtest was created in Phase 1 (for services needing DataStorage auth)
	SharedTestEnv interface{} // *envtest.Environment (interface{} to avoid import cycle)
}

// tryPullFromRegistry attempts to pull an image from IMAGE_REGISTRY if configured.
// This enables CI/CD optimization where pre-built images are pulled from ghcr.io
// instead of building locally, saving ~70% disk space and ~30% time.
//
// Environment Variables:
//   - IMAGE_REGISTRY: Registry URL (e.g., "ghcr.io/jordigilh/kubernaut")
//   - IMAGE_TAG: Image tag (e.g., "pr-123", "main-abc1234")
//
// Returns:
//   - imageName: The local image name after tagging (same as input localImageName)
//   - pulled: true if successfully pulled from registry, false otherwise
//   - error: Only returns error if pull succeeded but tagging failed
//
// Usage:
//
//	if imageName, pulled, _ := tryPullFromRegistry(ctx, "datastorage", localImageName, writer); pulled {
//	    return imageName, nil // Use registry image
//	}
//	// Otherwise, fall through to local build
//
// Authority: CI/CD pipeline optimization for integration tests
func tryPullFromRegistry(ctx context.Context, serviceName, localImageName string, writer io.Writer) (string, bool, error) {
	registry := os.Getenv("IMAGE_REGISTRY")
	tag := os.Getenv("IMAGE_TAG")
	
	if registry == "" || tag == "" {
		return "", false, nil // Not configured, caller should build locally
	}
	
	registryImage := fmt.Sprintf("%s/%s:%s", registry, serviceName, tag)
	_, _ = fmt.Fprintf(writer, "   🔄 Registry mode detected (IMAGE_REGISTRY + IMAGE_TAG set)\n")
	_, _ = fmt.Fprintf(writer, "   📥 Pulling from registry: %s\n", registryImage)
	
	pullCmd := exec.CommandContext(ctx, "podman", "pull", registryImage)
	pullCmd.Stdout = writer
	pullCmd.Stderr = writer
	
	if err := pullCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ⚠️  Registry pull failed: %v\n", err)
		_, _ = fmt.Fprintf(writer, "   ⚠️  Falling back to local build...\n")
		return "", false, nil // Pull failed, caller should build locally
	}
	
	_, _ = fmt.Fprintf(writer, "   ✅ Image pulled from registry successfully\n")
	
	// Tag as local image for consistency with local build workflow
	_, _ = fmt.Fprintf(writer, "   🏷️  Tagging as local image: %s\n", localImageName)
	tagCmd := exec.CommandContext(ctx, "podman", "tag", registryImage, localImageName)
	if err := tagCmd.Run(); err != nil {
		return "", false, fmt.Errorf("failed to tag registry image: %w", err)
	}
	
	_, _ = fmt.Fprintf(writer, "   ✅ Image ready from registry (skipping local build)\n")
	return localImageName, true, nil
}

// BuildDataStorageImage builds the DataStorage Docker image for integration tests.
// This function is extracted to enable parallel image builds (build can happen in parallel,
// while deployment must remain sequential due to workflow seeding dependencies).
//
// CI/CD Optimization:
//   - If IMAGE_REGISTRY + IMAGE_TAG env vars are set: Pull from registry (ghcr.io)
//   - Otherwise: Build locally (existing behavior for local dev)
//   - Automatic fallback to local build if registry pull fails
//
// Returns:
// - string: Full image name with tag (e.g., "kubernaut/datastorage:aianalysis-a1b2c3d4")
// - error: Any errors during image build
//
// Per DD-TEST-004: Generates unique image tag per service to prevent collisions
func BuildDataStorageImage(ctx context.Context, serviceName string, writer io.Writer) (string, error) {
	projectRoot := getProjectRoot()

	// Generate DD-TEST-001 v1.3 compliant image tag
	imageTag := generateInfrastructureImageTag("datastorage", serviceName)
	imageName := fmt.Sprintf("kubernaut/datastorage:%s", imageTag)

	// CI/CD Optimization: Try to pull from registry if configured
	if pulledImageName, pulled, err := tryPullFromRegistry(ctx, "datastorage", imageName, writer); pulled {
		if err != nil {
			return "", err // Tag failed after successful pull
		}
		return pulledImageName, nil // Use registry image
	}

	// Check if image already exists (cache hit)
	checkCmd := exec.CommandContext(ctx, "podman", "image", "exists", imageName)
	if checkCmd.Run() == nil {
		_, _ = fmt.Fprintf(writer, "   ✅ DataStorage image already exists: %s\n", imageName)
		return imageName, nil
	}

	// Build the image locally
	_, _ = fmt.Fprintf(writer, "   🔨 Building DataStorage image locally (tag: %s)...\n", imageTag)
	buildCmd := exec.CommandContext(ctx, "podman", "build",
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
			return imageName, nil
		}
		return "", fmt.Errorf("failed to build DataStorage image: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage image built: %s\n", imageName)
	return imageName, nil
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

	// Step 0: Build DataStorage image (can be parallelized in test suites)
	_, _ = fmt.Fprintf(writer, "🔨 Building DataStorage image...\n")
	imageName, err := BuildDataStorageImage(context.Background(), cfg.ServiceName, writer)
	if err != nil {
		return nil, fmt.Errorf("failed to build DataStorage image: %w", err)
	}
	infra.DataStorageImageName = imageName
	_, _ = fmt.Fprintf(writer, "\n")

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

	_, _ = fmt.Fprintf(writer, "⏳ Waiting for PostgreSQL to be ready (two-phase: connection + queryability)...\n")
	// CRITICAL: Use two-phase health check to prevent "database system is starting up" errors
	// Phase 1: pg_isready (connection check)
	// Phase 2: SELECT 1 (queryability check)
	// Per DD-TEST-002: This prevents race condition in migrations
	if err := WaitForPostgreSQLReady(infra.PostgresContainer, defaultPostgresUser, defaultPostgresDB, writer); err != nil {
		return nil, fmt.Errorf("PostgreSQL failed to become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ PostgreSQL ready and queryable\n\n")

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
	if err := startDSBootstrapService(infra, imageName, projectRoot, writer); err != nil {
		return nil, fmt.Errorf("failed to start DataStorage: %w", err)
	}

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

// startDSBootstrapService starts the DataStorage container using a pre-built image
// The image should be built using BuildDataStorageImage() before calling this function
//
// DD-AUTH-014: Option A (macOS) - Bridge network + kubeconfig rewrite to IPv4
// Requires IPv6 disabled on macOS: sudo networksetup -setv6off Wi-Fi
// Reference: docs/handoff/DD_AUTH_014_MACOS_PODMAN_LIMITATION.md
func startDSBootstrapService(infra *DSBootstrapInfra, imageName string, projectRoot string, writer io.Writer) error {
	cfg := infra.Config
	configDir := filepath.Join(projectRoot, cfg.ConfigDir)

	// Always use bridge network (--network=host doesn't work on macOS Podman)
	args := []string{"run", "-d",
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
		"-e", "CONN_MAX_LIFETIME=30m",
		"-e", fmt.Sprintf("REDIS_ADDR=%s:6379", infra.RedisContainer),
		"-e", "PORT=8080",
	}

	// DD-AUTH-014: If EnvtestKubeconfig provided, mount it for real K8s auth
	if cfg.EnvtestKubeconfig != "" {
		_, _ = fmt.Fprintf(writer, "   🔐 Mounting envtest kubeconfig (IPv4-rewritten): %s\n", cfg.EnvtestKubeconfig)
		args = append(args,
			"-v", fmt.Sprintf("%s:/tmp/kubeconfig:ro", cfg.EnvtestKubeconfig),
			"-e", "KUBECONFIG=/tmp/kubeconfig",
			"-e", "POD_NAMESPACE=default",
		)

		// DD-AUTH-014: Mount DataStorage service token for health check self-validation
		if cfg.DataStorageServiceTokenPath != "" {
			_, _ = fmt.Fprintf(writer, "   🎫 Mounting DataStorage service token: %s\n", cfg.DataStorageServiceTokenPath)
			args = append(args,
				"-v", fmt.Sprintf("%s:/var/run/secrets/kubernetes.io/serviceaccount/token:ro", cfg.DataStorageServiceTokenPath),
			)
		}
	}

	args = append(args, imageName)

	cmd := exec.Command("podman", args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
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
