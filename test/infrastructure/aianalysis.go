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
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// AIAnalysis E2E Infrastructure
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Port Allocation (per DD-TEST-001):
//   AIAnalysis:    Host 8084  → NodePort 30084 (API), 30184 (metrics), 30284 (health)
//   Data Storage:  Host 8081  → NodePort 30081 (API)
//   HolmesGPT-API: Host 8088  → NodePort 30088 (API)
//   PostgreSQL:    Host 5433  → NodePort 30433
//   Redis:         Host 6380  → NodePort 30380
//
// Dependencies:
//   AIAnalysis → HolmesGPT-API (AI analysis)
//   AIAnalysis → Data Storage (audit events)
//   HolmesGPT-API → Data Storage (workflow catalog, audit)
//   Data Storage → PostgreSQL (persistence)
//   Data Storage → Redis (caching/DLQ)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	// AIAnalysis ports per DD-TEST-001
	AIAnalysisHostPort    = 8084
	AIAnalysisNodePort    = 30084
	AIAnalysisMetricsPort = 30184
	AIAnalysisHealthPort  = 30284

	// Data Storage ports
	DataStorageHostPort = 8081
	DataStorageNodePort = 30081

	// HolmesGPT-API ports
	HolmesGPTAPIHostPort = 8088
	HolmesGPTAPINodePort = 30088

	// PostgreSQL ports
	PostgreSQLHostPort = 5433
	PostgreSQLNodePort = 30433

	// Redis ports
	RedisHostPort = 6380
	RedisNodePort = 30380
)

// CreateAIAnalysisCluster creates a Kind cluster for AIAnalysis E2E testing.
// This sets up the complete dependency chain:
// 1. Kind cluster with port mappings
// 2. AIAnalysis CRD
// 3. PostgreSQL + Redis (for Data Storage)
// 4. Data Storage service
// 5. HolmesGPT-API service
// 6. AIAnalysis controller
//
// Time: ~2-3 minutes
func CreateAIAnalysisCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "AIAnalysis E2E Cluster Setup")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Dependencies:")
	fmt.Fprintln(writer, "  • PostgreSQL (port 5433) - Data Storage persistence")
	fmt.Fprintln(writer, "  • Redis (port 6380) - Data Storage caching")
	fmt.Fprintln(writer, "  • Data Storage (port 8081) - Audit trail")
	fmt.Fprintln(writer, "  • HolmesGPT-API (port 8088) - AI analysis")
	fmt.Fprintln(writer, "  • AIAnalysis (port 8084) - CRD controller")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Create context for infrastructure deployment
	ctx := context.Background()
	namespace := "kubernaut-system"

	// 1. Create Kind cluster with AIAnalysis config
	fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createAIAnalysisKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Create namespace for deployments (ignore if already exists)
	fmt.Fprintln(writer, "📁 Creating namespace...")
	createNsCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"create", "namespace", namespace)
	nsOutput := &strings.Builder{}
	createNsCmd.Stdout = io.MultiWriter(writer, nsOutput)
	createNsCmd.Stderr = io.MultiWriter(writer, nsOutput)
	if err := createNsCmd.Run(); err != nil {
		// Ignore if namespace already exists
		if !strings.Contains(nsOutput.String(), "AlreadyExists") {
			return fmt.Errorf("failed to create namespace: %w", err)
		}
		fmt.Fprintln(writer, "  (namespace already exists, continuing...)")
	}

	// 3. Install AIAnalysis CRD
	fmt.Fprintln(writer, "📋 Installing AIAnalysis CRD...")
	if err := installAIAnalysisCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install AIAnalysis CRD: %w", err)
	}

	// 4. Deploy PostgreSQL (using shared function from datastorage.go)
	fmt.Fprintln(writer, "🐘 Deploying PostgreSQL...")
	if err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 5. Deploy Redis (using shared function from datastorage.go)
	fmt.Fprintln(writer, "🔴 Deploying Redis...")
	if err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 5. Wait for infrastructure to be ready
	fmt.Fprintln(writer, "⏳ Waiting for PostgreSQL and Redis to be ready...")
	if err := waitForAIAnalysisInfraReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("infrastructure not ready: %w", err)
	}

	// 7-9. Build all images in parallel (OPTIMIZATION: saves 3-4 minutes per E2E run)
	// Per DD-E2E-001: Parallel Image Build Pattern
	// Images are independent and can be built concurrently
	// Measured times (Dec 16, 2025): DS 1:22, HAPI 2:30, AA 3:53 (slowest)
	fmt.Fprintln(writer, "🔨 Building all images in parallel...")
	fmt.Fprintln(writer, "  • Data Storage (1-2 min)")
	fmt.Fprintln(writer, "  • HolmesGPT-API (2-3 min)")
	fmt.Fprintln(writer, "  • AIAnalysis controller (3-4 min) - slowest, determines total")

	// Build all images in parallel (DD-E2E-001 compliant)
	type imageBuildResult struct {
		name  string
		image string
		err   error
	}

	buildResults := make(chan imageBuildResult, 3)
	projectRoot := getProjectRoot()

	// Build Data Storage image (parallel)
	go func() {
		err := buildImageOnly("Data Storage", "localhost/kubernaut-datastorage:latest",
			"docker/data-storage.Dockerfile", projectRoot, writer)
		buildResults <- imageBuildResult{"datastorage", "localhost/kubernaut-datastorage:latest", err}
	}()

	// Build HolmesGPT-API image (parallel)
	go func() {
		err := buildImageOnly("HolmesGPT-API", "localhost/kubernaut-holmesgpt-api:latest",
			"holmesgpt-api/Dockerfile", projectRoot, writer)
		buildResults <- imageBuildResult{"holmesgpt-api", "localhost/kubernaut-holmesgpt-api:latest", err}
	}()

	// Build AIAnalysis controller image (parallel)
	go func() {
		// Check if E2E_COVERAGE is enabled (DD-TEST-007)
		var err error
		if os.Getenv("E2E_COVERAGE") == "true" {
			fmt.Fprintf(writer, "   📊 Building AIAnalysis with coverage instrumentation (GOFLAGS=-cover)\n")
			buildArgs := []string{"--build-arg", "GOFLAGS=-cover"}
			err = buildImageWithArgs("AIAnalysis controller", "localhost/kubernaut-aianalysis:latest",
				"docker/aianalysis.Dockerfile", projectRoot, buildArgs, writer)
		} else {
			err = buildImageOnly("AIAnalysis controller", "localhost/kubernaut-aianalysis:latest",
				"docker/aianalysis.Dockerfile", projectRoot, writer)
		}
		buildResults <- imageBuildResult{"aianalysis", "localhost/kubernaut-aianalysis:latest", err}
	}()

	// Wait for all builds to complete
	builtImages := make(map[string]string)
	for i := 0; i < 3; i++ {
		result := <-buildResults
		if result.err != nil {
			return fmt.Errorf("parallel build failed for %s: %w", result.name, result.err)
		}
		builtImages[result.name] = result.image
		fmt.Fprintf(writer, "  ✅ %s image built\n", result.name)
	}

	fmt.Fprintln(writer, "✅ All images built successfully (parallel - DD-E2E-001 compliant)")

	// Now deploy in sequence (deployment has dependencies)
	fmt.Fprintln(writer, "💾 Deploying Data Storage...")
	if err := deployDataStorageOnly(clusterName, kubeconfigPath, builtImages["datastorage"], writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage: %w", err)
	}

	fmt.Fprintln(writer, "🤖 Deploying HolmesGPT-API...")
	// FIX: Use pre-built image from parallel build phase (saves 10-15 min)
	if err := deployHolmesGPTAPIOnly(clusterName, kubeconfigPath, builtImages["holmesgpt-api"], writer); err != nil {
		return fmt.Errorf("failed to deploy HolmesGPT-API: %w", err)
	}

	fmt.Fprintln(writer, "🧠 Deploying AIAnalysis controller...")
	// FIX: Use pre-built image from parallel build phase (saves 3-4 min)
	if err := deployAIAnalysisControllerOnly(clusterName, kubeconfigPath, builtImages["aianalysis"], writer); err != nil {
		return fmt.Errorf("failed to deploy AIAnalysis controller: %w", err)
	}

	// FIX: Wait for all services to be ready before returning
	// NOTE: Coverage-instrumented binaries take longer to start (2-5 min vs 30s for production)
	// This ensures test suite's health check succeeds immediately (suite_test.go:169-172)
	fmt.Fprintln(writer, "⏳ Waiting for all services to be ready...")
	if err := waitForAllServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "✅ AIAnalysis E2E cluster ready!")
	fmt.Fprintf(writer, "  • AIAnalysis API: http://localhost:%d\n", AIAnalysisHostPort)
	fmt.Fprintf(writer, "  • AIAnalysis Metrics: http://localhost:%d/metrics\n", 9184)
	fmt.Fprintf(writer, "  • Data Storage: http://localhost:%d\n", DataStorageHostPort)
	fmt.Fprintf(writer, "  • HolmesGPT-API: http://localhost:%d\n", HolmesGPTAPIHostPort)
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// DeleteAIAnalysisCluster deletes the Kind cluster
func DeleteAIAnalysisCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "🗑️  Deleting AIAnalysis E2E cluster...")

	cmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete cluster: %w", err)
	}

	// Remove kubeconfig
	if kubeconfigPath != "" {
		_ = os.Remove(kubeconfigPath)
	}

	fmt.Fprintln(writer, "✅ Cluster deleted")
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Internal helper functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func createAIAnalysisKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Find config file
	configPath := findAIAnalysisKindConfig()
	if configPath == "" {
		return fmt.Errorf("kind-aianalysis-config.yaml not found")
	}

	// Check if cluster already exists (with cleanup if partial state)
	checkCmd := exec.Command("kind", "get", "clusters")
	output, _ := checkCmd.Output()
	if containsCluster(string(output), clusterName) {
		fmt.Fprintln(writer, "  Cluster already exists, reusing...")
		return exportKubeconfig(clusterName, kubeconfigPath, writer)
	}

	// Clean up any leftover Podman containers from previous failed runs
	// This fixes issues on macOS where Kind/Podman can leave orphaned containers
	fmt.Fprintln(writer, "  Cleaning up any leftover containers...")
	cleanupContainers := []string{
		clusterName + "-control-plane",
		clusterName + "-worker",
	}
	for _, containerName := range cleanupContainers {
		cleanupCmd := exec.Command("podman", "rm", "-f", containerName)
		_ = cleanupCmd.Run() // Ignore errors - container may not exist
	}

	// Ensure kubeconfig directory exists
	kubeconfigDir := filepath.Dir(kubeconfigPath)
	if err := os.MkdirAll(kubeconfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	// Remove any leftover kubeconfig lock file
	lockFile := kubeconfigPath + ".lock"
	_ = os.Remove(lockFile) // Ignore errors - file may not exist

	// Create cluster
	cmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", configPath,
		"--kubeconfig", kubeconfigPath,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	// Wait for cluster to be ready
	return waitForClusterReady(kubeconfigPath, writer)
}

