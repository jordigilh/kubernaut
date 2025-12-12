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

// Package infrastructure provides shared E2E test infrastructure for all services.
//
// This file implements the RemediationOrchestrator E2E infrastructure.
// Uses the shared migration library per DS_E2E_MIGRATION_LIBRARY_IMPLEMENTATION_SCHEDULE.md
//
// RO Audit Events Emitted:
//   - orchestrator.lifecycle.started
//   - orchestrator.phase.transitioned
//   - orchestrator.remediation.completed
//   - orchestrator.remediation.failed
//
// All audit events require the audit_events table (DD-AUDIT-003).
package infrastructure

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// ROClusterConfig holds configuration for RO E2E cluster setup
type ROClusterConfig struct {
	ClusterName    string
	KubeconfigPath string
	Namespace      string
	Writer         io.Writer
}

// DefaultROClusterConfig returns default configuration for RO E2E tests
func DefaultROClusterConfig() ROClusterConfig {
	homeDir, _ := os.UserHomeDir()
	return ROClusterConfig{
		ClusterName:    "ro-e2e",
		KubeconfigPath: filepath.Join(homeDir, ".kube", "ro-e2e-config"),
		Namespace:      "kubernaut-system",
	}
}

// CreateROCluster creates a Kind cluster for RemediationOrchestrator E2E testing.
// This is called ONCE in SynchronizedBeforeSuite (first parallel process only).
//
// Steps:
// 1. Create Kind cluster with production-like configuration
// 2. Export kubeconfig to ~/.kube/ro-e2e-config
// 3. Install ALL CRDs required for RO orchestration
// 4. Deploy PostgreSQL for audit storage (DD-AUDIT-003)
// 5. Apply audit migrations using shared library
// 6. Deploy Data Storage service
// 7. Deploy RO controller and dependent controllers
//
// Time: ~2-3 minutes (full stack deployment)
func CreateROCluster(ctx context.Context, config ROClusterConfig) error {
	writer := config.Writer
	if writer == nil {
		writer = os.Stdout
	}

	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "RemediationOrchestrator E2E Cluster Setup")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Check if cluster already exists
	if roClusterExists(config.ClusterName) {
		fmt.Fprintln(writer, "♻️  Reusing existing cluster")
		if err := roExportKubeconfig(config.ClusterName, config.KubeconfigPath, writer); err != nil {
			return fmt.Errorf("failed to export kubeconfig: %w", err)
		}
	} else {
		// Create Kind cluster
		fmt.Fprintln(writer, "📦 Creating Kind cluster...")
		if err := createROKindCluster(config.ClusterName, config.KubeconfigPath, writer); err != nil {
			return fmt.Errorf("failed to create Kind cluster: %w", err)
		}
	}

	// 2. Set KUBECONFIG environment variable
	if err := os.Setenv("KUBECONFIG", config.KubeconfigPath); err != nil {
		return fmt.Errorf("failed to set KUBECONFIG: %w", err)
	}
	fmt.Fprintf(writer, "📂 KUBECONFIG=%s\n", config.KubeconfigPath)

	// 3. Install ALL CRDs required for RO orchestration
	fmt.Fprintln(writer, "📋 Installing CRDs...")
	if err := installROCRDs(config.KubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install CRDs: %w", err)
	}

	// 4. Create namespace
	fmt.Fprintf(writer, "📁 Creating namespace: %s\n", config.Namespace)
	if err := roCreateNamespace(config.KubeconfigPath, config.Namespace, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 5. Deploy PostgreSQL for audit storage
	fmt.Fprintln(writer, "🐘 Deploying PostgreSQL...")
	if err := deployROPostgreSQL(ctx, config.Namespace, config.KubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 6. Apply audit migrations using shared library (DD-AUDIT-003)
	fmt.Fprintln(writer, "🔄 Applying audit migrations (shared library)...")
	if err := ApplyAuditMigrations(ctx, config.Namespace, config.KubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply audit migrations: %w", err)
	}

	// 7. Verify migrations applied correctly
	fmt.Fprintln(writer, "✅ Verifying migrations...")
	migConfig := DefaultMigrationConfig(config.Namespace, config.KubeconfigPath)
	migConfig.Tables = []string{"audit_events"}
	if err := VerifyMigrations(ctx, migConfig, writer); err != nil {
		return fmt.Errorf("migration verification failed: %w", err)
	}

	// 8. Deploy Data Storage service
	fmt.Fprintln(writer, "📦 Deploying Data Storage service...")
	if err := deployDataStorageForRO(ctx, config.Namespace, config.KubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage: %w", err)
	}

	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "✅ RemediationOrchestrator E2E cluster ready!")
	fmt.Fprintf(writer, "   Cluster: %s\n", config.ClusterName)
	fmt.Fprintf(writer, "   Kubeconfig: %s\n", config.KubeconfigPath)
	fmt.Fprintf(writer, "   Namespace: %s\n", config.Namespace)
	fmt.Fprintln(writer, "   Audit: audit_events table + partitions + indexes")
	fmt.Fprintln(writer, "")

	return nil
}

