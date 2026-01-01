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

// Gateway Integration Test Infrastructure Constants
// Port Allocation per DD-TEST-001: Port Allocation Strategy
const (
	// Gateway Integration Test Ports
	GatewayIntegrationPostgresPort    = 15437 // PostgreSQL (DataStorage backend)
	GatewayIntegrationRedisPort       = 16383 // Redis (DataStorage DLQ)
	GatewayIntegrationDataStoragePort = 18091 // DataStorage API (Audit + State)
	GatewayIntegrationMetricsPort     = 19091 // DataStorage Metrics

	// Container Names (unique to Gateway integration tests)
	GatewayIntegrationPostgresContainer    = "gateway_postgres_test"
	GatewayIntegrationRedisContainer       = "gateway_redis_test"
	GatewayIntegrationDataStorageContainer = "gateway_datastorage_test"
	GatewayIntegrationMigrationsContainer  = "gateway_migrations"
	GatewayIntegrationNetwork              = "gateway_test_network"
)

// StartGatewayIntegrationInfrastructure starts the Gateway integration test infrastructure
// using sequential podman run commands per DD-TEST-002.
//
// Pattern: DD-TEST-002 Sequential Startup Pattern
// - Sequential container startup (eliminates race conditions)
// - Explicit health checks after each service
// - No podman-compose (avoids parallel startup issues)
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

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Gateway Integration Test Infrastructure Setup\n")
	fmt.Fprintf(writer, "Per DD-TEST-002: Sequential Startup Pattern\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", GatewayIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:          localhost:%d\n", GatewayIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", GatewayIntegrationDataStoragePort)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	// ============================================================================
	// STEP 1: Cleanup existing containers (using shared utility)
	// ============================================================================
	fmt.Fprintf(writer, "🧹 Cleaning up existing containers...\n")
	CleanupContainers([]string{
		GatewayIntegrationPostgresContainer,
		GatewayIntegrationRedisContainer,
		GatewayIntegrationDataStorageContainer,
		GatewayIntegrationMigrationsContainer,
	}, writer)
	fmt.Fprintf(writer, "   ✅ Cleanup complete\n\n")

	// ============================================================================
	// STEP 2: Network setup (SKIPPED - using host network for localhost connectivity)
	// ============================================================================
	// Note: Using host network instead of custom podman network to avoid DNS resolution issues
	// All services connect via localhost:PORT (same pattern as other successful services)
	fmt.Fprintf(writer, "🌐 Network: Using host network for localhost connectivity\n\n")

	// ============================================================================
	// STEP 3: Start PostgreSQL FIRST (using shared utility)
	// ============================================================================
	fmt.Fprintf(writer, "🐘 Starting PostgreSQL...\n")
	if err := StartPostgreSQL(PostgreSQLConfig{
		ContainerName: GatewayIntegrationPostgresContainer,
		Port:          GatewayIntegrationPostgresPort,
		DBName:        "kubernaut",
		DBUser:        "kubernaut",
		DBPassword:    "kubernaut-test-password",
	}, writer); err != nil {
		return fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// CRITICAL: Wait for PostgreSQL to be ready before proceeding (using shared utility)
	fmt.Fprintf(writer, "⏳ Waiting for PostgreSQL to be ready...\n")
	if err := WaitForPostgreSQLReady(GatewayIntegrationPostgresContainer, "kubernaut", "kubernaut", writer); err != nil {
		return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
	}
	fmt.Fprintf(writer, "\n")

	// ============================================================================
	// STEP 4: Run migrations
	// ============================================================================
	fmt.Fprintf(writer, "🔄 Running database migrations...\n")
	if err := runGatewayMigrations(projectRoot, writer); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ Migrations applied successfully\n\n")

	// ============================================================================
	// STEP 5: Start Redis SECOND (using shared utility)
	// ============================================================================
	fmt.Fprintf(writer, "🔴 Starting Redis...\n")
	if err := StartRedis(RedisConfig{
		ContainerName: GatewayIntegrationRedisContainer,
		Port:          GatewayIntegrationRedisPort,
	}, writer); err != nil {
		return fmt.Errorf("failed to start Redis: %w", err)
	}

	// Wait for Redis to be ready (using shared utility)
	fmt.Fprintf(writer, "⏳ Waiting for Redis to be ready...\n")
	if err := WaitForRedisReady(GatewayIntegrationRedisContainer, writer); err != nil {
		return fmt.Errorf("Redis failed to become ready: %w", err)
	}
	fmt.Fprintf(writer, "\n")

	// ============================================================================
	// STEP 6: Start DataStorage LAST
	// ============================================================================
	fmt.Fprintf(writer, "📦 Starting DataStorage service...\n")
	if err := startGatewayDataStorage(projectRoot, writer); err != nil {
		return fmt.Errorf("failed to start DataStorage: %w", err)
	}

	// CRITICAL: Wait for DataStorage HTTP endpoint to be ready (using shared utility)
	fmt.Fprintf(writer, "⏳ Waiting for DataStorage HTTP endpoint to be ready...\n")
	if err := WaitForHTTPHealth(
		fmt.Sprintf("http://localhost:%d/health", GatewayIntegrationDataStoragePort),
		30*time.Second,
		writer,
	); err != nil {
		// Print container logs for debugging
		fmt.Fprintf(writer, "\n⚠️  DataStorage failed to become healthy. Container logs:\n")
		logsCmd := exec.Command("podman", "logs", GatewayIntegrationDataStorageContainer)
		logsCmd.Stdout = writer
		logsCmd.Stderr = writer
		_ = logsCmd.Run()
		return fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "\n")

	// ============================================================================
	// SUCCESS
	// ============================================================================
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "✅ Gateway Integration Infrastructure Ready\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:        localhost:%d\n", GatewayIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:             localhost:%d\n", GatewayIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage HTTP:  http://localhost:%d\n", GatewayIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  DataStorage Metrics: http://localhost:%d\n", GatewayIntegrationMetricsPort)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// StopGatewayIntegrationInfrastructure stops and cleans up the Gateway integration test infrastructure
//
// Pattern: DD-TEST-002 Sequential Cleanup
// - Stop containers in reverse order
// - Remove containers and network
// - Parallel-safe (called from SynchronizedAfterSuite)
//
// Returns:
// - error: Any errors during infrastructure cleanup
func StopGatewayIntegrationInfrastructure(writer io.Writer) error {
	fmt.Fprintf(writer, "🛑 Stopping Gateway Integration Infrastructure...\n")

	// Stop and remove containers (using shared utility)
	CleanupContainers([]string{
		GatewayIntegrationDataStorageContainer,
		GatewayIntegrationRedisContainer,
		GatewayIntegrationPostgresContainer,
		GatewayIntegrationMigrationsContainer,
	}, writer)

	// Remove network (ignore errors)
	networkCmd := exec.Command("podman", "network", "rm", GatewayIntegrationNetwork)
	_ = networkCmd.Run()

	fmt.Fprintf(writer, "✅ Gateway Integration Infrastructure stopped and cleaned up\n")
	return nil
}

// ============================================================================
// Service-Specific Helper Functions
// ============================================================================
// Note: Common functions (PostgreSQL, Redis, HTTP health, cleanup) moved to
// shared_integration_utils.go. Only Gateway-specific functions remain here.

// runGatewayMigrations applies database migrations
func runGatewayMigrations(projectRoot string, writer io.Writer) error {
	migrationsDir := filepath.Join(projectRoot, "migrations")

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

	// Use host.containers.internal for macOS compatibility (Podman VM can reach host)
	cmd := exec.Command("podman", "run", "--rm",
		"--name", GatewayIntegrationMigrationsContainer,
		"-v", fmt.Sprintf("%s:/migrations:ro", migrationsDir),
		"-e", "PGHOST=host.containers.internal",
		"-e", fmt.Sprintf("PGPORT=%d", GatewayIntegrationPostgresPort),
		"-e", "PGUSER=kubernaut",
		"-e", "PGPASSWORD=kubernaut-test-password",
		"-e", "PGDATABASE=kubernaut",
		"postgres:16-alpine",
		"bash", "-c", migrationScript,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// startGatewayDataStorage starts the DataStorage container for Gateway integration tests
func startGatewayDataStorage(projectRoot string, writer io.Writer) error {
	configDir := filepath.Join(projectRoot, "test", "integration", "gateway", "config")

	// Check if DataStorage image exists, build if not
	checkCmd := exec.Command("podman", "image", "exists", "kubernaut/datastorage:latest")
	if checkCmd.Run() != nil {
		fmt.Fprintf(writer, "   Building DataStorage image...\n")
		buildCmd := exec.Command("podman", "build",
			"-t", "kubernaut/datastorage:latest",
			"-f", filepath.Join(projectRoot, "docker", "data-storage.Dockerfile"),
			projectRoot,
		)
		buildCmd.Stdout = writer
		buildCmd.Stderr = writer
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build DataStorage image: %w", err)
		}
		fmt.Fprintf(writer, "   ✅ DataStorage image built\n")
	}

	// Use port mapping (not --network host) for macOS compatibility
	// macOS Podman runs in VM, so host network doesn't expose ports to Mac host
	// Per working pattern from other services: explicit port mapping
	cmd := exec.Command("podman", "run", "-d",
		"--name", GatewayIntegrationDataStorageContainer,
		"-p", fmt.Sprintf("%d:18091", GatewayIntegrationDataStoragePort),
		"-v", fmt.Sprintf("%s:/etc/datastorage:ro", configDir),
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		"kubernaut/datastorage:latest",
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
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

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// E2E COVERAGE CAPTURE (per DD-TEST-007)
// Go 1.20+ binary profiling for E2E coverage measurement
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// BuildGatewayImageWithCoverage builds the Gateway image with coverage instrumentation.
// Per DD-TEST-007: Uses GOFLAGS=-cover to enable binary profiling.
// Uses the standard Dockerfile with --build-arg GOFLAGS=-cover.
func BuildGatewayImageWithCoverage(writer io.Writer) error {
	projectRoot := getProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("project root not found")
	}

	dockerfilePath := filepath.Join(projectRoot, "docker", "gateway-ubi9.Dockerfile")
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		return fmt.Errorf("Gateway Dockerfile not found at %s", dockerfilePath)
	}

	containerCmd := "podman"
	if _, err := exec.LookPath("podman"); err != nil {
		containerCmd = "docker"
	}

	// Use unique image tag with coverage suffix
	imageTag := "e2e-test-coverage"
	imageName := fmt.Sprintf("localhost/kubernaut-gateway:%s", imageTag)
	fmt.Fprintf(writer, "  📦 Building Gateway with coverage: %s\n", imageName)

	// Build with GOFLAGS=-cover for E2E coverage
	// Using go-toolset:1.25 (no dnf update) reduces build time from 10min to 2-3min
	// CRITICAL: --no-cache ensures latest code changes are included (DD-TEST-002)
	cmd := exec.Command(containerCmd, "build",
		"--no-cache", // Force fresh build to include latest code changes
		"-t", imageName,
		"-f", dockerfilePath,
		"--build-arg", "GOFLAGS=-cover",
		projectRoot,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	cmd.Dir = projectRoot

	return cmd.Run()
}

// GetGatewayCoverageImageTag returns the coverage-enabled image tag
func GetGatewayCoverageImageTag() string {
	return "e2e-test-coverage"
}

// GetGatewayCoverageFullImageName returns the full image name with coverage tag
func GetGatewayCoverageFullImageName() string {
	return fmt.Sprintf("localhost/kubernaut-gateway:%s", GetGatewayCoverageImageTag())
}

// LoadGatewayCoverageImage loads the coverage-enabled image into Kind
func LoadGatewayCoverageImage(clusterName string, writer io.Writer) error {
	imageTag := GetGatewayCoverageImageTag()
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("kubernaut-gateway-%s.tar", imageTag))
	imageName := GetGatewayCoverageFullImageName()

	fmt.Fprintf(writer, "  Saving coverage image to tar file: %s...\n", tmpFile)
	saveCmd := exec.Command("podman", "save",
		"-o", tmpFile,
		imageName,
	)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	fmt.Fprintln(writer, "  Loading coverage image into Kind...")
	loadCmd := exec.Command("kind", "load", "image-archive",
		tmpFile,
		"--name", clusterName,
	)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		os.Remove(tmpFile)
		return fmt.Errorf("failed to load image: %w", err)
	}

	os.Remove(tmpFile)
	fmt.Fprintf(writer, "  ✅ Coverage image loaded and temp file cleaned\n")
	return nil
}

// GatewayCoverageManifest returns the Gateway deployment manifest with GOCOVERDIR set
// This is based on test/e2e/gateway/gateway-deployment.yaml with coverage modifications
func GatewayCoverageManifest() string {
	imageName := GetGatewayCoverageFullImageName()

	return fmt.Sprintf(`---
# Gateway Service ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-config
  namespace: kubernaut-system
data:
  config.yaml: |
    server:
      listen_addr: ":8080"
      read_timeout: 30s
      write_timeout: 30s
      idle_timeout: 120s

    middleware:
      rate_limit:
        requests_per_minute: 100
        burst: 10

    infrastructure:
      # ADR-032: Data Storage URL is MANDATORY for P0 services (Gateway)
      # DD-API-001: Gateway uses OpenAPI client to communicate with Data Storage
      data_storage_url: "http://datastorage.kubernaut-system.svc.cluster.local:8080"

    processing:
      deduplication:
        ttl: 10s  # Minimum allowed TTL (production: 5m)

      environment:
        cache_ttl: 5s              # Fast cache for E2E tests (production: 30s)
        configmap_namespace: "kubernaut-system"
        configmap_name: "kubernaut-environment-overrides"

      priority:
        policy_path: "/etc/gateway-policy/priority-policy.rego"

---
# Gateway Service Rego Policy ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: gateway-rego-policy
  namespace: kubernaut-system
data:
  priority-policy.rego: |
    package priority

    # Default priority assignment based on severity and environment
    default priority := "P2"

    # P0: Critical alerts in production
    priority := "P0" if {
        input.severity == "critical"
        input.environment == "production"
    }

    # P1: Critical alerts in staging or warning in production
    priority := "P1" if {
        input.severity == "critical"
        input.environment == "staging"
    }

    priority := "P1" if {
        input.severity == "warning"
        input.environment == "production"
    }

---
# Gateway Service Deployment (Coverage-Enabled)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
  labels:
    app: gateway
    component: webhook
spec:
  replicas: 1
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
        component: webhook
    spec:
      serviceAccountName: gateway
      terminationGracePeriodSeconds: 30
      # E2E Coverage: Run as root to write to hostPath volume (acceptable for E2E tests)
      securityContext:
        runAsUser: 0
        runAsGroup: 0
      # Run on control-plane node to access NodePort mappings
      nodeSelector:
        node-role.kubernetes.io/control-plane: ""
      tolerations:
        - key: node-role.kubernetes.io/control-plane
          operator: Exists
          effect: NoSchedule
      containers:
        - name: gateway
          image: %s
          imagePullPolicy: Never  # Use local image loaded into Kind
          args:
            - "--config=/etc/gateway/config.yaml"
          env:
          # E2E Coverage: Set GOCOVERDIR to enable coverage capture
          - name: GOCOVERDIR
            value: /coverdata
          ports:
            - name: http
              containerPort: 8080
              protocol: TCP
            - name: metrics
              containerPort: 9090
              protocol: TCP
          volumeMounts:
            - name: config
              mountPath: /etc/gateway
              readOnly: true
            - name: rego-policy
              mountPath: /etc/gateway-policy
              readOnly: true
            # E2E Coverage: Mount coverage directory
            - name: coverdata
              mountPath: /coverdata
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 5
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /ready
              port: 8080
            initialDelaySeconds: 30
            periodSeconds: 5
            timeoutSeconds: 5
            failureThreshold: 6
          resources:
            requests:
              memory: "256Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
              cpu: "500m"
      volumes:
        - name: config
          configMap:
            name: gateway-config
        - name: rego-policy
          configMap:
            name: gateway-rego-policy
        # E2E Coverage: hostPath volume for coverage data
        - name: coverdata
          hostPath:
            path: /coverdata
            type: DirectoryOrCreate

---
# Gateway Service
apiVersion: v1
kind: Service
metadata:
  name: gateway-service
  namespace: kubernaut-system
  labels:
    app: gateway
spec:
  type: NodePort
  selector:
    app: gateway
  ports:
    - name: http
      protocol: TCP
      port: 8080
      targetPort: 8080
      nodePort: 30080  # Expose on host for E2E testing
    - name: metrics
      protocol: TCP
      port: 9090
      targetPort: 9090
      nodePort: 30090  # Expose metrics on host

---
# Gateway ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: gateway
  namespace: kubernaut-system

---
# Gateway ClusterRole (for CRD creation and namespace access)
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: gateway-role
rules:
  # RemediationRequest CRD access (updated to kubernaut.ai API group)
  - apiGroups: ["kubernaut.ai"]
    resources: ["remediationrequests"]
    verbs: ["create", "get", "list", "watch", "update", "patch"]

  # RemediationRequest status subresource access (DD-GATEWAY-011)
  # Required for Gateway StatusUpdater to update Status.Deduplication
  - apiGroups: ["kubernaut.ai"]
    resources: ["remediationrequests/status"]
    verbs: ["update", "patch"]

  # Namespace access (for environment classification)
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list", "watch"]

  # ConfigMap access (for environment overrides)
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch"]

---
# Gateway ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: gateway-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: gateway-role
subjects:
  - kind: ServiceAccount
    name: gateway
    namespace: kubernaut-system
`, imageName)
}

// DeployGatewayCoverageManifest deploys the coverage-enabled Gateway
func DeployGatewayCoverageManifest(kubeconfigPath string, writer io.Writer) error {
	manifest := GatewayCoverageManifest()

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply coverage Gateway manifest: %w", err)
	}

	fmt.Fprintln(writer, "⏳ Waiting for coverage-enabled Gateway to be ready...")
	return waitForGatewayHealth(kubeconfigPath, writer, 90*time.Second)
}

