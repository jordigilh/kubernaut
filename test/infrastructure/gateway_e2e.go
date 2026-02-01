/*
Copyright 2025 Jordi Gil.

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
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ========================================
// GATEWAY E2E INFRASTRUCTURE
// ========================================
//
// Gateway E2E tests require:
// - Kind cluster (RemediationRequest CRD)
// - PostgreSQL (Data Storage dependency)
// - Redis (Data Storage dependency)
// - Data Storage service (audit events)
// - Gateway service (signal ingestion)
//
// Pattern: Follows AIAnalysis E2E infrastructure pattern
// Authority: test/infrastructure/aianalysis.go
// ========================================

// Gateway E2E service ports (DD-TEST-001 port allocation strategy)
const (
	GatewayE2EHostPort     = 8080  // Gateway API (NodePort 30080 → host port 8080)
	GatewayE2EMetricsPort  = 9080  // Gateway metrics
	GatewayDataStoragePort = 30081 // Data Storage NodePort (from shared deployDataStorage)
	DataStorageE2EHostPort = 18091 // Data Storage host port (NodePort 30081 → host port 18091)

	// Gateway Integration test ports (restored from git history for backward compatibility)
	GatewayIntegrationPostgresPort    = 15437 // PostgreSQL (DataStorage backend)
	GatewayIntegrationRedisPort       = 16383 // Redis (DataStorage DLQ)
	GatewayIntegrationDataStoragePort = 18091 // DataStorage API (Audit + State)
	GatewayIntegrationMetricsPort     = 19091 // DataStorage Metrics
)

// SetupGatewayInfrastructureParallel creates the full E2E infrastructure using HYBRID parallel pattern.
// This optimizes setup time by building images BEFORE cluster creation (eliminating idle time).
//
// HYBRID Parallel Execution Strategy (OPTIMIZED - 18% faster):
//
//	Phase 1 (PARALLEL):   Build Gateway image | Build DataStorage image (~2-3 min, NO CLUSTER YET)
//	Phase 2 (Sequential): Create Kind cluster + CRDs + namespace (~10-15 sec)
//	Phase 3 (PARALLEL):   Load Gateway image | Load DataStorage image | Deploy PostgreSQL+Redis (~30-60 sec)
//	Phase 4 (Sequential): Deploy DataStorage (~30s)
//	Phase 5 (Sequential): Deploy Gateway (~30s)
//
// Total time: ~4-5 minutes (vs ~5.5 minutes standard pattern)
// Savings: ~1 minute (18% faster) - cluster never sits idle
//
// Authority: docs/handoff/E2E_PATTERN_PERFORMANCE_ANALYSIS_JAN07.md
// Pattern: Hybrid (build-before-cluster) - eliminates cluster idle time during builds
// Reference: test/infrastructure/remediationorchestrator_e2e_hybrid.go (authoritative hybrid pattern)
func SetupGatewayInfrastructureParallel(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer, enableCoverage bool) error {
	coverageStatus := "DISABLED"
	if enableCoverage {
		coverageStatus = "ENABLED (DD-TEST-007)"
	}
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "🚀 Gateway E2E Infrastructure (HYBRID PARALLEL, Coverage: %s)\n", coverageStatus)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build images → Create cluster → Load → Deploy")
	_, _ = fmt.Fprintln(writer, "  Optimization: 18% faster (eliminates cluster idle time)")
	_, _ = fmt.Fprintln(writer, "  Authority: E2E_PATTERN_PERFORMANCE_ANALYSIS_JAN07.md")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := "kubernaut-system"

	// DD-TEST-007: Create coverdata directory BEFORE cluster creation if coverage enabled
	if enableCoverage {
		projectRoot := getProjectRoot()
		coverdataPath := filepath.Join(projectRoot, "coverdata")
		_, _ = fmt.Fprintf(writer, "📁 Creating coverage directory: %s\n", coverdataPath)
		if err := os.MkdirAll(coverdataPath, 0777); err != nil {
			return fmt.Errorf("failed to create coverdata directory: %w", err)
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images in PARALLEL (BEFORE cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel (NO CLUSTER YET)...")
	_, _ = fmt.Fprintln(writer, "  ├── Gateway image (direct podman build)")
	_, _ = fmt.Fprintln(writer, "  └── DataStorage image (with dynamic tag)")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~2-3 minutes")

	type buildResult struct {
		name      string
		imageName string
		err       error
	}

	buildResults := make(chan buildResult, 2)

	// Build Gateway image using standardized BuildImageForKind (with registry pull fallback)
	// Authority: docs/handoff/GATEWAY_VALIDATION_RESULTS_JAN07.md (Option A)
	// Registry Strategy: Attempts pull from ghcr.io first, falls back to local build
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "gateway",
			ImageName:        "gateway",  // No repo prefix, just service name
			DockerfilePath:   "docker/gateway-ubi9.Dockerfile",
			BuildContextPath: "", // Empty = project root
			EnableCoverage:   enableCoverage,
		}
		imageName, err := BuildImageForKind(cfg, writer)
		if err != nil {
			err = fmt.Errorf("Gateway image build failed: %w", err)
		}
		buildResults <- buildResult{name: "Gateway", imageName: imageName, err: err}
	}()

	// Build DataStorage image using new split API
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "datastorage",
			ImageName:        "kubernaut/datastorage",
			DockerfilePath:   "docker/data-storage.Dockerfile",
			BuildContextPath: "", // Empty = project root
			EnableCoverage:   enableCoverage, // Use parameter instead of env var
		}
		imageName, err := BuildImageForKind(cfg, writer)
		if err != nil {
			err = fmt.Errorf("DS image build failed: %w", err)
		}
		buildResults <- buildResult{name: "DataStorage", imageName: imageName, err: err}
	}()

	// Collect build results
	var gatewayImageName, dataStorageImageName string
	var buildErrors []string
	for i := 0; i < 2; i++ {
		r := <-buildResults
		if r.err != nil {
			buildErrors = append(buildErrors, fmt.Sprintf("%s: %v", r.name, r.err))
			_, _ = fmt.Fprintf(writer, "  ❌ %s build failed: %v\n", r.name, r.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s build completed\n", r.name)
			if r.name == "DataStorage" {
				dataStorageImageName = r.imageName
			} else if r.name == "Gateway" {
				gatewayImageName = r.imageName
			}
		}
	}

	if len(buildErrors) > 0 {
		return fmt.Errorf("image builds failed: %s", strings.Join(buildErrors, "; "))
	}

	_, _ = fmt.Fprintln(writer, "\n✅ All images built successfully!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster + CRDs + namespace (images ready, no idle time)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster + CRDs + namespace...")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~10-15 seconds")

	// Create Kind cluster
	_, _ = fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createGatewayKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// Install RemediationRequest CRD
	_, _ = fmt.Fprintln(writer, "📋 Installing RemediationRequest CRD...")
	crdPath := getProjectRoot() + "/config/crd/bases/kubernaut.ai_remediationrequests.yaml"
	crdCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", crdPath)
	crdCmd.Stdout = writer
	crdCmd.Stderr = writer
	if err := crdCmd.Run(); err != nil {
		return fmt.Errorf("failed to install RemediationRequest CRD: %w", err)
	}

	// Create namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Kind cluster ready!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images + Deploy infrastructure in PARALLEL
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⚡ PHASE 3: Loading images + Deploying infrastructure...")
	_, _ = fmt.Fprintln(writer, "  ├── Loading Gateway image to Kind")
	_, _ = fmt.Fprintln(writer, "  ├── Loading DataStorage image to Kind")
	_, _ = fmt.Fprintln(writer, "  └── Deploying PostgreSQL + Redis")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~30-60 seconds")

	type result struct {
		name string
		err  error
	}

	results := make(chan result, 3)

	// Goroutine 1: Load Gateway image (FIXED: Now uses split API!)
	// Authority: docs/handoff/GATEWAY_VALIDATION_RESULTS_JAN07.md (Option A)
	go func() {
		err := loadGatewayImageToKind(gatewayImageName, clusterName, writer)
		if err != nil {
			err = fmt.Errorf("Gateway image load failed: %w", err)
		}
		results <- result{name: "Gateway image", err: err}
	}()

	// Goroutine 2: Load DataStorage image using new split API
	go func() {
		err := LoadImageToKind(dataStorageImageName, "datastorage", clusterName, writer)
		if err != nil {
			err = fmt.Errorf("DS image load failed: %w", err)
		}
		results <- result{name: "DataStorage image", err: err}
	}()

	// Goroutine 3: Deploy PostgreSQL and Redis
	go func() {
		var err error
		if pgErr := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); pgErr != nil {
			err = fmt.Errorf("PostgreSQL deploy failed: %w", pgErr)
		} else if redisErr := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); redisErr != nil {
			err = fmt.Errorf("Redis deploy failed: %w", redisErr)
		}
		results <- result{name: "PostgreSQL+Redis", err: err}
	}()

	// Wait for all goroutines
	var errors []string
	for i := 0; i < 3; i++ {
		r := <-results
		if r.err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", r.name, r.err))
			_, _ = fmt.Fprintf(writer, "  ❌ %s failed: %v\n", r.name, r.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s completed\n", r.name)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("parallel load/deploy failed: %s", strings.Join(errors, "; "))
	}

	_, _ = fmt.Fprintln(writer, "✅ Images loaded + Infrastructure deployed!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Apply migrations + Deploy DataStorage (requires PostgreSQL)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4: Applying migrations + Deploying DataStorage...")

	// 4a. Apply database migrations
	_, _ = fmt.Fprintf(writer, "📋 Applying database migrations...\n")
	if err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// 4b. Deploy client ClusterRole for SAR permissions (DD-AUTH-014)
	// This enables Gateway ServiceAccount to pass SAR checks when calling DataStorage
	_, _ = fmt.Fprintf(writer, "🔐 Deploying data-storage-client ClusterRole (DD-AUTH-014)...\n")
	if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy client ClusterRole: %w", err)
	}

	// 4c. Deploy DataStorage service RBAC (DD-AUTH-014) - REQUIRED for pod creation
	_, _ = fmt.Fprintf(writer, "🔐 Deploying DataStorage service RBAC for auth middleware (DD-AUTH-014)...\n")
	if err := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy service RBAC: %w", err)
	}

	// 4c2. Create RoleBinding for Gateway ServiceAccount to access DataStorage (DD-AUTH-014)
	// This grants Gateway pod permission to emit audit events to DataStorage
	// Without this, Gateway's audit client will get 403 Forbidden from DataStorage middleware
	_, _ = fmt.Fprintf(writer, "🔐 Creating RoleBinding for Gateway → DataStorage access (DD-AUTH-014)...\n")
	if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, "gateway", writer); err != nil {
		return fmt.Errorf("failed to create Gateway DataStorage access RoleBinding: %w", err)
	}

	// 4d. Deploy DataStorage with middleware-based auth (DD-AUTH-014)
	_, _ = fmt.Fprintf(writer, "🚀 Deploying Data Storage Service with middleware-based auth...\n")
	if err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, dataStorageImageName, writer); err != nil {
		return fmt.Errorf("failed to deploy DataStorage: %w", err)
	}

	// 4e. Wait for DataStorage Service DNS + endpoints (Gateway dependency)
	// Root cause: Gateway starts → tries to emit audit events → "no such host" DNS errors
	// Pattern: DataStorage E2E (test/infrastructure/datastorage.go:waitForDataStorageServicesReady)
	// This function waits for:
	// 1. Pod Running + Ready
	// 2. Service endpoints populated
	// 3. Internal cluster DNS resolution working
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for DataStorage Service readiness (pod + endpoints + DNS)...\n")
	if err := waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("DataStorage readiness check failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage Service ready for internal cluster access\n")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5: Deploy Gateway (requires DataStorage)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 5: Deploying Gateway...")

	// Deploy Gateway service (coverage-enabled or standard)
	if enableCoverage {
		// Use coverage manifest (DD-TEST-007)
		_, _ = fmt.Fprintln(writer, "   Using coverage-enabled Gateway deployment...")
		if err := DeployGatewayCoverageManifest(kubeconfigPath, writer); err != nil {
			return fmt.Errorf("failed to deploy Gateway with coverage: %w", err)
		}
	} else {
		// Use standard deployment
		// Authority: docs/handoff/GATEWAY_VALIDATION_RESULTS_JAN07.md (Option A - parameter-based)
		if err := deployGatewayService(ctx, namespace, kubeconfigPath, gatewayImageName, writer); err != nil {
			return fmt.Errorf("failed to deploy Gateway: %w", err)
		}
	}

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ Gateway E2E infrastructure ready (HYBRID PARALLEL MODE)!")
	_, _ = fmt.Fprintf(writer, "  • Gateway: http://localhost:%d\n", GatewayE2EHostPort)
	_, _ = fmt.Fprintf(writer, "  • Gateway Metrics: http://localhost:%d/metrics\n", GatewayE2EMetricsPort)
	_, _ = fmt.Fprintf(writer, "  • DataStorage: http://localhost:%d (NodePort %d)\n", DataStorageE2EHostPort, GatewayDataStoragePort)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// SetupGatewayInfrastructureSequentialWithCoverage creates the full E2E infrastructure with coverage enabled using SEQUENTIAL setup.
// This is the RECOMMENDED approach that fixes Kind cluster timeout issues (Dec 25, 2025).
//
// SEQUENTIAL APPROACH (Build → Cluster → Load → Deploy):
// 1. Build images FIRST (Gateway ~2-3min with SKIP_SYSTEM_UPDATE, DataStorage ~1-2min)
// 2. Create Kind cluster (10s)
// 3. Load images immediately (30s) - no idle time for cluster
// 4. Deploy services (1-2min)
//
// Why Sequential vs Parallel:
// - OLD (Parallel): Cluster created → sits idle 10min during Gateway build → container crashes → FAIL
// - NEW (Sequential): Build 3min → create cluster → load immediately → SUCCESS
//
// Per DD-TEST-007: E2E Coverage Capture Standard
//
// Differences from standard setup:
// 1. Builds Gateway image with GOFLAGS=-cover + SKIP_SYSTEM_UPDATE=true (2-3min vs 10min)
// 2. Deploys Gateway with GOCOVERDIR=/coverdata
// 3. Uses hostPath volume for coverage data collection
//
// Usage: Set COVERAGE_MODE=true environment variable
func CreateGatewayCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "Gateway E2E Cluster Setup")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "Dependencies:")
	_, _ = fmt.Fprintf(writer, "  • PostgreSQL (port 5433) - Data Storage persistence\n")
	_, _ = fmt.Fprintf(writer, "  • Redis (port 6380) - Data Storage caching\n")
	_, _ = fmt.Fprintf(writer, "  • Data Storage (host port %d, NodePort %d) - Audit trail\n", DataStorageE2EHostPort, GatewayDataStoragePort)
	_, _ = fmt.Fprintf(writer, "  • Gateway (host port %d) - Signal ingestion\n", GatewayE2EHostPort)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Create Kind cluster
	_, _ = fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createGatewayKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Install RemediationRequest CRD (reuse from signalprocessing.go)
	_, _ = fmt.Fprintln(writer, "📋 Installing RemediationRequest CRD...")
	crdPath := getProjectRoot() + "/config/crd/bases/kubernaut.ai_remediationrequests.yaml" // Updated to new API group
	crdCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", crdPath)
	crdCmd.Stdout = writer
	crdCmd.Stderr = writer
	if err := crdCmd.Run(); err != nil {
		return fmt.Errorf("failed to install RemediationRequest CRD: %w", err)
	}

	// 3. Build and load Gateway Docker image
	_, _ = fmt.Fprintln(writer, "🐳 Building Gateway Docker image...")
	if err := buildAndLoadGatewayImage(clusterName, writer); err != nil {
		return fmt.Errorf("failed to build Gateway image: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Gateway E2E cluster created successfully")
	return nil
}

// DeployTestServices deploys Gateway and its dependencies to the Kind cluster
// This includes:
// 0. Namespace creation
// 1. PostgreSQL deployment
// 2. Redis deployment
// 3. Data Storage deployment
// 4. Gateway deployment
//
// NOTE: This function appears to be UNUSED in actual test code (only referenced in documentation)
// Consider removing in future cleanup if confirmed unused.
// DeployTestServices deploys Gateway E2E services including DataStorage with OAuth2-Proxy.
// TD-E2E-001 Phase 1: oauth2ProxyImage parameter added for SOC2 architecture parity.
func DeployTestServices(ctx context.Context, namespace, kubeconfigPath, dataStorageImage, gatewayImage string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "📦 Deploying Gateway E2E services...")

	// 0. Create namespace first (shared function from datastorage.go)
	// Deploy shared Data Storage infrastructure with OAuth2-Proxy (TD-E2E-001 Phase 1)
	// Use same image tags that were built and loaded earlier
	_, _ = fmt.Fprintln(writer, "📦 Deploying Data Storage infrastructure with OAuth2-Proxy...")
	if err := DeployDataStorageTestServices(ctx, namespace, kubeconfigPath, dataStorageImage, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage infrastructure: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "✅ Data Storage infrastructure deployed")

	// 5. Deploy Gateway service with parameter-based image name (no file I/O)
	_, _ = fmt.Fprintln(writer, "🚪 Deploying Gateway service...")
	if err := deployGatewayService(ctx, namespace, kubeconfigPath, gatewayImage, writer); err != nil {
		return fmt.Errorf("failed to deploy Gateway: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ All services deployed successfully")
	return nil
}

// DeleteGatewayCluster deletes the Kind cluster
func DeleteGatewayCluster(clusterName, kubeconfigPath string, testsFailed bool, writer io.Writer) error {
	// Use shared cleanup function with log export on failure
	return DeleteCluster(clusterName, "gateway", testsFailed, writer)
}

// ========================================
// INTERNAL HELPERS
// ========================================

// createGatewayKindCluster creates a Kind cluster for Gateway E2E tests
// REFACTORED: Now uses shared CreateKindClusterWithConfig() helper
// Authority: docs/handoff/TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md (Phase 1)
func createGatewayKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	opts := KindClusterOptions{
		ClusterName:             clusterName,
		KubeconfigPath:          kubeconfigPath,
		ConfigPath:              "test/infrastructure/kind-gateway-config.yaml",
		WaitTimeout:             "5m",
		DeleteExisting:          false,
		ReuseExisting:           false,
		UsePodman:               true,
		ProjectRootAsWorkingDir: true, // DD-TEST-007: For ./coverdata resolution
	}
	return CreateKindClusterWithConfig(opts, writer)
}

// buildGatewayImageOnly builds Gateway image without loading it to Kind.
// This is Phase 1 of the hybrid E2E pattern (build before cluster creation).
//
// Authority: docs/handoff/GATEWAY_VALIDATION_RESULTS_JAN07.md (Option A)
// Pattern: Build only, load later with loadGatewayImageToKind()
//
// Returns: Full image name with localhost/ prefix for later loading (e.g., "localhost/gateway:tag")
// Note: NO file I/O - image name is passed directly as parameter to deployGatewayService()
func buildGatewayImageOnly(writer io.Writer) (string, error) {
	projectRoot := getProjectRoot()
	if projectRoot == "" {
		return "", fmt.Errorf("project root not found")
	}

	dockerfilePath := filepath.Join(projectRoot, "docker", "gateway-ubi9.Dockerfile")
	if _, err := os.Stat(dockerfilePath); os.IsNotExist(err) {
		return "", fmt.Errorf("Gateway Dockerfile not found at %s", dockerfilePath)
	}

	// Generate unique image tag compatible with DD-TEST-001 (same format as shared build script)
	// Format: gateway-{username}-{hash}-{timestamp}
	imageTag := GenerateInfraImageName("gateway", "e2e")
	// GenerateInfraImageName returns "localhost/gateway:tag", use it directly
	imageName := imageTag
	_, _ = fmt.Fprintf(writer, "🔨 Building Gateway image: %s\n", imageName)

	// Build with podman (similar to BuildGatewayImageWithCoverage but without --build-arg GOFLAGS=-cover)
	cmd := exec.Command("podman", "build",
		"--no-cache",        // Force fresh build to include latest code changes
		"--pull=always",     // Force pull fresh base image (clears Go build cache in base image)
		"--build-arg", "GOCACHE=/tmp/go-build-cache", // Force Go to not use cached build artifacts
		"-t", imageName,
		"-f", dockerfilePath,
		projectRoot,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	cmd.Dir = projectRoot

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("Gateway image build failed: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Gateway image built: %s\n", imageName)
	return imageName, nil
}

// loadGatewayImageToKind loads a pre-built Gateway image to Kind cluster.
// This is Phase 3 of the hybrid E2E pattern (load after cluster creation).
//
// Authority: docs/handoff/GATEWAY_VALIDATION_RESULTS_JAN07.md (Option A)
// Pattern: Load pre-built image using LoadImageToKind() helper
func loadGatewayImageToKind(imageName, clusterName string, writer io.Writer) error {
	// Use the consolidated LoadImageToKind() helper
	return LoadImageToKind(imageName, "gateway", clusterName, writer)
}

// buildAndLoadGatewayImage builds Gateway Docker image using shared build utilities and loads it into Kind
// DD-TEST-001: Uses shared build script for unique container tags and multi-team testing support
//
// DEPRECATED for hybrid pattern: Use buildGatewayImageOnly() + loadGatewayImageToKind() instead
// Still used by: standard pattern E2E tests (if any)
func buildAndLoadGatewayImage(clusterName string, writer io.Writer) error {
	projectRoot := getProjectRoot()

	// Use shared build utilities (DD-TEST-001 compliant)
	// Benefits:
	// - Unique tags prevent multi-developer test conflicts
	// - Consistent with all other services (notification, signalprocessing, etc.)
	// - Zero maintenance (Platform Team owns shared script)
	// - Automatic cleanup support
	_, _ = fmt.Fprintln(writer, "   Building Gateway image via shared build utilities (DD-TEST-001)...")

	buildScript := filepath.Join(projectRoot, "scripts", "build-service-image.sh")
	buildCmd := exec.Command(buildScript,
		"gateway",
		"--kind",
		"--cluster", clusterName,
	)
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("shared build script failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "   ✅ Gateway image built and loaded to Kind with unique tag")
	return nil
}
// deployGatewayService deploys Gateway service to the cluster
// DD-TEST-001: Uses unique image tag for multi-developer testing support
//
// Parameters:
//   - gatewayImageName: Full image name (e.g., "localhost/gateway:tag") - REQUIRED
//
// Authority: docs/handoff/GATEWAY_VALIDATION_RESULTS_JAN07.md (Option A - parameter-based, no file I/O)
func deployGatewayService(ctx context.Context, namespace, kubeconfigPath, gatewayImageName string, writer io.Writer) error {
	projectRoot := getProjectRoot()

	if gatewayImageName == "" {
		return fmt.Errorf("gatewayImageName parameter is required (no file-based fallback)")
	}

	_, _ = fmt.Fprintf(writer, "   Using Gateway image: %s\n", gatewayImageName)

	// Read deployment manifest and replace hardcoded tag with actual tag
	deploymentPath := filepath.Join(projectRoot, "test/e2e/gateway/gateway-deployment.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return fmt.Errorf("failed to read deployment file: %w", err)
	}

	// Replace hardcoded tag with actual unique tag
	updatedContent := strings.ReplaceAll(string(deploymentContent),
		"localhost/kubernaut-gateway:e2e-test",
		gatewayImageName)

	// Create temporary modified deployment file
	tmpDeployment := filepath.Join(os.TempDir(), "gateway-deployment-e2e.yaml")
	if err := os.WriteFile(tmpDeployment, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("failed to write temp deployment: %w", err)
	}
	defer func() { _ = os.Remove(tmpDeployment) }()

	// Apply the modified deployment
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", tmpDeployment,
		"-n", namespace)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply Gateway deployment failed: %w", err)
	}

	// Wait for Gateway to be ready (extended timeout for RBAC propagation + initial image pull in Podman)
	_, _ = fmt.Fprintln(writer, "   Waiting for Gateway pod (may take up to 5 minutes for RBAC + initial startup)...")
	waitCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"wait", "--for=condition=ready", "pod",
		"-l", "app=gateway",
		"-n", namespace,
		"--timeout=300s") // 5 minutes for Podman-based Kind clusters
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("Gateway pod not ready: %w", err)
	}

	return nil
}
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
	_, _ = fmt.Fprintf(writer, "  📦 Building Gateway with coverage: %s\n", imageName)

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

func GetGatewayCoverageImageTag() string {
	return "e2e-test-coverage"
}

func GetGatewayCoverageFullImageName() string {
	return fmt.Sprintf("localhost/kubernaut-gateway:%s", GetGatewayCoverageImageTag())
}

func LoadGatewayCoverageImage(clusterName string, writer io.Writer) error {
	imageTag := GetGatewayCoverageImageTag()
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("kubernaut-gateway-%s.tar", imageTag))
	imageName := GetGatewayCoverageFullImageName()

	_, _ = fmt.Fprintf(writer, "  Saving coverage image to tar file: %s...\n", tmpFile)
	saveCmd := exec.Command("podman", "save",
		"-o", tmpFile,
		imageName,
	)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  Loading coverage image into Kind...")
	loadCmd := exec.Command("kind", "load", "image-archive",
		tmpFile,
		"--name", clusterName,
	)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to load image: %w", err)
	}

	_ = os.Remove(tmpFile)

	// CRITICAL: Remove Podman image immediately to free disk space
	// Image is now in Kind, Podman copy is duplicate
	_, _ = fmt.Fprintf(writer, "  🗑️  Removing Podman image to free disk space...\n")
	rmiCmd := exec.Command("podman", "rmi", "-f", imageName)
	rmiCmd.Stdout = writer
	rmiCmd.Stderr = writer
	if err := rmiCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "  ⚠️  Failed to remove Podman image (non-fatal): %v\n", err)
	} else {
		_, _ = fmt.Fprintf(writer, "  ✅ Podman image removed: %s\n", imageName)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Coverage image loaded and temp file cleaned\n")
	return nil
}

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
      # Service name: data-storage-service (matches production, required for SAR)
      # Port 8080: DataStorage container port (DD-AUTH-014: Direct access, no proxy)
      data_storage_url: "http://data-storage-service.kubernaut-system.svc.cluster.local:8080"

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

func DeployGatewayCoverageManifest(kubeconfigPath string, writer io.Writer) error {
	manifest := GatewayCoverageManifest()

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply coverage Gateway manifest: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "⏳ Waiting for coverage-enabled Gateway to be ready...")
	return waitForGatewayHealth(kubeconfigPath, writer, 90*time.Second)
}

// waitForGatewayHealth waits for the Gateway service to become healthy
// This is a helper wrapper around WaitForHTTPHealth specifically for Gateway E2E tests
func waitForGatewayHealth(kubeconfigPath string, writer io.Writer, timeout time.Duration) error {
	// Gateway health endpoint is available via NodePort on the Kind cluster
	// Using localhost as the cluster is accessible from the test machine
	healthURL := fmt.Sprintf("http://localhost:%d/health", GatewayE2EHostPort)
	return WaitForHTTPHealth(healthURL, timeout, writer)
}

func ScaleDownGatewayForCoverage(kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "📊 Scaling down Gateway for coverage flush...")

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"scale", "deployment", "gateway",
		"-n", "kubernaut-system", "--replicas=0")
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to scale down Gateway: %w", err)
	}

	// Wait for pod to terminate using kubectl wait (blocks until pod is deleted)
	_, _ = fmt.Fprintln(writer, "⏳ Waiting for Gateway pod to terminate...")
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

	_, _ = fmt.Fprintln(writer, "✅ Gateway scaled down, coverage data should be flushed")
	return nil
}