func findAIAnalysisKindConfig() string {
	// First, try to find via runtime caller location (most reliable)
	_, currentFile, _, ok := runtime.Caller(0)
	if ok {
		dir := filepath.Dir(currentFile)
		configPath := filepath.Join(dir, "kind-aianalysis-config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			return configPath
		}
	}

	// Try relative paths from different working directories
	candidates := []string{
		"test/infrastructure/kind-aianalysis-config.yaml",
		"../test/infrastructure/kind-aianalysis-config.yaml",
		"../../test/infrastructure/kind-aianalysis-config.yaml",
		"../../../test/infrastructure/kind-aianalysis-config.yaml",
		// Also try from the test directory itself
		"../infrastructure/kind-aianalysis-config.yaml",
		"../../infrastructure/kind-aianalysis-config.yaml",
		"infrastructure/kind-aianalysis-config.yaml",
		// Try from this package's location
		"kind-aianalysis-config.yaml",
	}

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}

	return ""
}

func containsCluster(output, clusterName string) bool {
	lines := splitLines(output)
	for _, line := range lines {
		if line == clusterName {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	for _, line := range []byte(s) {
		if line == '\n' {
			continue
		}
	}
	// Simple split
	current := ""
	for _, c := range s {
		if c == '\n' {
			if current != "" {
				lines = append(lines, current)
			}
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func exportKubeconfig(clusterName, kubeconfigPath string, writer io.Writer) error {
	cmd := exec.Command("kind", "export", "kubeconfig",
		"--name", clusterName,
		"--kubeconfig", kubeconfigPath,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func waitForClusterReady(kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "  Waiting for cluster to be ready...")
	for i := 0; i < 60; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "nodes", "-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}")
		output, err := cmd.Output()
		if err == nil && containsReady(string(output)) {
			fmt.Fprintln(writer, "  Cluster nodes ready")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for cluster")
}

func containsReady(s string) bool {
	return len(s) > 0 && s != "" && (s == "True" || s == "True True")
}

func installAIAnalysisCRD(kubeconfigPath string, writer io.Writer) error {
	// Find CRD file
	crdPath := findCRDFile("kubernaut.ai_aianalyses.yaml")
	if crdPath == "" {
		return fmt.Errorf("AIAnalysis CRD not found")
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"apply", "-f", crdPath)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to apply CRD: %w", err)
	}

	// Wait for CRD to be established
	fmt.Fprintln(writer, "  Waiting for CRD to be established...")
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "crd", "aianalyses.kubernaut.ai")
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timeout waiting for CRD")
}

func findCRDFile(name string) string {
	// Try to find via runtime caller location first
	_, currentFile, _, ok := runtime.Caller(0)
	if ok {
		// Go up to project root (from test/infrastructure/)
		projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
		crdPath := filepath.Join(projectRoot, "config/crd/bases", name)
		if _, err := os.Stat(crdPath); err == nil {
			return crdPath
		}
	}

	candidates := []string{
		"config/crd/bases/" + name,
		"../config/crd/bases/" + name,
		"../../config/crd/bases/" + name,
		"../../../config/crd/bases/" + name,
		"config/crd/" + name,
		"../config/crd/" + name,
		"../../config/crd/" + name,
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}
	return ""
}

// NOTE: deployPostgreSQL, createInlinePostgreSQL, and deployRedis functions removed
// Now using shared functions from datastorage.go:
// - deployPostgreSQLInNamespace (used by Gateway, SignalProcessing, Notification, DataStorage)
// - deployRedisInNamespace (used by Gateway, SignalProcessing, Notification, DataStorage)
// This reduces code duplication and ensures consistent infrastructure across all services

// buildImageOnly builds a container image without deploying (for parallel builds)
// Per DD-E2E-001: Parallel Image Build Pattern
func buildImageOnly(name, imageTag, dockerfile, projectRoot string, writer io.Writer) error {
	return buildImageWithArgs(name, imageTag, dockerfile, projectRoot, nil, writer)
}

func buildImageWithArgs(name, imageTag, dockerfile, projectRoot string, buildArgs []string, writer io.Writer) error {
	fmt.Fprintf(writer, "  🔨 Building %s...\n", name)

	// Build base command
	cmdArgs := []string{"build", "--no-cache", "-t", imageTag}

	// Add optional build arguments
	if len(buildArgs) > 0 {
		cmdArgs = append(cmdArgs, buildArgs...)
	}

	// Add dockerfile and context
	cmdArgs = append(cmdArgs, "-f", dockerfile, ".")

	buildCmd := exec.Command("podman", cmdArgs...)
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build %s image: %w", name, err)
	}

	return nil
}

// deployDataStorage builds and deploys Data Storage (backward compatibility wrapper)
// DEPRECATED: New code should use parallel builds via buildImageOnly + deployDataStorageOnly
// This function is retained for Gateway and other services using the old pattern
func deployDataStorage(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Build image first
	projectRoot := getProjectRoot()
	if err := buildImageOnly("Data Storage", "localhost/kubernaut-datastorage:latest",
		"docker/data-storage.Dockerfile", projectRoot, writer); err != nil {
		return err
	}

	// Then deploy using the new pattern
	return deployDataStorageOnly(clusterName, kubeconfigPath, "kubernaut-datastorage:latest", writer)
}

// deployDataStorageOnly deploys Data Storage using pre-built image (separation of build/deploy)
func deployDataStorageOnly(clusterName, kubeconfigPath, imageName string, writer io.Writer) error {
	// Apply database migrations BEFORE deploying Data Storage
	fmt.Fprintln(writer, "  📋 Applying database migrations (shared library)...")
	ctx := context.Background()

	config := DefaultMigrationConfig("kubernaut-system", kubeconfigPath)
	config.Tables = []string{"audit_events", "remediation_workflow_catalog"}
	if err := ApplyMigrationsWithConfig(ctx, config, writer); err != nil {
		fmt.Fprintf(writer, "  ⚠️  Migration warning (may already be applied): %v\n", err)
	}

	verifyConfig := DefaultMigrationConfig("kubernaut-system", kubeconfigPath)
	verifyConfig.Tables = []string{"audit_events"}
	if err := VerifyMigrations(ctx, verifyConfig, writer); err != nil {
		fmt.Fprintf(writer, "  ⚠️  Verification warning: %v\n", err)
	}

	// Load pre-built image into Kind
	// Strip localhost/ prefix if present (loadImageToKind adds it)
	imageNameForKind := strings.TrimPrefix(imageName, "localhost/")
	fmt.Fprintln(writer, "  Loading Data Storage image into Kind...")
	if err := loadImageToKind(clusterName, imageNameForKind, writer); err != nil {
		return fmt.Errorf("failed to load image: %w", err)
	}

	// Deploy manifest (reuse existing logic)
	return deployDataStorageManifest(clusterName, kubeconfigPath, writer)
}

// deployDataStorageManifest deploys the Data Storage manifest (extracted for reuse)
// Note: Assumes image is already built and loaded into Kind
func deployDataStorageManifest(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Deploy manifest with ConfigMap (ADR-030 authoritative pattern)
	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
  namespace: kubernaut-system
data:
  config.yaml: |
    shutdownTimeout: 30s
    server:
      port: 8080
      host: "0.0.0.0"
      read_timeout: 30s
      write_timeout: 30s
    database:
      host: postgresql
      port: 5432
      name: action_history
      user: slm_user
      ssl_mode: disable
      max_open_conns: 25
      max_idle_conns: 5
      conn_max_lifetime: 5m
      conn_max_idle_time: 10m
      secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
      usernameKey: "username"
      passwordKey: "password"
    redis:
      addr: redis:6379
      db: 0
      dlq_stream_name: dlq-stream
      dlq_max_len: 1000
      dlq_consumer_group: dlq-group
      secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
      passwordKey: "password"
    logging:
      level: debug
      format: json
---
apiVersion: v1
kind: Secret
metadata:
  name: datastorage-secret
  namespace: kubernaut-system
stringData:
  db-secrets.yaml: |
    username: slm_user
    password: test_password
  redis-secrets.yaml: |
    password: ""
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: datastorage
  template:
    metadata:
      labels:
        app: datastorage
    spec:
      containers:
      - name: datastorage
        image: localhost/kubernaut-datastorage:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: secrets
          mountPath: /etc/datastorage/secrets
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: secrets
        secret:
          secretName: datastorage-secret
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: datastorage
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30081
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// deployHolmesGPTAPIOnly deploys HolmesGPT-API using pre-built image (separation of build/deploy)
// ADR-030 compliant: Uses ConfigMap for configuration
func deployHolmesGPTAPIOnly(clusterName, kubeconfigPath, imageName string, writer io.Writer) error {
	// Load pre-built image into Kind
	// Strip localhost/ prefix if present (loadImageToKind adds it)
	imageNameForKind := strings.TrimPrefix(imageName, "localhost/")
	fmt.Fprintln(writer, "  Loading HolmesGPT-API image into Kind...")
	if err := loadImageToKind(clusterName, imageNameForKind, writer); err != nil {
		return fmt.Errorf("failed to load HolmesGPT-API image: %w", err)
	}

	// ADR-030: Deploy manifest with ConfigMap
	manifest := fmt.Sprintf(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: holmesgpt-api-config
  namespace: kubernaut-system
data:
  config.yaml: |
    llm:
      provider: "mock"
      model: "mock/test-model"
      endpoint: "http://localhost:11434"
    data_storage:
      url: "http://datastorage:8080"
    logging:
      level: "INFO"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: holmesgpt-api
  template:
    metadata:
      labels:
        app: holmesgpt-api
    spec:
      containers:
      - name: holmesgpt-api
        image: %s
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
        args:
        - "-config"
        - "/etc/holmesgpt/config.yaml"
        env:
        - name: MOCK_LLM_MODE
          value: "true"
        volumeMounts:
        - name: config
          mountPath: /etc/holmesgpt
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: holmesgpt-api-config
---
apiVersion: v1
kind: Service
metadata:
  name: holmesgpt-api
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: holmesgpt-api
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30088
`, imageName)
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// deployAIAnalysisControllerOnly deploys AIAnalysis controller using pre-built image (separation of build/deploy)
func deployAIAnalysisControllerOnly(clusterName, kubeconfigPath, imageName string, writer io.Writer) error {
	// Load pre-built image into Kind
	// Strip localhost/ prefix if present (loadImageToKind adds it)
	imageNameForKind := strings.TrimPrefix(imageName, "localhost/")
	fmt.Fprintln(writer, "  Loading AIAnalysis controller image into Kind...")
	if err := loadImageToKind(clusterName, imageNameForKind, writer); err != nil {
		return fmt.Errorf("failed to load AIAnalysis image: %w", err)
	}

	// Deploy controller with RBAC (extracted from deployAIAnalysisController)
	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: aianalysis-config
  namespace: kubernaut-system
data:
  config.yaml: |
    server:
      port: 8080
      host: "0.0.0.0"
      read_timeout: "30s"
      write_timeout: "30s"
    logging:
      level: "info"
      format: "json"
    holmesgpt:
      url: "http://holmesgpt-api:8080"
      timeout: "60s"
    datastorage:
      url: "http://datastorage:8080"
      timeout: "60s"
    rego:
      policy_path: "/etc/aianalysis/policies/approval.rego"
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: aianalysis-controller
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["aianalyses"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["aianalyses/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: aianalysis-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: aianalysis-controller
subjects:
- kind: ServiceAccount
  name: aianalysis-controller
  namespace: kubernaut-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: aianalysis-controller
  template:
    metadata:
      labels:
        app: aianalysis-controller
    spec:
      serviceAccountName: aianalysis-controller
      containers:
      - name: aianalysis
        image: localhost/kubernaut-aianalysis:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
        - containerPort: 9090
        - containerPort: 8081
        readinessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 3
        env:
        - name: CONFIG_PATH
          value: /etc/aianalysis/config.yaml
        - name: REGO_POLICY_PATH
          value: /etc/aianalysis/policies/approval.rego
        - name: HOLMESGPT_API_URL
          value: http://holmesgpt-api:8080
        - name: DATASTORAGE_URL
          value: http://datastorage:8080
        volumeMounts:
        - name: config
          mountPath: /etc/aianalysis
          readOnly: true
        - name: rego-policies
          mountPath: /etc/aianalysis/policies
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: aianalysis-config
      - name: rego-policies
        configMap:
          name: aianalysis-policies
---
apiVersion: v1
kind: Service
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: aianalysis-controller
  ports:
  - name: api
    port: 8080
    targetPort: 8080
    nodePort: 30084
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30184
  - name: health
    port: 8081
    targetPort: 8081
    nodePort: 30284
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return err
	}

	// Deploy Rego policy ConfigMap
	return deployRegoPolicyConfigMap(kubeconfigPath, writer)
}

func deployHolmesGPTAPI(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Get project root for build context
	projectRoot := getProjectRoot()

	// Build HolmesGPT-API image
	fmt.Fprintln(writer, "  Building HolmesGPT-API image...")
	// NOTE: This takes 10-15 minutes due to Python dependencies (UBI9 + pip packages)
	// If timeout occurs, increase Makefile timeout (currently 30m, was 20m)
	// Try podman build first (macOS)
	fmt.Fprintln(writer, "  (Expected: 10-15 min for Python deps installation)")
	// FIX: Use localhost/ prefix and --no-cache to ensure fresh build
	buildCmd := exec.Command("podman", "build",
		"--no-cache", // Always build fresh for E2E tests
		"-t", "localhost/kubernaut-holmesgpt-api:latest",
		"-f", "holmesgpt-api/Dockerfile", ".")
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build HolmesGPT-API with podman: %w", err)
	}

	// Load into Kind
	fmt.Fprintln(writer, "  Loading HolmesGPT-API image into Kind...")
	if err := loadImageToKind(clusterName, "kubernaut-holmesgpt-api:latest", writer); err != nil {
		return fmt.Errorf("failed to load HolmesGPT-API image: %w", err)
	}

	// ADR-030: Deploy manifest with ConfigMap
	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: holmesgpt-api-config
  namespace: kubernaut-system
data:
  config.yaml: |
    llm:
      provider: "mock"
      model: "mock/test-model"
      endpoint: "http://localhost:11434"
    data_storage:
      url: "http://datastorage:8080"
    logging:
      level: "INFO"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: holmesgpt-api
  template:
    metadata:
      labels:
        app: holmesgpt-api
    spec:
      containers:
      - name: holmesgpt-api
        image: localhost/kubernaut-holmesgpt-api:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
        args:
        - "-config"
        - "/etc/holmesgpt/config.yaml"
        env:
        - name: MOCK_LLM_MODE
          value: "true"
        volumeMounts:
        - name: config
          mountPath: /etc/holmesgpt
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: holmesgpt-api-config
---
apiVersion: v1
kind: Service
metadata:
  name: holmesgpt-api
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: holmesgpt-api
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30088
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func deployAIAnalysisController(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Get project root for build context
	projectRoot := getProjectRoot()

	// Build AIAnalysis controller image
	fmt.Fprintln(writer, "  Building AIAnalysis controller image...")
	// FIX: Use localhost/ prefix and --no-cache to ensure fresh build
	buildCmd := exec.Command("podman", "build",
		"--no-cache", // Always build fresh for E2E tests
		"-t", "localhost/kubernaut-aianalysis:latest",
		"-f", "docker/aianalysis.Dockerfile", ".")
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build AIAnalysis controller with podman: %w", err)
	}

	// Load into Kind
	fmt.Fprintln(writer, "  Loading AIAnalysis image into Kind...")
	if err := loadImageToKind(clusterName, "kubernaut-aianalysis:latest", writer); err != nil {
		return fmt.Errorf("failed to load AIAnalysis image: %w", err)
	}

	// Deploy controller with RBAC
	manifest := `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: aianalysis-controller
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["aianalyses", "aianalyses/status", "aianalyses/finalizers"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: aianalysis-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: aianalysis-controller
subjects:
- kind: ServiceAccount
  name: aianalysis-controller
  namespace: kubernaut-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: aianalysis-controller
  template:
    metadata:
      labels:
        app: aianalysis-controller
    spec:
      serviceAccountName: aianalysis-controller
      containers:
      - name: controller
        image: localhost/kubernaut-aianalysis:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 8081
          name: health
        - containerPort: 9090
          name: metrics
        env:
        - name: HOLMESGPT_API_URL
          value: http://holmesgpt-api:8080
        - name: DATASTORAGE_URL
          value: http://datastorage:8080
        - name: REGO_POLICY_PATH
          value: /etc/rego/approval.rego
        - name: GOCOVERDIR
          value: /coverdata
        volumeMounts:
        - name: rego-policies
          mountPath: /etc/rego
        - name: coverdata
          mountPath: /coverdata
      volumes:
      - name: rego-policies
        configMap:
          name: aianalysis-policies
      - name: coverdata
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: aianalysis-controller
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: aianalysis-controller
  ports:
  - name: health
    port: 8081
    targetPort: 8081
    nodePort: 30284
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30184
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return err
	}

	// Deploy Rego policy ConfigMap
	return deployRegoPolicyConfigMap(kubeconfigPath, writer)
}

func deployRegoPolicyConfigMap(kubeconfigPath string, writer io.Writer) error {
	policyPath := findRegoPolicy()
	if policyPath == "" {
		// Use inline policy
		return createInlineRegoPolicyConfigMap(kubeconfigPath, writer)
	}

	// Create ConfigMap from file
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"create", "configmap", "aianalysis-policies",
		"--from-file=approval.rego="+policyPath,
		"-n", "kubernaut-system",
		"--dry-run=client", "-o", "yaml")

	applyCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")

	// Use io.Pipe to connect stdout of cmd to stdin of applyCmd
	pipeReader2, pipeWriter2 := io.Pipe()
	cmd.Stdout = pipeWriter2
	applyCmd.Stdin = pipeReader2
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := cmd.Start(); err != nil {
		return err
	}
	if err := applyCmd.Start(); err != nil {
		return err
	}
	if err := cmd.Wait(); err != nil {
		return err
	}
	pipeWriter2.Close()
	return applyCmd.Wait()
}

func findRegoPolicy() string {
	// Try to find via runtime caller location first
	_, currentFile, _, ok := runtime.Caller(0)
	if ok {
		// Go up to project root (from test/infrastructure/)
		projectRoot := filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
		policyPath := filepath.Join(projectRoot, "config/rego/aianalysis/approval.rego")
		if _, err := os.Stat(policyPath); err == nil {
			return policyPath
		}
	}

	candidates := []string{
		"config/rego/aianalysis/approval.rego",
		"../config/rego/aianalysis/approval.rego",
		"../../config/rego/aianalysis/approval.rego",
		"../../../config/rego/aianalysis/approval.rego",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}
	return ""
}

// loadImageToKind loads a container image into the Kind cluster
// Handles both Podman (localhost/ prefix) and Docker image naming
func loadImageToKind(clusterName, imageName string, writer io.Writer) error {
	// For Podman on macOS, kind load docker-image doesn't work well
	// Use podman save + kind load image-archive as workaround

	// Create temp file for image archive
	tmpFile, err := os.CreateTemp("", "kind-image-*.tar")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Save image with podman (imageName already includes localhost/ prefix)
	fmt.Fprintf(writer, "  Exporting image %s...\n", imageName)
	saveCmd := exec.Command("podman", "save", "-o", tmpFile.Name(), imageName)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image %s with podman: %w", imageName, err)
	}

	// Load image archive into Kind
	fmt.Fprintf(writer, "  Loading image archive into Kind...\n")
	loadCmd := exec.Command("kind", "load", "image-archive", tmpFile.Name(), "--name", clusterName)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image %s: %w", imageName, err)
	}

	return nil
}

// getProjectRoot returns the absolute path to the project root directory
func getProjectRoot() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if ok {
		// Go up from test/infrastructure/ to project root
		return filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
	}

	// Fallback: try to find go.mod
	candidates := []string{".", "..", "../..", "../../.."}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			absPath, _ := filepath.Abs(dir)
			return absPath
		}
	}
	return "."
}

