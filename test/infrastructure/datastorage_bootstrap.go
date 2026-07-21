package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/jordigilh/kubernaut/pkg/cert"
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
	HealthPort      int // DataStorage health probe port (Issue #753: defaults to DataStoragePort+1000)

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
// This simplified format is used for shared infrastructure images (DataStorage, KA) because:
// - Consumer service name provides clear isolation (gateway vs aianalysis vs ro)
// - UUID ensures zero collision risk (no timestamp needed - UUID is sufficient)
// - Simplest possible format while maintaining uniqueness
// - No redundancy - image name already contains infrastructure type
func generateInfrastructureImageTag(consumer string) string {
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
	HealthURL  string // DataStorage health URL: http://localhost:{HealthPort} (Issue #753)
	MetricsURL string // DataStorage metrics URL: http://localhost:{MetricsPort}

	// Image information for cleanup (DD-TEST-001 v1.3)
	DataStorageImageName string // Full image name with tag (e.g., kubernaut/datastorage:datastorage-gateway-1734278400)

	Config DSBootstrapConfig // Original configuration (for reference)

	// SigningCertDir holds the temp directory with tls.crt/tls.key for audit export signing.
	// Mounted into the container at /etc/certs (AU-9).
	SigningCertDir string

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
func tryPullFromRegistry(ctx context.Context, serviceName string, writer io.Writer) (string, bool, error) {
	registry := os.Getenv("IMAGE_REGISTRY")
	tag := os.Getenv("IMAGE_TAG")

	if registry == "" || tag == "" {
		return "", false, nil // Not configured, caller should build locally
	}

	registryImage := fmt.Sprintf("%s/%s:%s", registry, serviceName, tag)
	_, _ = fmt.Fprintf(writer, "   🔄 Registry mode detected (IMAGE_REGISTRY + IMAGE_TAG set)\n")

	exists, err := VerifyImageExistsInRegistry(ctx, registryImage, writer)
	if err != nil || !exists {
		_, _ = fmt.Fprintf(writer, "   ⚠️  Registry verification failed: %v\n", err)
		_, _ = fmt.Fprintf(writer, "   ⚠️  Falling back to local build...\n")
		return "", false, nil
	}

	_, _ = fmt.Fprintf(writer, "   📥 Pulling image from registry: %s\n", registryImage)
	pullCmd := exec.CommandContext(ctx, "podman", "pull", registryImage)
	pullCmd.Stdout = writer
	pullCmd.Stderr = writer
	if pullErr := pullCmd.Run(); pullErr != nil {
		_, _ = fmt.Fprintf(writer, "   ⚠️  Pull failed: %v, falling back to local build...\n", pullErr)
		return "", false, nil
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Image pulled from registry (skipping local build)\n")

	return registryImage, true, nil
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
	imageTag := generateInfrastructureImageTag(serviceName)
	imageName := fmt.Sprintf("kubernaut/datastorage:%s", imageTag)

	// Step -1: Use a CI-loaded artifact if one was already podman-loaded for
	// this service under the agreed-upon fixed tag (artifact-based CI mode,
	// no registry involved). Mirrors StartGenericContainer's equivalent
	// check (container_management.go) — without it, every suite that calls
	// StartDSBootstrap unconditionally re-runs a --no-cache local build even
	// though CI already built and loaded this exact image.
	if artifactTag := os.Getenv("KUBERNAUT_CI_ARTIFACT_TAG"); artifactTag != "" {
		prebuiltImage := fmt.Sprintf("localhost/datastorage:%s", artifactTag)
		if checkCmd := exec.CommandContext(ctx, "podman", "image", "exists", prebuiltImage); checkCmd.Run() == nil {
			_, _ = fmt.Fprintf(writer, "   ✅ Using CI-prebuilt artifact: %s\n", prebuiltImage)
			return prebuiltImage, nil
		}
	}

	// DEBUG: Show environment variable status
	registry := os.Getenv("IMAGE_REGISTRY")
	tag := os.Getenv("IMAGE_TAG")
	_, _ = fmt.Fprintf(writer, "   🔍 Environment check: IMAGE_REGISTRY=%q IMAGE_TAG=%q\n", registry, tag)

	// CI/CD Optimization: Try to pull from registry if configured
	if pulledImageName, pulled, err := tryPullFromRegistry(ctx, "datastorage", writer); pulled {
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
		"--build-arg", fmt.Sprintf("GOARCH=%s", runtime.GOARCH),
		"-t", imageName,
		"--force-rm=false", // Disable auto-cleanup to avoid podman cleanup errors
		"-f", filepath.Join(projectRoot, "docker", "data-storage.Dockerfile"),
		projectRoot,
	)
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		// Check if image was actually built despite error (podman cleanup issue)
		checkAgain := exec.CommandContext(ctx, "podman", "image", "exists", imageName)
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
func StartDSBootstrap(ctx context.Context, cfg DSBootstrapConfig, writer io.Writer) (*DSBootstrapInfra, error) {
	// Default HealthPort to DataStoragePort+10000 if not explicitly set.
	// Offset 10000 avoids collision with MetricsPort (which is typically DataStoragePort+1000).
	if cfg.HealthPort == 0 {
		cfg.HealthPort = cfg.DataStoragePort + 10000
	}

	// Build infrastructure references
	infra := &DSBootstrapInfra{
		PostgresContainer:    fmt.Sprintf("%s_postgres_test", cfg.ServiceName),
		RedisContainer:       fmt.Sprintf("%s_redis_test", cfg.ServiceName),
		DataStorageContainer: fmt.Sprintf("%s_datastorage_test", cfg.ServiceName),
		MigrationsContainer:  fmt.Sprintf("%s_migrations", cfg.ServiceName),
		Network:              fmt.Sprintf("%s_test_network", cfg.ServiceName),
		ServiceURL:           fmt.Sprintf("http://localhost:%d", cfg.DataStoragePort),
		HealthURL:            fmt.Sprintf("http://localhost:%d", cfg.HealthPort),
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
	cleanupDSBootstrapContainers(ctx, infra)
	_, _ = fmt.Fprintf(writer, "   ✅ Cleanup complete\n\n")

	// Step 2: Network
	_, _ = fmt.Fprintf(writer, "🌐 Creating test network...\n")
	if err := createDSBootstrapNetwork(ctx, infra, writer); err != nil {
		return nil, fmt.Errorf("failed to create network: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Network ready: %s\n\n", infra.Network)

	// Step 3: PostgreSQL
	_, _ = fmt.Fprintf(writer, "🐘 Starting PostgreSQL...\n")
	if err := startDSBootstrapPostgreSQL(ctx, infra, writer); err != nil {
		return nil, fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "⏳ Waiting for PostgreSQL to be ready (two-phase: connection + queryability)...\n")
	// CRITICAL: Use two-phase health check to prevent "database system is starting up" errors
	// Phase 1: pg_isready (connection check)
	// Phase 2: SELECT 1 (queryability check)
	// Per DD-TEST-002: This prevents race condition in migrations
	if err := WaitForPostgreSQLReady(ctx, infra.PostgresContainer, defaultPostgresUser, defaultPostgresDB, writer); err != nil {
		return nil, fmt.Errorf("PostgreSQL failed to become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ PostgreSQL ready and queryable\n\n")

	// Step 4: Migrations
	_, _ = fmt.Fprintf(writer, "🔄 Running database migrations...\n")
	if err := runDSBootstrapMigrations(ctx, infra, projectRoot, writer); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Migrations applied successfully\n\n")

	// Step 5: Redis
	_, _ = fmt.Fprintf(writer, "🔴 Starting Redis...\n")
	if err := startDSBootstrapRedis(ctx, infra, writer); err != nil {
		return nil, fmt.Errorf("failed to start Redis: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "⏳ Waiting for Redis to be ready...\n")
	if err := waitForDSBootstrapRedisReady(ctx, infra, writer); err != nil {
		return nil, fmt.Errorf("redis failed to become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Redis ready\n\n")

	// Step 5.5: Generate signing certificate (AU-9: audit export signing)
	_, _ = fmt.Fprintf(writer, "🔏 Generating signing certificate for audit exports (AU-9)...\n")
	signingCertDir, err := generateBootstrapSigningCert(cfg.ServiceName, writer)
	if err != nil {
		return nil, fmt.Errorf("failed to generate signing certificate: %w", err)
	}
	infra.SigningCertDir = signingCertDir
	_, _ = fmt.Fprintf(writer, "   ✅ Signing certificate generated in %s\n\n", signingCertDir)

	// Step 6: DataStorage
	_, _ = fmt.Fprintf(writer, "📦 Starting DataStorage service...\n")
	if err := startDSBootstrapService(ctx, infra, imageName, projectRoot, writer); err != nil {
		return nil, fmt.Errorf("failed to start DataStorage: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "⏳ Waiting for DataStorage HTTP endpoint to be ready...\n")
	if err := waitForDSBootstrapHTTPHealth(ctx, infra, 30*time.Second, writer); err != nil {
		// Print container logs for debugging
		_, _ = fmt.Fprintf(writer, "\n⚠️  DataStorage failed to become healthy. Container logs:\n")
		logsCmd := exec.CommandContext(ctx, "podman", "logs", infra.DataStorageContainer)
		logsCmd.Stdout = writer
		logsCmd.Stderr = writer
		_ = logsCmd.Run()
		return nil, fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage ready\n\n")

	// #1661 Phase 55: action type seeding via DS's Postgres-backed API was removed
	// (DD-WORKFLOW-018 dropped the action_type_taxonomy table and the FK constraint
	// this step used to satisfy). Suites needing ActionType CRDs to exist for DS's
	// informer-backed cache now seed them directly via their own K8s client.

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
func cleanupDSBootstrapContainers(ctx context.Context, infra *DSBootstrapInfra) {
	containers := []string{
		infra.PostgresContainer,
		infra.RedisContainer,
		infra.DataStorageContainer,
		infra.MigrationsContainer,
	}

	for _, container := range containers {
		stopCmd := exec.CommandContext(ctx, "podman", "stop", container)
		_ = stopCmd.Run() // Ignore errors

		rmCmd := exec.CommandContext(ctx, "podman", "rm", container)
		_ = rmCmd.Run() // Ignore errors
	}
}

// createDSBootstrapNetwork creates the test network
func createDSBootstrapNetwork(ctx context.Context, infra *DSBootstrapInfra, writer io.Writer) error {
	// Check if network already exists
	checkCmd := exec.CommandContext(ctx, "podman", "network", "exists", infra.Network)
	if checkCmd.Run() == nil {
		return nil // Network exists
	}

	// Create network
	cmd := exec.CommandContext(ctx, "podman", "network", "create", infra.Network)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// PullImageWithRetry pulls a container image with exponential backoff.
// This prevents transient registry failures (e.g., Docker Hub 504) from failing the entire test suite.
func PullImageWithRetry(ctx context.Context, image string, maxRetries int, writer io.Writer) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		cmd := exec.CommandContext(ctx, "podman", "pull", image)
		cmd.Stdout = writer
		cmd.Stderr = writer
		if err := cmd.Run(); err != nil {
			lastErr = err
			if attempt < maxRetries {
				backoff := time.Duration(1<<uint(attempt)) * time.Second // 2s, 4s, 8s...
				_, _ = fmt.Fprintf(writer, "   ⚠️  Image pull failed (attempt %d/%d), retrying in %v: %v\n", attempt, maxRetries, backoff, err)
				time.Sleep(backoff)
			}
			continue
		}
		if attempt > 1 {
			_, _ = fmt.Fprintf(writer, "   ✅ Image pull succeeded on attempt %d/%d\n", attempt, maxRetries)
		}
		return nil
	}
	return fmt.Errorf("image pull failed after %d attempts: %w", maxRetries, lastErr)
}

// startDSBootstrapPostgreSQL starts the PostgreSQL container
func startDSBootstrapPostgreSQL(ctx context.Context, infra *DSBootstrapInfra, writer io.Writer) error {
	cfg := infra.Config

	const postgresImage = "docker.io/library/postgres:16-alpine"
	if err := PullImageWithRetry(ctx, postgresImage, 3, writer); err != nil {
		return fmt.Errorf("failed to pull PostgreSQL image: %w", err)
	}

	cmd := exec.CommandContext(ctx, "podman", "run", "-d",
		"--name", infra.PostgresContainer,
		"--network", infra.Network,
		"-p", fmt.Sprintf("%d:5432", cfg.PostgresPort),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", defaultPostgresUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", defaultPostgresPassword),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", defaultPostgresDB),
		postgresImage,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// runDSBootstrapMigrations applies database migrations using the goose Go library (DD-012).
// Connects directly to PostgreSQL via the Podman-exposed port and runs goose.Up().
func runDSBootstrapMigrations(ctx context.Context, infra *DSBootstrapInfra, projectRoot string, writer io.Writer) error {
	migrationsDir := filepath.Join(projectRoot, defaultMigrationsPath)

	connStr := fmt.Sprintf("host=localhost port=%d user=%s password=%s dbname=%s sslmode=disable",
		infra.Config.PostgresPort, defaultPostgresUser, defaultPostgresPassword, defaultPostgresDB)

	db, err := sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("failed to connect to PostgreSQL for migrations: %w", err)
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("failed to ping PostgreSQL: %w", err)
	}

	return RunGooseMigrations(ctx, db, migrationsDir, writer)
}

// startDSBootstrapRedis starts the Redis container
func startDSBootstrapRedis(ctx context.Context, infra *DSBootstrapInfra, writer io.Writer) error {
	cfg := infra.Config

	const redisImage = "redis:7-alpine"
	if err := PullImageWithRetry(ctx, redisImage, 3, writer); err != nil {
		return fmt.Errorf("failed to pull Redis image: %w", err)
	}

	cmd := exec.CommandContext(ctx, "podman", "run", "-d",
		"--name", infra.RedisContainer,
		"--network", infra.Network,
		"-p", fmt.Sprintf("%d:6379", cfg.RedisPort),
		redisImage,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// waitForDSBootstrapRedisReady waits for Redis to be ready
func waitForDSBootstrapRedisReady(ctx context.Context, infra *DSBootstrapInfra, writer io.Writer) error {
	for i := 1; i <= 10; i++ {
		cmd := exec.CommandContext(ctx, "podman", "exec", infra.RedisContainer,
			"redis-cli", "ping")
		output, err := cmd.Output()
		if err == nil && strings.Contains(string(output), "PONG") {
			_, _ = fmt.Fprintf(writer, "   Redis ready (attempt %d/10)\n", i)
			return nil
		}
		if i == 10 {
			return fmt.Errorf("redis failed to become ready after 10 seconds")
		}
		_, _ = fmt.Fprintf(writer, "   Waiting... (attempt %d/10)\n", i)
		time.Sleep(1 * time.Second)
	}
	return nil
}

// startDSBootstrapService starts the DataStorage container using a pre-built image
// The image should be built using BuildDataStorageImage() before calling this function
//
// DD-AUTH-014: Platform-specific network configuration (per DD_AUTH_014_MACOS_PODMAN_LIMITATION.md)
//   - Linux CI/CD: --network=host (Option D) - Container can reach localhost directly
//   - macOS: Bridge network (Option A) - Requires IPv6 disabled + kubeconfig rewrite to IPv4
func startDSBootstrapService(ctx context.Context, infra *DSBootstrapInfra, imageName string, projectRoot string, writer io.Writer) error {
	cfg := infra.Config
	configDir := filepath.Join(projectRoot, cfg.ConfigDir)

	// DD-AUTH-014: Platform-specific network configuration
	// Linux: Use --network=host when envtest kubeconfig is provided (allows direct localhost access)
	// macOS: Always use bridge network (--network=host doesn't work on macOS Podman VM)
	useHostNetwork := false
	if cfg.EnvtestKubeconfig != "" && runtime.GOOS == "linux" {
		useHostNetwork = true
		_, _ = fmt.Fprintf(writer, "   🌐 Using host network (Linux CI/CD) - container can reach localhost directly\n")
	} else if cfg.EnvtestKubeconfig != "" {
		_, _ = fmt.Fprintf(writer, "   🌐 Using bridge network (macOS) - kubeconfig rewrites 127.0.0.1 → host.containers.internal\n")
		_, _ = fmt.Fprintf(writer, "   ⚠️  Requires IPv6 disabled: sudo networksetup -setv6off Wi-Fi\n")
	}

	// Base container arguments
	args := []string{"run", "-d",
		"--name", infra.DataStorageContainer,
	}

	// Network configuration (platform-specific)
	if useHostNetwork {
		// Linux: Host network mode (no port mapping needed)
		args = append(args, "--network", "host")
	} else {
		// macOS: Bridge network (requires port mapping)
		args = append(args,
			"--network", infra.Network,
			"-p", fmt.Sprintf("%d:8080", cfg.DataStoragePort),
			"-p", fmt.Sprintf("%d:8081", cfg.HealthPort),
			"-p", fmt.Sprintf("%d:9090", cfg.MetricsPort),
		)
	}

	// Database/Redis configuration (network-dependent)
	var postgresHost string
	var postgresPort int
	var redisAddr string
	var listenPort int
	var healthListenPort int

	if useHostNetwork {
		// Host network: Access PostgreSQL/Redis via localhost at their exposed ports
		// PostgreSQL exposes internal 5432 → external cfg.PostgresPort
		// Redis exposes internal 6379 → external cfg.RedisPort
		postgresHost = "localhost"
		postgresPort = cfg.PostgresPort // Use exposed port (e.g., 15437)
		redisAddr = fmt.Sprintf("localhost:%d", cfg.RedisPort)

		// CRITICAL: Host network - no port mapping, so listen on external port
		// Test infrastructure expects DataStorage on cfg.DataStoragePort
		listenPort = cfg.DataStoragePort
		healthListenPort = cfg.HealthPort
	} else {
		// Bridge network: Access via container names at internal ports
		postgresHost = infra.PostgresContainer
		postgresPort = 5432 // Internal PostgreSQL port
		redisAddr = fmt.Sprintf("%s:6379", infra.RedisContainer)

		// Bridge network: Always listen on 8080/8081, port mapping handles external
		// BACKWARDS COMPATIBLE: This is the original behavior for macOS
		// Example: -p 18096:8080 maps external 18096 → internal 8080
		listenPort = 8080
		healthListenPort = 8081
	}

	// AU-9: Mount signing certificate for audit export signing
	if infra.SigningCertDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/etc/certs:ro", infra.SigningCertDir))
	}

	// Common configuration
	args = append(args,
		"-v", fmt.Sprintf("%s:/etc/datastorage:ro", configDir),
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		"-e", fmt.Sprintf("POSTGRES_HOST=%s", postgresHost),
		"-e", fmt.Sprintf("POSTGRES_PORT=%d", postgresPort),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", defaultPostgresUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", defaultPostgresPassword),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", defaultPostgresDB),
		"-e", "CONN_MAX_LIFETIME=30m",
		"-e", fmt.Sprintf("REDIS_ADDR=%s", redisAddr),
		"-e", fmt.Sprintf("PORT=%d", listenPort),
		"-e", fmt.Sprintf("HEALTH_PORT=%d", healthListenPort),
	)

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

	cmd := exec.CommandContext(ctx, "podman", args...)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

// waitForDSBootstrapHTTPHealth waits for DataStorage health endpoint to respond with 200 OK.
// Issue #753: Health probes moved to dedicated port (8081) with /readyz endpoint.
func waitForDSBootstrapHTTPHealth(ctx context.Context, infra *DSBootstrapInfra, timeout time.Duration, writer io.Writer) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(infra.HealthURL + "/readyz")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}

		// Log progress every 10 seconds
		if time.Now().Unix()%10 == 0 {
			_, _ = fmt.Fprintf(writer, "   Still waiting for %s/readyz...\n", infra.HealthURL)
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for %s/readyz to become healthy after %v", infra.HealthURL, timeout)
}

// generateBootstrapSigningCert creates a self-signed certificate for audit export signing.
// AU-9: DataStorage requires a signing certificate at /etc/certs/{tls.crt, tls.key}.
// Returns the path to the temp directory containing tls.crt and tls.key.
func generateBootstrapSigningCert(serviceName string, writer io.Writer) (string, error) {
	tmpDir, err := os.MkdirTemp("", fmt.Sprintf("ds-signing-%s-*", serviceName))
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	if err := os.Chmod(tmpDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to chmod temp dir: %w", err)
	}

	pair, err := cert.GenerateSelfSigned(cert.CertificateOptions{
		CommonName:       fmt.Sprintf("datastorage-signing-%s", serviceName),
		Organization:     "Kubernaut Integration Tests",
		DNSNames:         []string{"localhost"},
		ValidityDuration: 24 * time.Hour,
		KeySize:          2048,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate self-signed cert: %w", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "tls.crt"), pair.CertPEM, 0o644); err != nil {
		return "", fmt.Errorf("failed to write tls.crt: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "tls.key"), pair.KeyPEM, 0o644); err != nil {
		return "", fmt.Errorf("failed to write tls.key: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   🔏 Generated signing cert: CN=datastorage-signing-%s, validity=24h\n", serviceName)
	return tmpDir, nil
}