// DeleteROCluster deletes the RO E2E Kind cluster
func DeleteROCluster(config ROClusterConfig, preserveOnFailure bool) error {
	writer := config.Writer
	if writer == nil {
		writer = os.Stdout
	}

	if preserveOnFailure {
		fmt.Fprintln(writer, "⚠️  PRESERVE_E2E_CLUSTER=true, keeping cluster for debugging")
		fmt.Fprintf(writer, "   To access: export KUBECONFIG=%s\n", config.KubeconfigPath)
		fmt.Fprintf(writer, "   To delete: kind delete cluster --name %s\n", config.ClusterName)
		return nil
	}

	fmt.Fprintln(writer, "🗑️  Deleting Kind cluster...")
	cmd := exec.Command("kind", "delete", "cluster", "--name", config.ClusterName)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(writer, "⚠️  Failed to delete cluster (may not exist): %v\n", err)
	}

	// Remove kubeconfig file
	if config.KubeconfigPath != "" {
		defaultConfig := os.ExpandEnv("$HOME/.kube/config")
		if config.KubeconfigPath != defaultConfig {
			_ = os.Remove(config.KubeconfigPath)
			fmt.Fprintf(writer, "🗑️  Removed kubeconfig: %s\n", config.KubeconfigPath)
		}
	}

	return nil
}

// ============================================================================
// Internal Helper Functions (prefixed with ro to avoid conflicts)
// ============================================================================

func roClusterExists(name string) bool {
	cmd := exec.Command("kind", "get", "clusters")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	for _, line := range roSplitLines(string(output)) {
		if line == name {
			return true
		}
	}
	return false
}

func roSplitLines(s string) []string {
	var lines []string
	var current string
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

func createROKindCluster(name, kubeconfig string, writer io.Writer) error {
	cmd := exec.Command("kind", "create", "cluster",
		"--name", name,
		"--kubeconfig", kubeconfig,
		"--wait", "120s",
	)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}
	return nil
}

func roExportKubeconfig(name, kubeconfig string, writer io.Writer) error {
	cmd := exec.Command("kind", "export", "kubeconfig",
		"--name", name,
		"--kubeconfig", kubeconfig,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func installROCRDs(kubeconfig string, writer io.Writer) error {
	// CRDs required for RO orchestration
	crdPaths := []string{
		"config/crd/bases/remediation.kubernaut.ai_remediationrequests.yaml",
		"config/crd/bases/remediation.kubernaut.ai_remediationapprovalrequests.yaml",
		"config/crd/bases/signalprocessing.kubernaut.ai_signalprocessings.yaml",
		"config/crd/bases/aianalysis.kubernaut.ai_aianalyses.yaml",
		"config/crd/bases/workflowexecution.kubernaut.ai_workflowexecutions.yaml",
		"config/crd/bases/notification.kubernaut.ai_notificationrequests.yaml",
	}

	for _, crdPath := range crdPaths {
		fullPath := roFindProjectFile(crdPath)
		if fullPath == "" {
			fmt.Fprintf(writer, "⚠️  CRD not found: %s\n", crdPath)
			continue
		}

		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig,
			"apply", "-f", fullPath)
		cmd.Stdout = writer
		cmd.Stderr = writer

		if err := cmd.Run(); err != nil {
			fmt.Fprintf(writer, "⚠️  Failed to install CRD %s: %v\n", crdPath, err)
		}
	}
	return nil
}

func roFindProjectFile(relativePath string) string {
	paths := []string{
		relativePath,
		"../../../" + relativePath,
		"../../" + relativePath,
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func roCreateNamespace(kubeconfig, namespace string, writer io.Writer) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig,
		"create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
	yaml, err := cmd.Output()
	if err != nil {
		return err
	}

	applyCmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "apply", "-f", "-")
	applyCmd.Stdin = roBytesReader(yaml)
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer
	return applyCmd.Run()
}

func roBytesReader(b []byte) *roBytesReaderImpl {
	return &roBytesReaderImpl{data: b}
}

type roBytesReaderImpl struct {
	data []byte
	pos  int
}

func (r *roBytesReaderImpl) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

func deployROPostgreSQL(ctx context.Context, namespace, kubeconfig string, writer io.Writer) error {
	fmt.Fprintln(writer, "   Checking for existing PostgreSQL...")

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace,
		"get", "pod", "-l", "app=postgresql", "-o", "name")
	output, _ := cmd.Output()
	if len(output) > 0 {
		fmt.Fprintln(writer, "   ♻️  Reusing existing PostgreSQL")
		return waitForROPostgreSQL(ctx, namespace, kubeconfig, writer)
	}

	fmt.Fprintln(writer, "   Deploying PostgreSQL...")
	return createMinimalROPostgreSQL(namespace, kubeconfig, writer)
}