func createInlineRegoPolicyConfigMap(kubeconfigPath string, writer io.Writer) error {
	// Simplified E2E test policy - requires approval for all production
	// This is intentionally simpler than production policy for E2E test predictability
	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: aianalysis-policies
  namespace: kubernaut-system
data:
  approval.rego: |
    package aianalysis.approval
    import rego.v1

    # Default values
    default require_approval := false
    default reason := "Auto-approved"

    # Production environment always requires approval (E2E test simplification)
    require_approval if {
        input.environment == "production"
    }

    # Multiple recovery attempts require approval (any environment)
    require_approval if {
        input.is_recovery_attempt == true
        input.recovery_attempt_number >= 3
    }

    # Data quality issues in production require approval (BR-AI-011)
    # Check both warnings (from HAPI) and failed_detections (from SignalProcessing)
    require_approval if {
        input.environment == "production"
        count(input.warnings) > 0
    }

    require_approval if {
        input.environment == "production"
        count(input.failed_detections) > 0
    }

    # Reason determination (single rule to avoid eval_conflict_error)
    reason := msg if {
        require_approval
        input.is_recovery_attempt == true
        input.recovery_attempt_number >= 3
        msg := sprintf("Multiple recovery attempts (%d) - human approval required", [input.recovery_attempt_number])
    }

    reason := "Data quality warnings in production environment" if {
        require_approval
        input.environment == "production"
        count(input.warnings) > 0
        not input.is_recovery_attempt
    }

    reason := "Data quality issues detected in production environment" if {
        require_approval
        input.environment == "production"
        count(input.failed_detections) > 0
        count(input.warnings) == 0
        not input.is_recovery_attempt
    }

    reason := "Production environment requires manual approval" if {
        require_approval
        input.environment == "production"
        count(input.warnings) == 0
        not input.is_recovery_attempt
    }

    reason := "Production environment requires manual approval" if {
        require_approval
        input.environment == "production"
        input.is_recovery_attempt == true
        input.recovery_attempt_number < 3
    }
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// NOTE: findManifest function removed - no longer needed with shared deployment functions

// stringReader creates an io.Reader from a string
type stringReaderImpl struct {
	data string
	pos  int
}

func stringReader(s string) io.Reader {
	return &stringReaderImpl{data: s}
}

func (r *stringReaderImpl) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HAPI Container Infrastructure (for Integration Tests)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Per NOTICE_INTEGRATION_TEST_INFRASTRUCTURE_OWNERSHIP.md:
// - AIAnalysis integration tests connect to HAPI via HTTP
// - HAPI is started programmatically (not via shared podman-compose)
// - HAPI requires Data Storage HTTP API to be running (owned by DS team)
//
// Port Allocation (per DD-TEST-001):
//   HAPI: Default 18081 (can be overridden for parallel tests)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	// DefaultHAPIPort is the default port for HAPI in integration tests
	DefaultHAPIPort = 18081

	// HAPIImageName is the image name for HAPI container
	HAPIImageName = "kubernaut-holmesgpt-api:test"
)