// ScaleDownGatewayForCoverage scales the Gateway deployment to 0 to trigger graceful shutdown
// and flush coverage data to /coverdata
func ScaleDownGatewayForCoverage(kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "📊 Scaling down Gateway for coverage flush...")

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"scale", "deployment", "gateway",
		"-n", "kubernaut-system", "--replicas=0")
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to scale down Gateway: %w", err)
	}

	// Wait for pod to terminate using kubectl wait (blocks until pod is deleted)
	fmt.Fprintln(writer, "⏳ Waiting for Gateway pod to terminate...")
	waitCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"wait", "--for=delete", "pod",
		"-l", "app=gateway",
		"-n", "kubernaut-system",
		"--timeout=60s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	_ = waitCmd.Run() // Ignore error if no pods exist

	// Coverage data is written on SIGTERM before pod exits, no additional wait needed
	// The kubectl wait --for=delete already blocks until pod is fully terminated

	fmt.Fprintln(writer, "✅ Gateway scaled down, coverage data should be flushed")
	return nil
}

// waitForGatewayHealth waits for Gateway to become healthy using kubectl
func waitForGatewayHealth(kubeconfigPath string, writer io.Writer, timeout time.Duration) error {
	// Wait for deployment to be ready
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"wait", "deployment/gateway",
		"-n", "kubernaut-system",
		"--for=condition=Available",
		fmt.Sprintf("--timeout=%s", timeout))
	cmd.Stdout = writer
	cmd.Stderr = writer

	return cmd.Run()
}

// Note: getProjectRoot() is defined in aianalysis.go and shared across all infrastructure files