func createMinimalROPostgreSQL(namespace, kubeconfig string, writer io.Writer) error {
	manifest := `
apiVersion: v1
kind: Service
metadata:
  name: postgresql
spec:
  ports:
    - port: 5432
  selector:
    app: postgresql
---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgresql
spec:
  serviceName: postgresql
  replicas: 1
  selector:
    matchLabels:
      app: postgresql
  template:
    metadata:
      labels:
        app: postgresql
    spec:
      containers:
        - name: postgresql
          image: postgres:16-alpine
          ports:
            - containerPort: 5432
          env:
            - name: POSTGRES_USER
              value: slm_user
            - name: POSTGRES_PASSWORD
              value: slm_password
            - name: POSTGRES_DB
              value: action_history
          readinessProbe:
            exec:
              command: ["pg_isready", "-U", "slm_user", "-d", "action_history"]
            initialDelaySeconds: 5
            periodSeconds: 5
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace, "apply", "-f", "-")
	cmd.Stdin = roBytesReader([]byte(manifest))
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create PostgreSQL: %w", err)
	}

	return waitForROPostgreSQL(context.Background(), namespace, kubeconfig, writer)
}

func waitForROPostgreSQL(ctx context.Context, namespace, kubeconfig string, writer io.Writer) error {
	fmt.Fprintln(writer, "   Waiting for PostgreSQL to be ready...")

	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace,
			"wait", "--for=condition=ready", "pod", "-l", "app=postgresql", "--timeout=10s")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "   ✅ PostgreSQL ready")
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	return fmt.Errorf("PostgreSQL not ready within timeout")
}

func deployDataStorageForRO(ctx context.Context, namespace, kubeconfig string, writer io.Writer) error {
	manifestPath := roFindProjectFile("deploy/datastorage/deployment.yaml")
	if manifestPath == "" {
		fmt.Fprintln(writer, "   ⚠️  Data Storage manifest not found, skipping (audit will fail)")
		return nil
	}

	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace,
		"apply", "-f", manifestPath)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(writer, "   ⚠️  Failed to deploy Data Storage: %v\n", err)
		return nil
	}

	fmt.Fprintln(writer, "   Waiting for Data Storage to be ready...")
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfig, "-n", namespace,
			"wait", "--for=condition=ready", "pod", "-l", "app=datastorage", "--timeout=10s")
		if err := cmd.Run(); err == nil {
			fmt.Fprintln(writer, "   ✅ Data Storage ready")
			return nil
		}
		time.Sleep(5 * time.Second)
	}

	fmt.Fprintln(writer, "   ⚠️  Data Storage not ready within timeout (audit may fail)")
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// RemediationOrchestrator Integration Test Infrastructure (Podman Compose)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Pattern: AIAnalysis Pattern (Programmatic podman-compose management)
// Authority: docs/handoff/TRIAGE_RO_INFRASTRUCTURE_BOOTSTRAP_COMPARISON.md
//
// Port Allocation (per DD-TEST-001):
//   PostgreSQL:       15435 → 5432 (RO-specific, from range 15433-15442)
//   Redis:            16381 → 6379 (RO-specific, from range 16379-16388)
//   Data Storage API: 18140 → 8080 (RO-specific, after stateless services)
//   DS Metrics:       18141 → 9090
//
// Dependencies:
//   RO → Data Storage (audit events, workflow catalog)
//   Data Storage → PostgreSQL (persistence)
//   Data Storage → Redis (caching/DLQ)
//
// Parallel Execution:
//   - Uses SynchronizedBeforeSuite for parallel-safe setup
//   - Process 1 starts infrastructure ONCE
//   - ALL processes share the same infrastructure
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	// RO Integration Test Ports (per DD-TEST-001)
	ROIntegrationPostgresPort           = 15435
	ROIntegrationRedisPort              = 16381
	ROIntegrationDataStoragePort        = 18140
	ROIntegrationDataStorageMetricsPort = 18141

	// Compose project name
	ROIntegrationComposeProject = "remediationorchestrator-integration"

	// Compose file path relative to project root
	ROIntegrationComposeFile = "test/integration/remediationorchestrator/podman-compose.remediationorchestrator.test.yml"
)

// StartROIntegrationInfrastructure starts the full podman-compose stack for RO integration tests
// This includes: PostgreSQL, Redis, and Data Storage API
//
// Pattern: AIAnalysis Pattern (per TRIAGE_RO_INFRASTRUCTURE_BOOTSTRAP_COMPARISON.md)
// - Programmatic podman-compose management
// - Health checks via HTTP endpoints
// - Parallel-safe (called from SynchronizedBeforeSuite)
//
// Prerequisites:
// - podman-compose must be installed
// - Ports 15435, 16381, 18140, 18141 must be available
//
// Returns:
// - error: Any errors during infrastructure startup
func StartROIntegrationInfrastructure(writer io.Writer) error {
	projectRoot := getProjectRoot()
	composeFile := filepath.Join(projectRoot, ROIntegrationComposeFile)

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Starting RO Integration Test Infrastructure\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", ROIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:          localhost:%d\n", ROIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", ROIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  DS Metrics:     http://localhost:%d\n", ROIntegrationDataStorageMetricsPort)
	fmt.Fprintf(writer, "  Compose File:   %s\n", ROIntegrationComposeFile)
	fmt.Fprintf(writer, "  Pattern:        AIAnalysis (Programmatic podman-compose)\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Check if podman-compose is available
	if err := exec.Command("podman-compose", "--version").Run(); err != nil {
		return fmt.Errorf("podman-compose not found: %w (install via: pip install podman-compose)", err)
	}

	// Start services with --build flag
	cmd := exec.Command("podman-compose",
		"-f", composeFile,
		"-p", ROIntegrationComposeProject,
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
	if err := waitForROHTTPHealth(
		fmt.Sprintf("http://localhost:%d/health", ROIntegrationDataStoragePort),
		90*time.Second, // Longer timeout for migrations + build
		writer,
	); err != nil {
		return fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "✅ DataStorage is healthy (postgres + redis + migrations validated)\n")

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "✅ RO Integration Infrastructure Ready\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// StopROIntegrationInfrastructure stops and cleans up the RO integration test infrastructure
//
// Pattern: AIAnalysis Pattern (per TRIAGE_RO_INFRASTRUCTURE_BOOTSTRAP_COMPARISON.md)
// - Programmatic podman-compose cleanup
// - Removes volumes (-v flag)
// - Parallel-safe (called from SynchronizedAfterSuite)
//
// Returns:
// - error: Any errors during infrastructure cleanup
func StopROIntegrationInfrastructure(writer io.Writer) error {
	projectRoot := getProjectRoot()
	composeFile := filepath.Join(projectRoot, ROIntegrationComposeFile)

	fmt.Fprintf(writer, "🛑 Stopping RO Integration Infrastructure...\n")

	// Stop and remove containers + volumes
	cmd := exec.Command("podman-compose",
		"-f", composeFile,
		"-p", ROIntegrationComposeProject,
		"down", "-v",
	)
	cmd.Dir = projectRoot
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(writer, "⚠️  Warning: Error stopping infrastructure: %v\n", err)
		return err
	}

	fmt.Fprintf(writer, "✅ RO Integration Infrastructure stopped and cleaned up\n")
	return nil
}

// waitForROHTTPHealth waits for an HTTP health endpoint to respond with 200 OK
// Includes retry logging for debugging infrastructure issues
func waitForROHTTPHealth(healthURL string, timeout time.Duration, writer io.Writer) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 5 * time.Second}
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				fmt.Fprintf(writer, "   ✅ Health check passed after %d attempts\n", attempt)
				return nil
			}
			fmt.Fprintf(writer, "   ⏳ Attempt %d: Status %d (waiting for 200 OK)...\n", attempt, resp.StatusCode)
		} else {
			if attempt%5 == 0 { // Log every 5th attempt
				fmt.Fprintf(writer, "   ⏳ Attempt %d: Connection failed (%v), retrying...\n", attempt, err)
			}
		}
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("timeout waiting for health endpoint after %d attempts: %s", attempt, healthURL)
}