// AIAnalysis Integration Test Ports (per DD-TEST-001)
const (
	// PostgreSQL port for AIAnalysis integration tests (DD-TEST-001)
	// Changed from 15434 → 15438 to avoid conflict with Effectiveness Monitor
	AIAnalysisIntegrationPostgresPort = 15438

	// Redis port for AIAnalysis integration tests (DD-TEST-001)
	// Changed from 16380 → 16384 to avoid conflict with Gateway
	AIAnalysisIntegrationRedisPort = 16384

	// DataStorage API port for AIAnalysis integration tests (DD-TEST-001)
	// Changed from 18091 → 18095 to avoid conflict with Gateway
	AIAnalysisIntegrationDataStoragePort = 18095

	// HolmesGPT API port for AIAnalysis integration tests (DD-TEST-001)
	// Already correct - no change needed
	AIAnalysisIntegrationHAPIPort = 18120
)

// HAPIContainerConfig holds configuration for starting HAPI container
type HAPIContainerConfig struct {
	// ContainerName is the unique name for the container
	ContainerName string
	// Port is the host port to expose HAPI on (container always uses 8080)
	Port int
	// DataStorageURL is the URL of the Data Storage service
	DataStorageURL string
	// BuildImage if true, builds the HAPI image before starting
	BuildImage bool
}

// StartHAPIContainer starts a HAPI container for integration testing
//
// This starts HolmesGPT-API with MOCK_LLM_MODE=true for deterministic testing
// without incurring LLM costs.
//
// Prerequisites:
// - Data Storage must be running and accessible at DataStorageURL
// - Podman machine must be running
//
// Parameters:
// - config: Container configuration
// - writer: io.Writer for progress output
//
// Returns:
// - int: The actual port HAPI is running on
// - error: Any errors during container creation
func StartHAPIContainer(config HAPIContainerConfig, writer io.Writer) (int, error) {
	if config.ContainerName == "" {
		config.ContainerName = "aianalysis-hapi-integration"
	}
	if config.Port == 0 {
		config.Port = DefaultHAPIPort
	}
	if config.DataStorageURL == "" {
		config.DataStorageURL = "http://host.containers.internal:18090"
	}

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Starting HAPI Container for Integration Tests\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  Container: %s\n", config.ContainerName)
	fmt.Fprintf(writer, "  Port: %d\n", config.Port)
	fmt.Fprintf(writer, "  Data Storage: %s\n", config.DataStorageURL)
	fmt.Fprintf(writer, "  Mock LLM: ENABLED\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Check if port is available
	if !isHAPIPortAvailable(config.Port) {
		return 0, fmt.Errorf("port %d is already in use", config.Port)
	}

	// Check if container already exists and is running
	checkCmd := exec.Command("podman", "ps", "--filter", fmt.Sprintf("name=%s", config.ContainerName), "--format", "{{.Names}}")
	output, _ := checkCmd.CombinedOutput()
	if strings.TrimSpace(string(output)) == config.ContainerName {
		fmt.Fprintf(writer, "✅ HAPI container '%s' already running on port %d\n", config.ContainerName, config.Port)
		return config.Port, nil
	}

	// Check if container exists but stopped
	checkStoppedCmd := exec.Command("podman", "ps", "-a", "--filter", fmt.Sprintf("name=%s", config.ContainerName), "--format", "{{.Names}}")
	stoppedOutput, _ := checkStoppedCmd.CombinedOutput()
	if strings.TrimSpace(string(stoppedOutput)) == config.ContainerName {
		// Container exists but stopped, remove it
		fmt.Fprintf(writer, "🗑️  Removing stopped HAPI container '%s'...\n", config.ContainerName)
		rmCmd := exec.Command("podman", "rm", config.ContainerName)
		_ = rmCmd.Run()
	}

	// Build image if requested
	if config.BuildImage {
		fmt.Fprintf(writer, "🔨 Building HAPI image...\n")
		if err := buildHAPIImage(writer); err != nil {
			return 0, fmt.Errorf("failed to build HAPI image: %w", err)
		}
	}

	// Start container with mock LLM mode
	fmt.Fprintf(writer, "🚀 Starting HAPI container...\n")
	cmd := exec.Command("podman", "run", "-d",
		"--name", config.ContainerName,
		"-p", fmt.Sprintf("%d:8080", config.Port),
		"-e", "MOCK_LLM_MODE=true",
		"-e", fmt.Sprintf("DATASTORAGE_URL=%s", config.DataStorageURL),
		"-e", "LOG_LEVEL=INFO",
		"--add-host", "host.containers.internal:host-gateway",
		HAPIImageName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to start HAPI container: %w, output: %s", err, string(output))
	}

	// Wait for HAPI to be ready
	fmt.Fprintf(writer, "⏳ Waiting for HAPI to be ready...\n")
	if err := waitForHAPIHealth(config.Port, 60*time.Second); err != nil {
		// Get container logs for debugging
		logsCmd := exec.Command("podman", "logs", "--tail", "50", config.ContainerName)
		logs, _ := logsCmd.CombinedOutput()
		return 0, fmt.Errorf("HAPI not ready: %w\nContainer logs:\n%s", err, string(logs))
	}

	fmt.Fprintf(writer, "✅ HAPI container '%s' started on port %d\n", config.ContainerName, config.Port)
	return config.Port, nil
}

// StopHAPIContainer stops and removes a HAPI container
func StopHAPIContainer(containerName string, writer io.Writer) error {
	if containerName == "" {
		containerName = "aianalysis-hapi-integration"
	}

	fmt.Fprintf(writer, "🛑 Stopping HAPI container '%s'...\n", containerName)

	// Check if container exists
	checkCmd := exec.Command("podman", "ps", "-a", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, _ := checkCmd.CombinedOutput()
	if strings.TrimSpace(string(output)) != containerName {
		fmt.Fprintf(writer, "✅ HAPI container '%s' does not exist (already cleaned up)\n", containerName)
		return nil
	}

	// Stop container
	stopCmd := exec.Command("podman", "stop", containerName)
	if err := stopCmd.Run(); err != nil {
		fmt.Fprintf(writer, "⚠️  Warning: Failed to stop HAPI container '%s': %v\n", containerName, err)
	}

	// Remove container
	rmCmd := exec.Command("podman", "rm", containerName)
	if err := rmCmd.Run(); err != nil {
		return fmt.Errorf("failed to remove HAPI container: %w", err)
	}

	fmt.Fprintf(writer, "✅ HAPI container '%s' stopped and removed\n", containerName)
	return nil
}

// buildHAPIImage builds the HAPI Docker image
func buildHAPIImage(writer io.Writer) error {
	projectRoot := getProjectRoot()

	cmd := exec.Command("podman", "build",
		"-t", HAPIImageName,
		"-f", "holmesgpt-api/Dockerfile",
		".",
	)
	cmd.Dir = projectRoot
	cmd.Stdout = writer
	cmd.Stderr = writer

	return cmd.Run()
}

// waitForHAPIHealth waits for HAPI health endpoint to respond
func waitForHAPIHealth(port int, timeout time.Duration) error {
	healthURL := fmt.Sprintf("http://localhost:%d/health", port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for HAPI health at %s", healthURL)
}

// isHAPIPortAvailable checks if a TCP port is available for binding
func isHAPIPortAvailable(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// AIAnalysis Integration Test Infrastructure (Programmatic Podman Setup)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Per DD-TEST-002: Integration Test Container Orchestration Pattern
// - Programmatic `podman run` commands (not shell scripts)
// - Sequential startup with explicit health checks
// - Parallel-safe (via SynchronizedBeforeSuite)
//
// Migration History:
// - Before Dec 26, 2025: Used podman-compose (race conditions)
// - Dec 26, 2025: Migrated to DD-TEST-002 sequential startup using shared utilities
//
// Related:
// - aianalysis.go (E2E): Kind cluster infrastructure
// - shared_integration_utils.go: Common Podman utilities
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	AIAnalysisIntegrationPostgresContainer    = "aianalysis_postgres_1"
	AIAnalysisIntegrationRedisContainer       = "aianalysis_redis_1"
	AIAnalysisIntegrationDataStorageContainer = "aianalysis_datastorage_1"
	AIAnalysisIntegrationHAPIContainer        = "aianalysis_hapi_1"
	AIAnalysisIntegrationMigrationsContainer  = "aianalysis_migrations"
	AIAnalysisIntegrationNetwork              = "aianalysis_test-network"
)

const (
	AIAnalysisIntegrationDBName     = "action_history"
	AIAnalysisIntegrationDBUser     = "slm_user"
	AIAnalysisIntegrationDBPassword = "test_password"
)

// StartAIAnalysisIntegrationInfrastructure starts the full Podman stack for AIAnalysis integration tests
// This includes: PostgreSQL, Redis, DataStorage API, and HolmesGPT API
//
// Pattern: DD-TEST-002 Sequential Startup Pattern (using shared utilities)
// - Programmatic `podman run` commands
// - Explicit health checks after each service
// - Parallel-safe (called from SynchronizedBeforeSuite)
//
// Prerequisites:
// - podman must be installed
// - Ports 15434, 16380, 18091, 18120 must be available
//
// Returns:
// - error: Any errors during infrastructure startup
func StartAIAnalysisIntegrationInfrastructure(writer io.Writer) error {
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Starting AIAnalysis Integration Test Infrastructure\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", AIAnalysisIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:          localhost:%d\n", AIAnalysisIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", AIAnalysisIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  HolmesGPT API:  http://localhost:%d\n", AIAnalysisIntegrationHAPIPort)
	fmt.Fprintf(writer, "  Pattern:        DD-TEST-002 Sequential Startup (Programmatic Go)\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	projectRoot := getProjectRoot()

	// ============================================================================
	// STEP 1: Cleanup existing containers and network
	// ============================================================================
	CleanupContainers([]string{
		AIAnalysisIntegrationPostgresContainer,
		AIAnalysisIntegrationRedisContainer,
		AIAnalysisIntegrationDataStorageContainer,
		AIAnalysisIntegrationHAPIContainer,
		AIAnalysisIntegrationMigrationsContainer,
	}, writer)
	_ = exec.Command("podman", "network", "rm", AIAnalysisIntegrationNetwork).Run() // Ignore errors
	fmt.Fprintf(writer, "   ✅ Cleanup complete\n\n")

	// ============================================================================
	// STEP 2: Create custom network for internal communication
	// ============================================================================
	fmt.Fprintf(writer, "🌐 Creating custom Podman network '%s'...\n", AIAnalysisIntegrationNetwork)
	createNetworkCmd := exec.Command("podman", "network", "create", AIAnalysisIntegrationNetwork)
	createNetworkCmd.Stdout = writer
	createNetworkCmd.Stderr = writer
	if err := createNetworkCmd.Run(); err != nil {
		// Ignore if network already exists
		if !strings.Contains(err.Error(), "already exists") {
			return fmt.Errorf("failed to create network '%s': %w", AIAnalysisIntegrationNetwork, err)
		}
		fmt.Fprintf(writer, "  (Network '%s' already exists, continuing...)\n", AIAnalysisIntegrationNetwork)
	}
	fmt.Fprintf(writer, "   ✅ Network '%s' created/ensured\n\n", AIAnalysisIntegrationNetwork)

	// ============================================================================
	// STEP 3: Start PostgreSQL FIRST (DD-TEST-002 Sequential Pattern)
	// ============================================================================
	pgConfig := PostgreSQLConfig{
		ContainerName:  AIAnalysisIntegrationPostgresContainer,
		Port:           AIAnalysisIntegrationPostgresPort,
		DBName:         AIAnalysisIntegrationDBName,
		DBUser:         AIAnalysisIntegrationDBUser,
		DBPassword:     AIAnalysisIntegrationDBPassword,
		Network:        AIAnalysisIntegrationNetwork,
		MaxConnections: 200,
	}
	if err := StartPostgreSQL(pgConfig, writer); err != nil {
		return fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// CRITICAL: Wait for PostgreSQL to be ready before proceeding
	if err := WaitForPostgreSQLReady(AIAnalysisIntegrationPostgresContainer, AIAnalysisIntegrationDBUser, AIAnalysisIntegrationDBName, writer); err != nil {
		return fmt.Errorf("PostgreSQL failed to become ready: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ PostgreSQL ready\n\n")

	// ============================================================================
	// STEP 4: Run migrations (inline approach - same as RO)
	// ============================================================================
	fmt.Fprintf(writer, "🔄 Running migrations...\n")
	migrationsCmd := exec.Command("podman", "run", "--rm",
		"--network", AIAnalysisIntegrationNetwork,
		"-e", "PGHOST="+AIAnalysisIntegrationPostgresContainer,
		"-e", "PGPORT=5432",
		"-e", "PGUSER="+AIAnalysisIntegrationDBUser,
		"-e", "PGPASSWORD="+AIAnalysisIntegrationDBPassword,
		"-e", "PGDATABASE="+AIAnalysisIntegrationDBName,
		"-v", filepath.Join(projectRoot, "migrations")+":/migrations:ro",
		"postgres:16-alpine",
		"sh", "-c",
		`set -e
echo "Applying migrations (Up sections only)..."
find /migrations -maxdepth 1 -name '*.sql' -type f | sort | while read f; do
  echo "Applying $f..."
  sed -n '1,/^-- +goose Down/p' "$f" | grep -v '^-- +goose Down' | psql
done
echo "Migrations complete!"`)
	migrationsCmd.Stdout = writer
	migrationsCmd.Stderr = writer
	if err := migrationsCmd.Run(); err != nil {
		return fmt.Errorf("migrations failed: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ Migrations applied successfully\n\n")

	// ============================================================================
	// STEP 5: Start Redis
	// ============================================================================
	redisConfig := RedisConfig{
		ContainerName: AIAnalysisIntegrationRedisContainer,
		Port:          AIAnalysisIntegrationRedisPort,
		Network:       AIAnalysisIntegrationNetwork,
	}
	if err := StartRedis(redisConfig, writer); err != nil {
		return fmt.Errorf("failed to start Redis: %w", err)
	}

	// Wait for Redis to be ready
	if err := WaitForRedisReady(AIAnalysisIntegrationRedisContainer, writer); err != nil {
		return fmt.Errorf("Redis failed to become ready: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ Redis ready\n\n")

	// ============================================================================
	// STEP 6: Start DataStorage (using shared utility)
	// ============================================================================
	fmt.Fprintf(writer, "📦 Starting DataStorage service...\n")

	// Generate composite image tag per DD-INTEGRATION-001 v2.0
	// Format: localhost/datastorage:aianalysis-{uuid}
	dsImageTag := GenerateInfraImageName("datastorage", "aianalysis")
	fmt.Fprintf(writer, "   Using image tag: %s\n", dsImageTag)

	if err := StartDataStorage(IntegrationDataStorageConfig{
		ContainerName: AIAnalysisIntegrationDataStorageContainer,
		Port:          AIAnalysisIntegrationDataStoragePort,
		Network:       AIAnalysisIntegrationNetwork,
		PostgresHost:  AIAnalysisIntegrationPostgresContainer, // Use container name for internal network
		PostgresPort:  5432,                                   // Internal port
		DBName:        AIAnalysisIntegrationDBName,
		DBUser:        AIAnalysisIntegrationDBUser,
		DBPassword:    AIAnalysisIntegrationDBPassword,
		RedisHost:     AIAnalysisIntegrationRedisContainer, // Use container name for internal network
		RedisPort:     6379,                                // Internal port
		LogLevel:      "info",
		ImageTag:      dsImageTag, // DD-INTEGRATION-001 v2.0: Composite tag for collision avoidance
	}, writer); err != nil {
		return fmt.Errorf("failed to start DataStorage: %w", err)
	}

	// CRITICAL: Wait for DataStorage HTTP endpoint to be ready
	if err := WaitForHTTPHealth(
		fmt.Sprintf("http://localhost:%d/health", AIAnalysisIntegrationDataStoragePort),
		60*time.Second,
		writer,
	); err != nil {
		// Print container logs for debugging
		fmt.Fprintf(writer, "\n⚠️  DataStorage failed to become healthy. Container logs:\n")
		logsCmd := exec.Command("podman", "logs", AIAnalysisIntegrationDataStorageContainer)
		logsCmd.Stdout = writer
		logsCmd.Stderr = writer
		_ = logsCmd.Run()
		return fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ DataStorage ready\n\n")

	// ============================================================================
	// STEP 7: Start HolmesGPT API (HAPI)
	// ============================================================================
	fmt.Fprintf(writer, "🤖 Starting HolmesGPT API service...\n")
	// Build the HAPI image with DD-INTEGRATION-001 compliant tag
	hapiImage := GenerateInfraImageName("holmesgpt-api", "aianalysis")
	hapiBuildCmd := exec.Command("podman", "build", "-t", hapiImage,
		"-f", filepath.Join(projectRoot, "holmesgpt-api", "Dockerfile"),
		projectRoot, // Build context is the project root
	)
	hapiBuildCmd.Stdout = writer
	hapiBuildCmd.Stderr = writer
	if err := hapiBuildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build HolmesGPT API image: %w", err)
	}

	// ADR-030: Create minimal HAPI config file for integration tests
	hapiConfigDir := filepath.Join(projectRoot, "test", "integration", "aianalysis", "hapi-config")
	os.MkdirAll(hapiConfigDir, 0755)

	hapiConfig := GetMinimalHAPIConfig(
		"http://"+AIAnalysisIntegrationDataStorageContainer+":8080",
		"INFO",
	)
	os.WriteFile(filepath.Join(hapiConfigDir, "config.yaml"), []byte(hapiConfig), 0644)

	// Start HAPI container
	// ADR-030: Use -config flag (consistent with Go services)
	hapiCmd := exec.Command("podman", "run", "-d",
		"--name", AIAnalysisIntegrationHAPIContainer,
		"--network", AIAnalysisIntegrationNetwork,
		"-p", fmt.Sprintf("%d:8080", AIAnalysisIntegrationHAPIPort), // Map host port to container's 8080
		"-v", fmt.Sprintf("%s:/etc/holmesgpt:ro", hapiConfigDir),
		"-e", "MOCK_LLM_MODE=true",
		hapiImage,
		"-config", "/etc/holmesgpt/config.yaml", // ADR-030: Use -config flag (like Go services)
	)
	hapiCmd.Stdout = writer
	hapiCmd.Stderr = writer
	if err := hapiCmd.Run(); err != nil {
		return fmt.Errorf("failed to start HolmesGPT API service: %w", err)
	}

	// CRITICAL: Wait for HAPI HTTP endpoint to be ready
	if err := WaitForHTTPHealth(
		fmt.Sprintf("http://localhost:%d/health", AIAnalysisIntegrationHAPIPort),
		60*time.Second,
		writer,
	); err != nil {
		// Print container logs for debugging
		fmt.Fprintf(writer, "\n⚠️  HolmesGPT API failed to become healthy. Container logs:\n")
		logsCmd := exec.Command("podman", "logs", AIAnalysisIntegrationHAPIContainer)
		logsCmd.Stdout = writer
		logsCmd.Stderr = writer
		_ = logsCmd.Run()
		return fmt.Errorf("HolmesGPT API failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "   ✅ HolmesGPT API ready\n\n")

	// ============================================================================
	// SUCCESS
	// ============================================================================
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "✅ AIAnalysis Integration Infrastructure Ready\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:        localhost:%d\n", AIAnalysisIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:             localhost:%d\n", AIAnalysisIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage HTTP:  http://localhost:%d\n", AIAnalysisIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  HolmesGPT API:     http://localhost:%d\n", AIAnalysisIntegrationHAPIPort)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// StopAIAnalysisIntegrationInfrastructure stops and cleans up the AIAnalysis integration test infrastructure
//
// Pattern: DD-TEST-002 Sequential Cleanup
// - Stop containers in reverse order
// - Remove containers and network
// - Parallel-safe (called from SynchronizedAfterSuite)
//
// Returns:
// - error: Any errors during infrastructure cleanup
func StopAIAnalysisIntegrationInfrastructure(writer io.Writer) error {
	fmt.Fprintf(writer, "🛑 Stopping AIAnalysis Integration Infrastructure...\n")

	containers := []string{
		AIAnalysisIntegrationHAPIContainer,
		AIAnalysisIntegrationDataStorageContainer,
		AIAnalysisIntegrationRedisContainer,
		AIAnalysisIntegrationPostgresContainer,
		AIAnalysisIntegrationMigrationsContainer,
	}
	CleanupContainers(containers, writer)

	// Remove network
	fmt.Fprintf(writer, "Removing network '%s'...\n", AIAnalysisIntegrationNetwork)
	networkCmd := exec.Command("podman", "network", "rm", AIAnalysisIntegrationNetwork)
	_ = networkCmd.Run() // Ignore errors, network may not exist
	fmt.Fprintf(writer, "✅ Network '%s' removed\n", AIAnalysisIntegrationNetwork)

	fmt.Fprintf(writer, "✅ AIAnalysis Integration Infrastructure stopped and cleaned up\n")
	return nil
}

// waitForAIAnalysisInfraReady waits for PostgreSQL and Redis to be ready
// This ensures Data Storage can successfully connect when it starts
// Pattern adapted from waitForDataStorageServicesReady in datastorage.go
func waitForAIAnalysisInfraReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// Build Kubernetes clientset
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// Wait for PostgreSQL pod to be ready
	fmt.Fprintf(writer, "   ⏳ Waiting for PostgreSQL pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=postgresql",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 3*time.Minute, 5*time.Second).Should(BeTrue(), "PostgreSQL pod should become ready")
	fmt.Fprintf(writer, "   ✅ PostgreSQL ready\n")

	// Wait for Redis pod to be ready
	fmt.Fprintf(writer, "   ⏳ Waiting for Redis pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=redis",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Redis pod should become ready")
	fmt.Fprintf(writer, "   ✅ Redis ready\n")

	return nil
}

// waitForAllServicesReady waits for DataStorage, HolmesGPT-API, and AIAnalysis pods to be ready
// This ensures infrastructure setup doesn't return until all services can handle requests
// Pattern: Same as waitForAIAnalysisInfraReady but for all 3 application pods
func waitForAllServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// Build Kubernetes clientset
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// Wait for DataStorage pod to be ready
	fmt.Fprintf(writer, "   ⏳ Waiting for DataStorage pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=datastorage",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "DataStorage pod should become ready")
	fmt.Fprintf(writer, "   ✅ DataStorage ready\n")

	// Wait for HolmesGPT-API pod to be ready
	fmt.Fprintf(writer, "   ⏳ Waiting for HolmesGPT-API pod to be ready...\n")

	// Track polling attempts for debugging
	pollCount := 0
	maxPolls := int((2 * time.Minute) / (5 * time.Second)) // 24 polls expected

	Eventually(func() bool {
		pollCount++
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=holmesgpt-api",
		})
		if err != nil {
			fmt.Fprintf(writer, "      [Poll %d/%d] Error listing HAPI pods: %v\n", pollCount, maxPolls, err)
			return false
		}
		if len(pods.Items) == 0 {
			fmt.Fprintf(writer, "      [Poll %d/%d] No HAPI pods found\n", pollCount, maxPolls)
			return false
		}

		// Debug: Show pod status every 4 polls (~20 seconds)
		for _, pod := range pods.Items {
			if pollCount%4 == 0 {
				fmt.Fprintf(writer, "      [Poll %d/%d] HAPI pod '%s': Phase=%s, Ready=",
					pollCount, maxPolls, pod.Name, pod.Status.Phase)
				isReady := false
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady {
						fmt.Fprintf(writer, "%s", condition.Status)
						if condition.Status != corev1.ConditionTrue {
							fmt.Fprintf(writer, " (Reason: %s, Message: %s)", condition.Reason, condition.Message)
						}
						isReady = condition.Status == corev1.ConditionTrue
						break
					}
				}
				if !isReady {
					// Show container statuses for debugging
					for _, containerStatus := range pod.Status.ContainerStatuses {
						if !containerStatus.Ready {
							fmt.Fprintf(writer, "\n         Container '%s': Ready=%t, RestartCount=%d",
								containerStatus.Name, containerStatus.Ready, containerStatus.RestartCount)
							if containerStatus.State.Waiting != nil {
								fmt.Fprintf(writer, ", Waiting: %s (%s)",
									containerStatus.State.Waiting.Reason, containerStatus.State.Waiting.Message)
							}
							if containerStatus.State.Terminated != nil {
								fmt.Fprintf(writer, ", Terminated: ExitCode=%d, Reason=%s",
									containerStatus.State.Terminated.ExitCode, containerStatus.State.Terminated.Reason)
							}
						}
					}
				}
				fmt.Fprintf(writer, "\n")
			}

			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "HolmesGPT-API pod should become ready")
	fmt.Fprintf(writer, "   ✅ HolmesGPT-API ready\n")

	// Wait for AIAnalysis controller pod to be ready
	// Note: Coverage-instrumented binaries may take longer to start
	fmt.Fprintf(writer, "   ⏳ Waiting for AIAnalysis controller pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=aianalysis-controller",
		})
		if err != nil || len(pods.Items) == 0 {
			return false
		}
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
						return true
					}
				}
			}
		}
		return false
	}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "AIAnalysis controller pod should become ready")
	fmt.Fprintf(writer, "   ✅ AIAnalysis controller ready\n")

	return nil
}

// CreateAIAnalysisClusterHybrid implements DD-TEST-002 Hybrid Parallel Setup:
// PHASE 1: Build images in parallel (FIRST - prevents cluster timeout)
// PHASE 2: Create Kind cluster AFTER builds complete
// PHASE 3: Load images into fresh cluster
// PHASE 4: Deploy services
//
// This matches the authoritative Gateway E2E implementation pattern.
// Per DD-TEST-002: Build parallel prevents cluster timeout issues.
func CreateAIAnalysisClusterHybrid(clusterName, kubeconfigPath string, writer io.Writer) error {
	ctx := context.Background()
	namespace := "kubernaut-system" // Infrastructure always in kubernaut-system; tests use dynamic namespaces

	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "🚀 AIAnalysis E2E Infrastructure (HYBRID PARALLEL)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "  Strategy: Build parallel → Create cluster → Load → Deploy")
	fmt.Fprintln(writer, "  Benefits: Fast builds + No cluster timeout + Reliable")
	fmt.Fprintln(writer, "  Per DD-TEST-002: Hybrid Parallel Setup Standard")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL (before cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	fmt.Fprintln(writer, "  ├── Data Storage (1-2 min)")
	fmt.Fprintln(writer, "  ├── HolmesGPT-API (2-3 min)")
	fmt.Fprintln(writer, "  └── AIAnalysis controller (3-4 min)")

	type imageBuildResult struct {
		name  string
		image string
		err   error
	}

	buildResults := make(chan imageBuildResult, 3)
	projectRoot := getProjectRoot()

	// Generate unique infrastructure image tags per DD-TEST-001 v1.3
	dataStorageImage := GenerateInfraImageName("datastorage", "aianalysis")
	hapiImage := GenerateInfraImageName("holmesgpt-api", "aianalysis")

	go func() {
		err := buildImageOnly("Data Storage", dataStorageImage,
			"docker/data-storage.Dockerfile", projectRoot, writer)
		buildResults <- imageBuildResult{"datastorage", dataStorageImage, err}
	}()

	go func() {
		err := buildImageOnly("HolmesGPT-API", hapiImage,
			"holmesgpt-api/Dockerfile", projectRoot, writer)
		buildResults <- imageBuildResult{"holmesgpt-api", hapiImage, err}
	}()

	go func() {
		var err error
		if os.Getenv("E2E_COVERAGE") == "true" {
			fmt.Fprintf(writer, "   📊 Building AIAnalysis with coverage (GOFLAGS=-cover)\n")
			buildArgs := []string{"--build-arg", "GOFLAGS=-cover"}
			err = buildImageWithArgs("AIAnalysis controller", "localhost/kubernaut-aianalysis:latest",
				"docker/aianalysis.Dockerfile", projectRoot, buildArgs, writer)
		} else {
			err = buildImageOnly("AIAnalysis controller", "localhost/kubernaut-aianalysis:latest",
				"docker/aianalysis.Dockerfile", projectRoot, writer)
		}
		buildResults <- imageBuildResult{"aianalysis", "localhost/kubernaut-aianalysis:latest", err}
	}()

	builtImages := make(map[string]string)
	for i := 0; i < 3; i++ {
		result := <-buildResults
		if result.err != nil {
			return fmt.Errorf("failed to build %s image: %w", result.name, result.err)
		}
		builtImages[result.name] = result.image
		fmt.Fprintf(writer, "  ✅ %s image built\n", result.name)
	}
	fmt.Fprintln(writer, "\n✅ All images built! (~3-4 min parallel)")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster (AFTER builds complete)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster...")
	if err := createAIAnalysisKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	fmt.Fprintln(writer, "📁 Creating namespace...")
	createNsCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"create", "namespace", namespace)
	nsOutput := &strings.Builder{}
	createNsCmd.Stdout = io.MultiWriter(writer, nsOutput)
	createNsCmd.Stderr = io.MultiWriter(writer, nsOutput)
	if err := createNsCmd.Run(); err != nil {
		if !strings.Contains(nsOutput.String(), "AlreadyExists") {
			return fmt.Errorf("failed to create namespace: %w", err)
		}
	}

	fmt.Fprintln(writer, "📋 Installing AIAnalysis CRD...")
	if err := installAIAnalysisCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install AIAnalysis CRD: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images (parallel)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into cluster...")
	type imageLoadResult struct {
		name string
		err  error
	}
	loadResults := make(chan imageLoadResult, 3)

	go func() {
		err := loadImageToKind(clusterName, builtImages["datastorage"], writer)
		loadResults <- imageLoadResult{"DataStorage", err}
	}()
	go func() {
		err := loadImageToKind(clusterName, builtImages["holmesgpt-api"], writer)
		loadResults <- imageLoadResult{"HolmesGPT-API", err}
	}()
	go func() {
		err := loadImageToKind(clusterName, builtImages["aianalysis"], writer)
		loadResults <- imageLoadResult{"AIAnalysis", err}
	}()

	for i := 0; i < 3; i++ {
		result := <-loadResults
		if result.err != nil {
			return fmt.Errorf("failed to load %s: %w", result.name, result.err)
		}
		fmt.Fprintf(writer, "  ✅ %s loaded\n", result.name)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy services IN PARALLEL (let Kubernetes reconcile)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying services in parallel...")
	fmt.Fprintln(writer, "  ├── Data Storage infrastructure (PostgreSQL + Redis + DataStorage + Migrations)")
	fmt.Fprintln(writer, "  ├── HolmesGPT-API")
	fmt.Fprintln(writer, "  └── AIAnalysis controller")
	fmt.Fprintln(writer, "  ⏱️  Kubernetes will handle dependencies via readiness probes")

	type deployResult struct {
		name string
		err  error
	}

	deployResults := make(chan deployResult, 3)

	// Deploy Data Storage infrastructure using shared function (DD-TEST-001 v1.3)
	go func() {
		err := DeployDataStorageTestServices(ctx, namespace, kubeconfigPath, builtImages["datastorage"], writer)
		deployResults <- deployResult{"DataStorage Infrastructure", err}
	}()

	// Deploy HAPI (AIAnalysis dependency)
	go func() {
		err := deployHolmesGPTAPIOnly(clusterName, kubeconfigPath, builtImages["holmesgpt-api"], writer)
		deployResults <- deployResult{"HolmesGPT-API", err}
	}()

	// Deploy AIAnalysis controller (service under test)
	go func() {
		err := deployAIAnalysisControllerOnly(clusterName, kubeconfigPath, builtImages["aianalysis"], writer)
		deployResults <- deployResult{"AIAnalysis", err}
	}()

	// Collect deployment results (kubectl apply results)
	fmt.Fprintln(writer, "\n⏳ Waiting for manifest applications...")
	for i := 0; i < 3; i++ {
		result := <-deployResults
		if result.err != nil {
			return fmt.Errorf("failed to deploy %s: %w", result.name, result.err)
		}
		fmt.Fprintf(writer, "  ✅ %s deployed\n", result.name)
	}
	fmt.Fprintln(writer, "✅ All services deployed! (Kubernetes reconciling...)")

	// Wait for ALL services to be ready (handles dependencies via readiness probes)
	// Per DD-TEST-002: Coverage-instrumented binaries take longer to start (2-5 min vs 30s)
	// Kubernetes reconciles dependencies:
	// - DataStorage waits for PostgreSQL + Redis (retry logic + readiness probe)
	// - HolmesGPT-API waits for PostgreSQL (retry logic + readiness probe)
	// - AIAnalysis waits for HAPI + DataStorage (retry logic + readiness probe)
	// This single wait point validates the entire dependency chain
	fmt.Fprintln(writer, "\n⏳ Waiting for all services to be ready (Kubernetes reconciling dependencies)...")
	if err := waitForAllServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "✅ AIAnalysis E2E Infrastructure Ready (DD-TEST-002 Compliant)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}
