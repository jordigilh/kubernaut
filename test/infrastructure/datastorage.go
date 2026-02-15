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
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/redis/go-redis/v9"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// CreateDataStorageCluster creates a Kind cluster for Data Storage E2E tests
// This includes:
// - Kind cluster (2 nodes: control-plane + worker)
// - Data Storage Service Docker image (build + load)
func CreateDataStorageCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "Data Storage E2E Cluster Setup (ONCE)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Create Kind cluster
	_, _ = fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Build Data Storage Docker image
	_, _ = fmt.Fprintln(writer, "🔨 Building Data Storage Docker image...")
	if err := buildDataStorageImage(writer); err != nil {
		return fmt.Errorf("failed to build Data Storage image: %w", err)
	}

	// 3. Load Data Storage image into Kind
	_, _ = fmt.Fprintln(writer, "📦 Loading Data Storage image into Kind cluster...")
	if err := loadDataStorageImage(clusterName, writer); err != nil {
		return fmt.Errorf("failed to load Data Storage image: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Cluster ready - tests can now deploy services per-namespace")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// MustGatherPodLogs collects logs from ALL pods in the specified namespace using
// kubectl logs. This captures both current and previous container logs, which is
// more reliable than `kind export logs` (which may miss some pod logs).
//
// Logs are written to /tmp/kubernaut-must-gather/{serviceName}/ so the CI pipeline's
// existing must-gather collection step picks them up automatically.
//
// Parameters:
//   - clusterName: Name of the Kind cluster (used for kubeconfig context)
//   - kubeconfigPath: Path to the kubeconfig file
//   - namespace: Kubernetes namespace to collect logs from (e.g., "kubernaut-system")
//   - serviceName: Service name for directory naming (e.g., "fullpipeline", "aianalysis")
//   - writer: Output writer for logging
func MustGatherPodLogs(clusterName, kubeconfigPath, namespace, serviceName string, writer io.Writer) {
	_, _ = fmt.Fprintf(writer, "═══════════════════════════════════════════════════════════\n")
	_, _ = fmt.Fprintf(writer, "📋 MUST-GATHER: Collecting pod logs via kubectl\n")
	_, _ = fmt.Fprintf(writer, "   Cluster: %s | Namespace: %s | Service: %s\n", clusterName, namespace, serviceName)
	_, _ = fmt.Fprintf(writer, "═══════════════════════════════════════════════════════════\n\n")

	mustGatherDir := fmt.Sprintf("/tmp/kubernaut-must-gather/%s", serviceName)
	if err := os.MkdirAll(mustGatherDir, 0755); err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Failed to create must-gather directory %s: %v\n", mustGatherDir, err)
		return
	}

	// Determine kubeconfig args
	kubeconfigArgs := []string{}
	if kubeconfigPath != "" {
		kubeconfigArgs = append(kubeconfigArgs, "--kubeconfig", kubeconfigPath)
	}

	// Get all pods in the namespace
	getPodArgs := append(kubeconfigArgs, "get", "pods", "-n", namespace,
		"-o", "jsonpath={range .items[*]}{.metadata.name},{.spec.containers[*].name},{.spec.initContainers[*].name}{\"\\n\"}{end}")
	getPodsCmd := exec.Command("kubectl", getPodArgs...)
	podOutput, err := getPodsCmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Failed to list pods in namespace %s: %v\n%s\n", namespace, err, string(podOutput))
		return
	}

	podLines := strings.Split(strings.TrimSpace(string(podOutput)), "\n")
	if len(podLines) == 0 || (len(podLines) == 1 && podLines[0] == "") {
		_, _ = fmt.Fprintf(writer, "⚠️  No pods found in namespace %s\n", namespace)
		return
	}

	collectedCount := 0
	for _, line := range podLines {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, ",", 3)
		if len(parts) < 2 {
			continue
		}
		podName := parts[0]
		containers := strings.Fields(parts[1])
		initContainers := []string{}
		if len(parts) > 2 && parts[2] != "" {
			initContainers = strings.Fields(parts[2])
		}

		allContainers := append(containers, initContainers...)

		for _, container := range allContainers {
			if container == "" {
				continue
			}

			// Collect current logs
			logFile := filepath.Join(mustGatherDir, fmt.Sprintf("%s_%s.log", podName, container))
			logArgs := append(kubeconfigArgs, "logs", "-n", namespace, podName, "-c", container, "--tail=-1")
			logCmd := exec.Command("kubectl", logArgs...)
			logOutput, logErr := logCmd.CombinedOutput()

			if logErr == nil && len(logOutput) > 0 {
				if writeErr := os.WriteFile(logFile, logOutput, 0644); writeErr == nil {
					collectedCount++
				}
			}

			// Collect previous container logs (for crashed/restarted containers)
			prevLogFile := filepath.Join(mustGatherDir, fmt.Sprintf("%s_%s_previous.log", podName, container))
			prevLogArgs := append(kubeconfigArgs, "logs", "-n", namespace, podName, "-c", container, "--previous", "--tail=-1")
			prevLogCmd := exec.Command("kubectl", prevLogArgs...)
			prevLogOutput, prevLogErr := prevLogCmd.CombinedOutput()

			if prevLogErr == nil && len(prevLogOutput) > 0 {
				if writeErr := os.WriteFile(prevLogFile, prevLogOutput, 0644); writeErr == nil {
					collectedCount++
				}
			}
		}
	}

	// Also collect events
	eventsFile := filepath.Join(mustGatherDir, "events.txt")
	eventsArgs := append(kubeconfigArgs, "get", "events", "-n", namespace, "--sort-by=.lastTimestamp")
	eventsCmd := exec.Command("kubectl", eventsArgs...)
	eventsOutput, eventsErr := eventsCmd.CombinedOutput()
	if eventsErr == nil && len(eventsOutput) > 0 {
		_ = os.WriteFile(eventsFile, eventsOutput, 0644)
	}

	// Collect pod status
	statusFile := filepath.Join(mustGatherDir, "pod_status.txt")
	statusArgs := append(kubeconfigArgs, "get", "pods", "-n", namespace, "-o", "wide")
	statusCmd := exec.Command("kubectl", statusArgs...)
	statusOutput, statusErr := statusCmd.CombinedOutput()
	if statusErr == nil && len(statusOutput) > 0 {
		_ = os.WriteFile(statusFile, statusOutput, 0644)
	}

	_, _ = fmt.Fprintf(writer, "✅ Must-gather collected %d log files to %s\n", collectedCount, mustGatherDir)
	_, _ = fmt.Fprintf(writer, "   (Events and pod status also captured)\n\n")
}

// DeleteCluster deletes a Kind cluster and optionally exports logs on test failure
//
// Parameters:
//   - clusterName: Name of the Kind cluster to delete
//   - serviceName: Service name for log directory naming (e.g., "gateway", "datastorage")
//   - testsFailed: If true, exports logs before deletion (must-gather style)
//   - writer: Output writer for logging
//
// Log Export Behavior (when testsFailed=true):
//   - CI/CD mode: Collects pod logs via kubectl to /tmp/kubernaut-must-gather/ and preserves cluster
//   - Local mode: Exports to /tmp/{serviceName}-e2e-logs-{timestamp} via kind export logs
//   - ALWAYS deletes cluster after log export (local mode only)
//
// Example:
//
//	err := DeleteCluster("gateway-e2e", "gateway", anyTestFailed, GinkgoWriter)
func DeleteCluster(clusterName, serviceName string, testsFailed bool, writer io.Writer) error {
	// ═══════════════════════════════════════════════════════════════════════
	// FIX: Preserve cluster in CI/CD when tests fail (for must-gather)
	// ═══════════════════════════════════════════════════════════════════════
	// In CI/CD (IMAGE_REGISTRY set), GitHub Actions workflow collects must-gather
	// artifacts. Tests must NOT delete cluster on failure so workflow can inspect
	// pod status, events, and logs.
	//
	// In local dev (IMAGE_REGISTRY not set), export logs immediately for debugging,
	// then delete cluster to free resources.
	inCICD := os.Getenv("IMAGE_REGISTRY") != ""

	if testsFailed {
		if inCICD {
			// ═══════════════════════════════════════════════════════════════════════
			// CI/CD MODE: Collect pod logs via kubectl BEFORE preserving cluster
			// ═══════════════════════════════════════════════════════════════════════
			_, _ = fmt.Fprintf(writer, "⚠️  Test failure detected in CI/CD environment\n")

			// Collect pod logs to /tmp/kubernaut-must-gather/ for CI artifact collection
			homeDir, _ := os.UserHomeDir()
			kubeconfigPath := fmt.Sprintf("%s/.kube/%s-config", homeDir, clusterName)
			MustGatherPodLogs(clusterName, kubeconfigPath, "kubernaut-system", serviceName, writer)

			_, _ = fmt.Fprintf(writer, "🔍 Preserving Kind cluster for must-gather collection\n")
			_, _ = fmt.Fprintf(writer, "   • Cluster: %s\n", clusterName)
			_, _ = fmt.Fprintf(writer, "   • GitHub Actions will collect pod logs, events, and status\n")
			_, _ = fmt.Fprintf(writer, "   • Workflow will delete cluster after artifact collection\n")
			_, _ = fmt.Fprintf(writer, "✅ Cluster preserved for diagnostics\n")
			return nil // Don't delete - let GitHub Actions handle it
		}

		// ═══════════════════════════════════════════════════════════════════════
		// LOCAL MODE: Export logs immediately (Must-Gather Style)
		// ═══════════════════════════════════════════════════════════════════════
		_, _ = fmt.Fprintf(writer, "⚠️  Test failure detected - collecting diagnostic information...\n\n")
		_, _ = fmt.Fprintf(writer, "📋 Exporting cluster logs (Kind must-gather)...\n")

		logsDir := fmt.Sprintf("/tmp/%s-e2e-logs-%s", serviceName, time.Now().Format("20060102-150405"))
		exportCmd := exec.Command("kind", "export", "logs", logsDir, "--name", clusterName)

		if exportOutput, exportErr := exportCmd.CombinedOutput(); exportErr != nil {
			_, _ = fmt.Fprintf(writer, "❌ Failed to export Kind logs: %s\n", string(exportOutput))
			_, _ = fmt.Fprintf(writer, "   (Continuing with cluster deletion)\n\n")
		} else {
			_, _ = fmt.Fprintf(writer, "✅ Cluster logs exported successfully\n")
			_, _ = fmt.Fprintf(writer, "📁 Location: %s\n", logsDir)
			_, _ = fmt.Fprintf(writer, "📁 Contents: pod logs, node logs, kubelet logs, and more\n\n")

			// ═══════════════════════════════════════════════════════════════════════
			// EXTRACT KUBERNAUT SERVICE LOGS (for immediate analysis)
			// ═══════════════════════════════════════════════════════════════════════
			extractKubernautServiceLogs(logsDir, serviceName, writer)
		}
	}

	// ═══════════════════════════════════════════════════════════════════════
	// DELETE CLUSTER (normal cleanup or after local log export)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(writer, "🗑️  Deleting Kind cluster...\n")
	cmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Failed to delete cluster: %s\n", output)
		return fmt.Errorf("failed to delete cluster: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ Kind cluster deleted\n")
	return nil
}

// ExportMustGatherLogs exports Kind cluster logs to a local directory for debugging.
// This is a standalone function that can be called independently of DeleteCluster,
// e.g., by test suites that use IMAGE_REGISTRY for remote images but still run locally.
//
// Parameters:
//   - clusterName: Name of the Kind cluster
//   - serviceName: Service name for log directory naming (e.g., "fullpipeline")
//   - writer: Output writer for logging
//
// Exports to: /tmp/{serviceName}-e2e-logs-{timestamp}
func ExportMustGatherLogs(clusterName, serviceName string, writer io.Writer) {
	_, _ = fmt.Fprintf(writer, "⚠️  Test failure detected - collecting diagnostic information...\n\n")
	_, _ = fmt.Fprintf(writer, "📋 Exporting cluster logs (Kind must-gather)...\n")

	logsDir := fmt.Sprintf("/tmp/%s-e2e-logs-%s", serviceName, time.Now().Format("20060102-150405"))
	exportCmd := exec.Command("kind", "export", "logs", logsDir, "--name", clusterName)

	if exportOutput, exportErr := exportCmd.CombinedOutput(); exportErr != nil {
		_, _ = fmt.Fprintf(writer, "❌ Failed to export Kind logs: %s\n", string(exportOutput))
	} else {
		_, _ = fmt.Fprintf(writer, "✅ Cluster logs exported successfully\n")
		_, _ = fmt.Fprintf(writer, "📁 Location: %s\n", logsDir)
		_, _ = fmt.Fprintf(writer, "📁 Contents: pod logs, node logs, kubelet logs, and more\n\n")

		extractKubernautServiceLogs(logsDir, serviceName, writer)
	}
}

// extractKubernautServiceLogs finds and displays logs from kubernaut services
// This helps with immediate debugging without manually navigating Kind log directories
func extractKubernautServiceLogs(logsDir, serviceName string, writer io.Writer) {
	// Define kubernaut service patterns to search for
	servicePatterns := []struct {
		name    string
		pattern string
	}{
		{serviceName, fmt.Sprintf("*%s*/*.log", serviceName)},
		{"datastorage", "*datastorage*/*.log"},
		{"gateway", "*gateway*/*.log"},
		{"holmesgpt-api", "*holmesgpt*/*.log"},
		{"aianalysis", "*aianalysis*/*.log"},
		{"notification", "*notification*/*.log"},
		{"signalprocessing", "*signalprocessing*/*.log"},
		{"workflowexecution", "*workflowexecution*/*.log"},
		{"remediationorchestrator", "*remediationorchestrator*/*.log"},
		{"authwebhook", "*authwebhook*/*.log"},
	}

	_, _ = fmt.Fprintf(writer, "═══════════════════════════════════════════════════════════\n")
	_, _ = fmt.Fprintf(writer, "📋 KUBERNAUT SERVICE LOGS (Last 100 lines each)\n")
	_, _ = fmt.Fprintf(writer, "═══════════════════════════════════════════════════════════\n\n")

	foundAny := false
	for _, svc := range servicePatterns {
		// Try to find service log files
		findPattern := filepath.Join(logsDir, "*", svc.pattern)
		findCmd := exec.Command("sh", "-c", fmt.Sprintf("ls %s 2>/dev/null | head -5", findPattern))
		logPaths, err := findCmd.Output()

		if err == nil && len(logPaths) > 0 {
			// Process each log file found
			for _, logPath := range strings.Split(strings.TrimSpace(string(logPaths)), "\n") {
				if logPath == "" {
					continue
				}

				foundAny = true
				_, _ = fmt.Fprintf(writer, "📄 Service: %s\n", svc.name)
				_, _ = fmt.Fprintf(writer, "📁 Path: %s\n", logPath)
				_, _ = fmt.Fprintf(writer, "-----------------------------------------------------------\n")

				// Display last 100 lines
				tailCmd := exec.Command("tail", "-100", logPath)
				if tailOutput, tailErr := tailCmd.CombinedOutput(); tailErr == nil {
					_, _ = fmt.Fprintln(writer, string(tailOutput))
				} else {
					_, _ = fmt.Fprintf(writer, "⚠️  Could not read log: %v\n", tailErr)
				}
				_, _ = fmt.Fprintf(writer, "═══════════════════════════════════════════════════════════\n\n")
				break // Only show first log file per service
			}
		}
	}

	if !foundAny {
		_, _ = fmt.Fprintf(writer, "⚠️  No kubernaut service logs found in exported directory\n")
		_, _ = fmt.Fprintf(writer, "   (This is normal if pods never started)\n\n")
	}
}

// SetupDataStorageInfrastructureParallel creates the full E2E infrastructure with parallel execution.
// This optimizes setup time by running independent tasks concurrently.
//
// Parallel Execution Strategy:
//
//	Phase 1 (Sequential): Create Kind cluster + namespace (~65s)
//	Phase 2 (PARALLEL):   Build/Load DS image | Deploy PostgreSQL | Deploy Redis (~60s)
//	Phase 3 (Sequential): Run migrations (~30s)
//	Phase 4 (Sequential): Deploy DataStorage service (~30s)
//	Phase 5 (Sequential): Wait for services ready (~30s)
//
// Total time: ~3.6 minutes (vs ~4.7 minutes sequential)
// Savings: ~1 minute per E2E run (~23% faster)
//
// PostgreSQL-only architecture (SOC2 audit storage)
//
// Based on SignalProcessing reference implementation (test/infrastructure/signalprocessing.go:246)
// SetupDataStorageInfrastructureParallel sets up DataStorage E2E infrastructure with OAuth2-Proxy.
// TD-E2E-001 Phase 1: OAuth2-Proxy pulled automatically from quay.io (no manual build/load).
func SetupDataStorageInfrastructureParallel(ctx context.Context, clusterName, kubeconfigPath, namespace, dataStorageImage string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 DataStorage E2E Infrastructure (HYBRID PATTERN)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build image → Create cluster → Load → Deploy")
	_, _ = fmt.Fprintln(writer, "  Optimization: Eliminates cluster idle time during image build")
	_, _ = fmt.Fprintln(writer, "  Authority: Gateway hybrid pattern migration (Jan 7, 2026)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build DataStorage image (BEFORE cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building DataStorage image (NO CLUSTER YET)...")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~1-2 minutes")

	cfg := E2EImageConfig{
		ServiceName:      "datastorage",
		ImageName:        "kubernaut/datastorage",
		DockerfilePath:   "docker/data-storage.Dockerfile",
		BuildContextPath: "", // Empty = use project root (default)
		EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
	}
	dsImageName, err := BuildImageForKind(cfg, writer)
	if err != nil {
		return fmt.Errorf("DS image build failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ DataStorage image built: %s\n", dsImageName)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster + namespace
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster + namespace...")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~10-15 seconds")

	// Create Kind cluster
	if err := createKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// Create namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// Deploy ClusterRole for client access (DD-AUTH-014)
	_, _ = fmt.Fprintf(writer, "🔐 Deploying data-storage-client ClusterRole (DD-AUTH-014)...\n")
	if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy client ClusterRole: %w", err)
	}

	// Deploy ServiceAccount and RBAC for DataStorage middleware (DD-AUTH-014)
	_, _ = fmt.Fprintf(writer, "🔐 Deploying DataStorage service RBAC for auth middleware (DD-AUTH-014)...\n")
	if err := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy service RBAC: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load image + Deploy infrastructure in PARALLEL
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n⚡ PHASE 3: Loading image + Deploying infrastructure in parallel...")
	_, _ = fmt.Fprintln(writer, "  ├── Loading DataStorage image to Kind")
	_, _ = fmt.Fprintln(writer, "  ├── Deploying PostgreSQL")
	_, _ = fmt.Fprintln(writer, "  └── Deploying Redis")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~30-60 seconds")

	type result struct {
		name string
		err  error
	}

	results := make(chan result, 3)

	// Goroutine 1: Load pre-built DataStorage image to Kind
	go func() {
		defer GinkgoRecover() // Required for Ginkgo assertions in goroutines
		err := LoadImageToKind(dsImageName, "datastorage", clusterName, writer)
		if err != nil {
			err = fmt.Errorf("DS image load failed: %w", err)
		}
		results <- result{name: "DS image load", err: err}
	}()

	// Goroutine 2: Deploy PostgreSQL
	go func() {
		defer GinkgoRecover() // Required for Ginkgo assertions in goroutines
		err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer)
		if err != nil {
			err = fmt.Errorf("PostgreSQL deploy failed: %w", err)
		}
		results <- result{name: "PostgreSQL", err: err}
	}()

	// Goroutine 3: Deploy Redis
	go func() {
		defer GinkgoRecover() // Required for Ginkgo assertions in goroutines
		err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer)
		if err != nil {
			err = fmt.Errorf("Redis deploy failed: %w", err)
		}
		results <- result{name: "Redis", err: err}
	}()

	// Wait for all parallel tasks to complete
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for parallel tasks to complete...")
	for i := 0; i < 3; i++ {
		r := <-results
		if r.err != nil {
			return fmt.Errorf("parallel setup failed (%s): %w", r.name, r.err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s complete\n", r.name)
	}

	// Update dataStorageImage to use the actual built image name for deployment
	dataStorageImage = dsImageName

	_, _ = fmt.Fprintln(writer, "✅ Phase 3 complete - image loaded + infrastructure deployed")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy migrations + DataStorage service in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying migrations + DataStorage service in parallel...")
	_, _ = fmt.Fprintln(writer, "  (Kubernetes will handle dependencies - DataStorage retries until migrations complete)")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~20-30 seconds")

	type deployResult struct {
		name string
		err  error
	}
	deployResults := make(chan deployResult, 2)

	// Launch migrations and DataStorage deployment concurrently
	go func() {
		defer GinkgoRecover() // Required for Ginkgo assertions in goroutines
		err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"Migrations", err}
	}()
	go func() {
		defer GinkgoRecover() // Required for Ginkgo assertions in goroutines
		// TD-E2E-001 Phase 1: Deploy with OAuth2-Proxy sidecar (image from quay.io)
		err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, dataStorageImage, writer)
		deployResults <- deployResult{"DataStorage", err}
	}()

	// Collect ALL results before proceeding (MANDATORY)
	var deployErrors []error
	for i := 0; i < 2; i++ {
		result := <-deployResults
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s deployment failed: %v\n", result.name, result.err)
			deployErrors = append(deployErrors, result.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s manifests applied\n", result.name)
		}
	}

	if len(deployErrors) > 0 {
		return fmt.Errorf("one or more deployments failed: %v", deployErrors)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ All manifests applied! (Kubernetes reconciling...)")

	// Single wait for DataStorage to be ready (migrations complete first, then DataStorage connects)
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for DataStorage to be ready (Kubernetes reconciling dependencies)...")
	if err := waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintf(writer, "✅ DataStorage E2E infrastructure ready in namespace %s\n", namespace)
	_, _ = fmt.Fprintln(writer, "   Setup time optimized: ~23%% faster than sequential")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

// DeployDataStorageTestServices deploys PostgreSQL, Redis, and Data Storage Service to a namespace
// This is used by E2E tests to create isolated test environments
// dataStorageImage: DD-TEST-001 compliant image tag (e.g., "datastorage:holmesgpt-api-a1b2c3d4")
//
// PostgreSQL-only architecture (SOC2 audit storage)
// DeployDataStorageTestServices deploys DataStorage with OAuth2-Proxy for testing.
// TD-E2E-001 Phase 1: OAuth2-Proxy pulled automatically from quay.io.
func DeployDataStorageTestServices(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Deploying Data Storage Test Services in Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "  📦 Data Storage image: %s\n", dataStorageImage)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 1. Create test namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 2. Deploy PostgreSQL
	_, _ = fmt.Fprintf(writer, "🚀 Deploying PostgreSQL...\n")
	if err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 3. Deploy Redis for DLQ
	_, _ = fmt.Fprintf(writer, "🚀 Deploying Redis for DLQ...\n")
	if err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 4. Apply database migrations using shared migration library
	_, _ = fmt.Fprintf(writer, "📋 Applying database migrations...\n")
	if err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// 4.5. Deploy DataStorage service RBAC (DD-AUTH-014) - REQUIRED before deployment
	// Creates ServiceAccount 'data-storage-sa' that deployment references
	// Without this, pod creation will be rejected by Kubernetes (silent failure)
	_, _ = fmt.Fprintf(writer, "🔐 Deploying DataStorage service RBAC (ServiceAccount + auth permissions)...\n")
	if err := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy service RBAC: %w", err)
	}

	// 5. Deploy Data Storage Service with OAuth2-Proxy (TD-E2E-001 Phase 1 - image from quay.io)
	_, _ = fmt.Fprintf(writer, "🚀 Deploying Data Storage Service with OAuth2-Proxy sidecar (quay.io)...\n")
	if err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, dataStorageImage, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage Service: %w", err)
	}

	// 6. Wait for all services ready
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for services to be ready...\n")
	if err := waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Data Storage test services ready in namespace %s (PostgreSQL + Redis + DataStorage)\n", namespace)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

// DeployDataStorageTestServicesWithNodePort deploys DataStorage with custom NodePort
// This variant allows E2E tests to specify NodePort to match Kind cluster port mappings
//
// Parameters:
//   - nodePort: NodePort to use for DataStorage service (e.g., 30081, 30090)
//
// Usage:
//
//	// Notification E2E: Uses NodePort 30090 (per kind-notification-config.yaml)
//	DeployDataStorageTestServicesWithNodePort(ctx, namespace, kubeconfigPath, image, 30090, writer)
//
//	// Gateway E2E: Uses NodePort 30081 (per kind-gateway-config.yaml)
//	DeployDataStorageTestServicesWithNodePort(ctx, namespace, kubeconfigPath, image, 30081, writer)
//
// DeployDataStorageTestServicesWithNodePort deploys DataStorage with OAuth2-Proxy using a specific NodePort.
// TD-E2E-001 Phase 1: OAuth2-Proxy pulled automatically from quay.io.
func DeployDataStorageTestServicesWithNodePort(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, nodePort int32, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Deploying Data Storage Test Services in Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "  📦 Data Storage image: %s\n", dataStorageImage)
	_, _ = fmt.Fprintf(writer, "  🔌 NodePort: %d\n", nodePort)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 1. Create test namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 2. Deploy PostgreSQL
	_, _ = fmt.Fprintf(writer, "🚀 Deploying PostgreSQL...\n")
	if err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 3. Deploy Redis for DLQ
	_, _ = fmt.Fprintf(writer, "🚀 Deploying Redis for DLQ...\n")
	if err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 4. Apply database migrations using shared migration library
	_, _ = fmt.Fprintf(writer, "📋 Applying database migrations...\n")
	if err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// 5. Deploy DataStorage service RBAC (DD-AUTH-014) - REQUIRED for pod creation
	_, _ = fmt.Fprintf(writer, "🔐 Deploying DataStorage service RBAC for auth middleware (DD-AUTH-014)...\n")
	if err := deployDataStorageServiceRBAC(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy service RBAC: %w", err)
	}

	// 6. Deploy Data Storage Service with middleware-based auth and custom NodePort (DD-AUTH-014)
	_, _ = fmt.Fprintf(writer, "🚀 Deploying Data Storage Service with middleware-based auth (NodePort %d)...\n", nodePort)
	if err := deployDataStorageServiceInNamespaceWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, nodePort, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage Service: %w", err)
	}

	// 7. Wait for all services ready
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for services to be ready...\n")
	if err := waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Data Storage test services ready in namespace %s (NodePort %d, PostgreSQL + Redis + DataStorage)\n", namespace, nodePort)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

// CleanupDataStorageTestNamespace deletes a test namespace and all resources
func CleanupDataStorageTestNamespace(namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🧹 Cleaning up namespace %s...\n", namespace)

	cmd := exec.Command("kubectl", "delete", "namespace", namespace,
		"--kubeconfig", kubeconfigPath,
		"--wait=true",
		"--timeout=60s")
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  Failed to delete namespace: %s\n", output)
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Namespace %s deleted\n", namespace)
	return nil
}

func createTestNamespace(namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
			Labels: map[string]string{
				// BR-SCOPE-002: Infrastructure namespaces must NOT be labeled as managed.
				// Only application/workload namespaces should have kubernaut.ai/managed=true.
				// Otherwise the Gateway processes events from Kubernaut's own pods
				// (FailedScheduling, FailedCreate) as signals, creating spurious RRs.
				"test": "datastorage-e2e",
			},
		},
	}

	_, err = clientset.CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		// Check for AlreadyExists error (case-insensitive for robustness)
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "alreadyexists") {
			_, _ = fmt.Fprintf(writer, "   ✅ Namespace %s already exists (reusing)\n", namespace)
			return nil
		}
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Namespace %s created\n", namespace)
	return nil
}

func getKubernetesClient(kubeconfigPath string) (*kubernetes.Clientset, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to build config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return clientset, nil
}

// deployDataStorageClientClusterRole deploys the data-storage-client ClusterRole
// for E2E tests. This ClusterRole grants full CRUD permissions on the data-storage-service
// resource, which is required for all DataStorage REST API operations with SAR validation.
//
// Authority: DD-AUTH-014 (Middleware-based authentication with Zero Trust)
//
// The ClusterRole is applied from deploy/data-storage/client-rbac-v2.yaml, which contains:
//   - ClusterRole: data-storage-client (verbs: ["create", "get", "list", "update", "delete"])
//   - RoleBindings for production services (Gateway, RO, SP, AA, WE, Notification, etc.)
//
// Note: E2E tests create their own RoleBindings programmatically (not from manifest).
func deployDataStorageClientClusterRole(ctx context.Context, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	rbacManifest := filepath.Join(workspaceRoot, "deploy", "data-storage", "client-rbac-v2.yaml")
	if _, err := os.Stat(rbacManifest); os.IsNotExist(err) {
		return fmt.Errorf("ClusterRole manifest not found at %s", rbacManifest)
	}

	// Apply only ClusterRole (skip RoleBindings which reference kubernaut-system namespace)
	// E2E tests create dynamic RoleBindings programmatically as needed
	// Use yq to extract only the ClusterRole (second document in manifest)
	applyCmd := exec.CommandContext(ctx, "sh", "-c",
		fmt.Sprintf("yq eval 'select(.kind == \"ClusterRole\")' %s | kubectl apply --kubeconfig %s --server-side --field-manager=e2e-test -f -",
			rbacManifest, kubeconfigPath))
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply ClusterRole: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ ClusterRole 'data-storage-client' deployed (verbs: create, get, list, update, delete)\n")
	return nil
}

// deployDataStorageServiceRBAC deploys the ServiceAccount, ClusterRole, and ClusterRoleBinding
// required for DataStorage's auth middleware to call TokenReview and SubjectAccessReview APIs.
//
// Authority: DD-AUTH-014 (Middleware-based authentication)
//
// The manifest contains:
//   - ServiceAccount: data-storage-sa
//   - ClusterRole: data-storage-auth-middleware (tokenreviews, subjectaccessreviews)
//   - ClusterRoleBinding: Binds SA to ClusterRole
//
// Without this RBAC, DataStorage cannot validate tokens or check permissions.
func deployDataStorageServiceRBAC(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	rbacManifest := filepath.Join(workspaceRoot, "deploy", "data-storage", "service-rbac.yaml")
	if _, err := os.Stat(rbacManifest); os.IsNotExist(err) {
		return fmt.Errorf("service RBAC manifest not found at %s", rbacManifest)
	}

	// Apply manifest with namespace substitution for ServiceAccount
	applyCmd := exec.CommandContext(ctx, "sh", "-c",
		fmt.Sprintf("sed 's/namespace: kubernaut-system/namespace: %s/' %s | kubectl apply --kubeconfig %s --server-side --field-manager=e2e-test -f -",
			namespace, rbacManifest, kubeconfigPath))
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply service RBAC: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ DataStorage service RBAC deployed (TokenReview + SAR permissions)\n")
	return nil
}

func deployPostgreSQLInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// 1. Create Secret for credentials
	// Note: PostgreSQL docker entrypoint auto-creates user and database from env vars
	// No init script needed - POSTGRES_USER gets ownership of POSTGRES_DB automatically
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql-secret",
			Namespace: namespace,
		},
		StringData: map[string]string{
			"POSTGRES_USER":     "slm_user",
			"POSTGRES_PASSWORD": "test_password",
			"POSTGRES_DB":       "action_history",
		},
	}

	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		// Handle case where secret already exists (from previous test run)
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create PostgreSQL secret: %w", err)
		}
		// Secret exists, update it to ensure it has correct values
		_, err = clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update existing PostgreSQL secret: %w", err)
		}
	}

	// 2. Create Service (NodePort for direct access from host - eliminates port-forward instability)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "postgresql",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:       "postgresql",
					Port:       5432,
					TargetPort: intstr.FromInt(5432),
					NodePort:   30432, // Mapped to localhost:5432 via Kind extraPortMappings
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "postgresql",
			},
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL service: %w", err)
	}

	// 3. Create Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "postgresql",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "postgresql",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "postgresql",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "postgresql",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "postgresql",
							Image: "postgres:16-alpine",
							Ports: []corev1.ContainerPort{
								{
									Name:          "postgresql",
									ContainerPort: 5432,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "POSTGRES_USER",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "postgresql-secret",
											},
											Key: "POSTGRES_USER",
										},
									},
								},
								{
									Name: "POSTGRES_PASSWORD",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "postgresql-secret",
											},
											Key: "POSTGRES_PASSWORD",
										},
									},
								},
								{
									Name: "POSTGRES_DB",
									ValueFrom: &corev1.EnvVarSource{
										SecretKeyRef: &corev1.SecretKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "postgresql-secret",
											},
											Key: "POSTGRES_DB",
										},
									},
								},
								{
									Name:  "PGDATA",
									Value: "/var/lib/postgresql/data/pgdata",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "postgresql-data",
									MountPath: "/var/lib/postgresql/data",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("250m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"pg_isready", "-U", "slm_user", "-d", "action_history"},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"pg_isready", "-U", "slm_user", "-d", "action_history"},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "postgresql-data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create PostgreSQL deployment: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ PostgreSQL deployed (Secret + Service + Deployment)\n")
	_, _ = fmt.Fprintf(writer, "   ℹ️  User and database auto-created by PostgreSQL entrypoint\n")
	return nil
}

func deployRedisInNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// 1. Create Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "redis",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "redis",
					Port:       6379,
					TargetPort: intstr.FromInt(6379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "redis",
			},
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Redis service: %w", err)
	}

	// 2. Create Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "redis",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "redis",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "redis",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "redis",
							Image: "quay.io/jordigilh/redis:7-alpine",
							Ports: []corev1.ContainerPort{
								{
									Name:          "redis",
									ContainerPort: 6379,
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("200m"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "ping"},
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{"redis-cli", "ping"},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
							},
						},
					},
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Redis deployment: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Redis deployed (Service + Deployment)\n")
	return nil
}

// ApplyMigrations is an exported wrapper for applying ALL migrations to a namespace.
// This is useful for re-applying migrations after PostgreSQL restarts (e.g., in DLQ tests).
//
// DEPRECATED: Use ApplyAllMigrations() for DS full schema, or ApplyAuditMigrations() for audit-only.
// This function is kept for backward compatibility.
func ApplyMigrations(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// Delegate to shared migration library
	return ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)
}

// deployDataStorageServiceInNamespace deploys DataStorage with OAuth2-Proxy sidecar using default NodePort.
// TD-E2E-001 Phase 1: All E2E deployments now include oauth2-proxy for SOC2 architecture parity.
// OAuth2-Proxy image pulled automatically from quay.io (public registry).
func deployDataStorageServiceInNamespace(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, writer io.Writer) error {
	return deployDataStorageServiceInNamespaceWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, 30081, writer)
}

// deployDataStorageServiceInNamespaceWithNodePort deploys DataStorage with OAuth2-Proxy sidecar.
// Architecture: Direct access via DD-AUTH-014 (no oauth-proxy)
// DD-AUTH-010: Real authentication with ServiceAccount tokens (no pass-through mode)
// DD-AUTH-011: SubjectAccessReview (SAR) with verb:"create" for all DataStorage operations
// DD-AUTH-009 v2.0: OpenShift oauth-proxy (NOT CNCF oauth2-proxy)
// DD-AUTH-014: Middleware-based SAR authentication (no oauth-proxy sidecar)
func deployDataStorageServiceInNamespaceWithNodePort(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, nodePort int32, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "📦 Deploying DataStorage with middleware-based auth (DD-AUTH-014)...\n")

	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// 1. Create ConfigMap for service configuration
	configYAML := fmt.Sprintf(`service:
  name: data-storage
  metricsPort: 9181
  logLevel: debug
  shutdownTimeout: 30s
server:
  port: 8080  # DD-AUTH-014: Direct access (no oauth-proxy)
  host: "0.0.0.0"
  read_timeout: 30s
  write_timeout: 30s
database:
  host: postgresql.%s.svc.cluster.local
  port: 5432
  name: action_history
  user: slm_user
  sslMode: disable
  maxOpenConns: 50
  maxIdleConns: 10
  connMaxLifetime: 1h
  connMaxIdleTime: 10m
  secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
  usernameKey: "username"
  passwordKey: "password"
redis:
  addr: redis.%s.svc.cluster.local:6379
  db: 0
  dlqStreamName: dlq-stream
  dlqMaxLen: 1000
  dlqConsumerGroup: dlq-group
  secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
  passwordKey: "password"
logging:
  level: debug
  format: json`, namespace, namespace)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage-config",
			Namespace: namespace,
		},
		Data: map[string]string{
			"config.yaml": configYAML,
		},
	}

	_, err = clientset.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage ConfigMap: %w", err)
	}

	// 2. Create Secret for database and Redis credentials
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage-secret",
			Namespace: namespace,
		},
		StringData: map[string]string{
			"db-secrets.yaml": `username: slm_user
password: test_password`,
			"redis-secrets.yaml": `password: ""`,
		},
	}

	_, err = clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		// Handle case where secret already exists (from previous test run)
		if !errors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create Data Storage Secret: %w", err)
		}
		// Secret exists, update it to ensure it has correct values
		_, err = clientset.CoreV1().Secrets(namespace).Update(ctx, secret, metav1.UpdateOptions{})
		if err != nil {
			return fmt.Errorf("failed to update existing Data Storage Secret: %w", err)
		}
	}

	// 2.5. Create ServiceAccount + RBAC for middleware-based auth (DD-AUTH-014)
	// Required for TokenReview and SubjectAccessReview API calls
	_, _ = fmt.Fprintf(writer, "   🔐 Creating DataStorage ServiceAccount + RBAC...\n")

	// ServiceAccount
	serviceAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-storage-sa",
			Namespace: namespace,
			Labels: map[string]string{
				"app":           "data-storage-service",
				"component":     "auth",
				"authorization": "dd-auth-014",
			},
		},
	}
	_, err = clientset.CoreV1().ServiceAccounts(namespace).Create(ctx, serviceAccount, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create DataStorage ServiceAccount: %w", err)
	}

	// ClusterRole for TokenReview + SubjectAccessReview
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: "data-storage-auth-middleware",
			Labels: map[string]string{
				"app":           "data-storage-service",
				"component":     "auth",
				"authorization": "dd-auth-014",
			},
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"authentication.k8s.io"},
				Resources: []string{"tokenreviews"},
				Verbs:     []string{"create"},
			},
			{
				APIGroups: []string{"authorization.k8s.io"},
				Resources: []string{"subjectaccessreviews"},
				Verbs:     []string{"create"},
			},
		},
	}
	_, err = clientset.RbacV1().ClusterRoles().Create(ctx, clusterRole, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create DataStorage ClusterRole: %w", err)
	}

	// ClusterRoleBinding
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "data-storage-auth-middleware",
			Labels: map[string]string{
				"app":           "data-storage-service",
				"component":     "auth",
				"authorization": "dd-auth-014",
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "data-storage-auth-middleware",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "data-storage-sa",
				Namespace: namespace,
			},
		},
	}
	_, err = clientset.RbacV1().ClusterRoleBindings().Create(ctx, clusterRoleBinding, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("failed to create DataStorage ClusterRoleBinding: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "     ✅ DataStorage RBAC configured\n")

	// 3. Create Service (NodePort for direct access from host - eliminates port-forward instability)
	// DD-AUTH-014: Direct to DataStorage (no oauth-proxy)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-storage-service", // DD-AUTH-011: Match production service name for SAR
			Namespace: namespace,
			Labels: map[string]string{
				"app": "datastorage",
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeNodePort,
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,                 // DD-AUTH-014: Direct to DataStorage (no proxy)
					TargetPort: intstr.FromInt(8080), // Maps to DataStorage container port
					NodePort:   nodePort,             // Configurable per service (default: 30081)
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "metrics",
					Port:       9181,
					TargetPort: intstr.FromInt(9181),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": "datastorage",
			},
		},
	}

	_, err = clientset.CoreV1().Services(namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage Service: %w", err)
	}

	// 4. Create Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage",
			Namespace: namespace,
			Labels: map[string]string{
				"app": "datastorage",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "datastorage",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "datastorage",
					},
				},
				Spec: corev1.PodSpec{
					// DD-AUTH-014: ServiceAccount for middleware-based auth
					// Required for TokenReview and SubjectAccessReview API calls
					ServiceAccountName: "data-storage-sa",
					// DD-TEST-001: Schedule on control-plane where images are loaded
					// Kind loads images to control-plane only by default
					NodeSelector: map[string]string{
						"node-role.kubernetes.io/control-plane": "",
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "node-role.kubernetes.io/control-plane",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					// DD-TEST-007: Run as root for E2E coverage (simplified permissions)
					// Per SP team guidance: non-root user may not have permission to write /coverdata
					SecurityContext: func() *corev1.PodSecurityContext {
						if os.Getenv("E2E_COVERAGE") == "true" {
							runAsUser := int64(0)
							runAsGroup := int64(0)
							return &corev1.PodSecurityContext{
								RunAsUser:  &runAsUser,
								RunAsGroup: &runAsGroup,
							}
						}
						return nil
					}(),
					Containers: []corev1.Container{
						// DD-AUTH-014: DataStorage with middleware-based auth (no oauth-proxy sidecar)
						// Authenticates using Kubernetes TokenReview API + authorizes using SAR API
						{
							Name:            "datastorage",
							Image:           dataStorageImage,       // DD-TEST-001: service-specific tag
							ImagePullPolicy: GetImagePullPolicyV1(), // Dynamic: IfNotPresent (CI/CD) or Never (local)
							Ports: []corev1.ContainerPort{
								{
									Name:          "http", // DD-AUTH-014: Direct access (no proxy)
									ContainerPort: 8080,   // Internal port (matches config.yaml)
								},
								{
									Name:          "metrics",
									ContainerPort: 9181,
								},
							},
							Env: func() []corev1.EnvVar {
								envVars := []corev1.EnvVar{
									{
										Name:  "CONFIG_PATH",
										Value: "/etc/datastorage/config.yaml",
									},
									// DD-AUTH-014: POD_NAMESPACE required for SAR namespace context
									{
										Name:  "POD_NAMESPACE",
										Value: namespace,
									},
									// DD-AUTH-014: Use in-cluster config (ServiceAccount mounted automatically)
									// KUBECONFIG env var removed - was causing crashes in E2E (host path doesn't exist in container)
									// With proper data-storage-sa ServiceAccount + RBAC, in-cluster config works correctly
								}
								// DD-TEST-007: E2E Coverage Capture Standard
								// Only add GOCOVERDIR if E2E_COVERAGE=true
								// MUST match Kind extraMounts path: /coverdata (not /tmp/coverage)
								coverageEnabled := os.Getenv("E2E_COVERAGE") == "true"
								_, _ = fmt.Fprintf(writer, "   🔍 DD-TEST-007: E2E_COVERAGE=%s (enabled=%v)\n", os.Getenv("E2E_COVERAGE"), coverageEnabled)
								if coverageEnabled {
									_, _ = fmt.Fprintf(writer, "   ✅ Adding GOCOVERDIR=/coverdata to DataStorage deployment\n")
									envVars = append(envVars, corev1.EnvVar{
										Name:  "GOCOVERDIR",
										Value: "/coverdata",
									})
								} else {
									_, _ = fmt.Fprintf(writer, "   ⚠️  E2E_COVERAGE not set, skipping GOCOVERDIR\n")
								}
								_, _ = fmt.Fprintf(writer, "   ✅ DD-AUTH-014: Using in-cluster config with ServiceAccount, POD_NAMESPACE=%s\n", namespace)
								return envVars
							}(),
							VolumeMounts: func() []corev1.VolumeMount {
								mounts := []corev1.VolumeMount{
									{
										Name:      "config",
										MountPath: "/etc/datastorage",
										ReadOnly:  true,
									},
									{
										Name:      "secrets",
										MountPath: "/etc/datastorage/secrets",
										ReadOnly:  true,
									},
								}
								// DD-TEST-007: Add coverage volume mount if enabled
								// MUST match Kind extraMounts path: /coverdata (not /tmp/coverage)
								if os.Getenv("E2E_COVERAGE") == "true" {
									mounts = append(mounts, corev1.VolumeMount{
										Name:      "coverage",
										MountPath: "/coverdata",
										ReadOnly:  false,
									})
								}
								return mounts
							}(),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("256Mi"),
									corev1.ResourceCPU:    resource.MustParse("250m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(8080), // DataStorage listens on 8080
									},
								},
								InitialDelaySeconds: 30, // Allow PostgreSQL/Redis startup (was 5s - too short)
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
								FailureThreshold:    3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(8080), // DataStorage listens on 8080
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
						},
					},
					Volumes: func() []corev1.Volume {
						volumes := []corev1.Volume{
							{
								Name: "config",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: "datastorage-config",
										},
									},
								},
							},
							{
								Name: "secrets",
								VolumeSource: corev1.VolumeSource{
									Secret: &corev1.SecretVolumeSource{
										SecretName: "datastorage-secret",
									},
								},
							},
						}
						// DD-TEST-007: Add hostPath volume for coverage if enabled
						// MUST match Kind extraMounts path: /coverdata (not /tmp/coverage)
						if os.Getenv("E2E_COVERAGE") == "true" {
							volumes = append(volumes, corev1.Volume{
								Name: "coverage",
								VolumeSource: corev1.VolumeSource{
									HostPath: &corev1.HostPathVolumeSource{
										Path: "/coverdata",
										Type: func() *corev1.HostPathType {
											t := corev1.HostPathDirectoryOrCreate
											return &t
										}(),
									},
								},
							})
						}
						return volumes
					}(),
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage Deployment: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ✅ Data Storage Service deployed (ConfigMap + Secret + Service + Deployment)\n")
	return nil
}

func waitForDataStorageServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// Wait for PostgreSQL pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for PostgreSQL pod to be ready...\n")
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
	}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "PostgreSQL pod should be ready")
	_, _ = fmt.Fprintf(writer, "   ✅ PostgreSQL pod ready\n")

	// Wait for Redis pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for Redis pod to be ready...\n")
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
	}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "Redis pod should be ready")
	_, _ = fmt.Fprintf(writer, "   ✅ Redis pod ready\n")

	// Wait for Data Storage Service pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for Data Storage Service pod to be ready...\n")
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
	}, 5*time.Minute, 5*time.Second).Should(BeTrue(), "Data Storage Service pod should be ready")
	_, _ = fmt.Fprintf(writer, "   ✅ Data Storage Service pod ready\n")

	return nil
}

func createKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	// REFACTORED: Now uses shared CreateKindClusterWithConfig() helper
	// Authority: docs/handoff/TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md (Phase 1)
	opts := KindClusterOptions{
		ClusterName:    clusterName,
		KubeconfigPath: kubeconfigPath,
		ConfigPath:     "test/infrastructure/kind-datastorage-config.yaml",
		WaitTimeout:    "60s",
		DeleteExisting: true, // Original behavior: delete if exists
		ReuseExisting:  false,
	}
	return CreateKindClusterWithConfig(opts, writer)
}

func buildDataStorageImage(writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Build Data Storage image using Podman (following Gateway pattern)
	// CRITICAL: --no-cache ensures latest code changes are included (DD-TEST-002)
	buildArgs := []string{
		"build",
		"--no-cache",                                                 // Force fresh build to include latest code changes
		"-t", "localhost/kubernaut-datastorage:e2e-test-datastorage", // DD-TEST-001: service-specific tag
		"-f", "docker/data-storage.Dockerfile",
	}

	// E2E Coverage Collection (E2E_COVERAGE_COLLECTION.md)
	// If E2E_COVERAGE=true, build with coverage instrumentation
	if os.Getenv("E2E_COVERAGE") == "true" {
		buildArgs = append(buildArgs, "--build-arg", "GOFLAGS=-cover")
		_, _ = fmt.Fprintln(writer, "   📊 Building with coverage instrumentation (GOFLAGS=-cover)")
	}

	buildArgs = append(buildArgs, ".")

	buildCmd := exec.Command("podman", buildArgs...)
	buildCmd.Dir = workspaceRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("podman build failed: %w", err)
	}

	// Tag image for SP E2E compatibility (SP expects e2e-test tag)
	tagCmd := exec.Command("podman", "tag", "localhost/kubernaut-datastorage:e2e-test-datastorage", "localhost/kubernaut-datastorage:e2e-test")
	tagCmd.Stdout = writer
	tagCmd.Stderr = writer
	if err := tagCmd.Run(); err != nil {
		return fmt.Errorf("podman tag failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "   Data Storage image built: localhost/kubernaut-datastorage:e2e-test-datastorage")
	_, _ = fmt.Fprintln(writer, "   Data Storage image tagged: localhost/kubernaut-datastorage:e2e-test (SP E2E compatibility)")

	// PROFILING: Get image size for optimization analysis
	sizeCmd := exec.Command("podman", "images", "--format", "{{.Size}}", "localhost/kubernaut-datastorage:e2e-test-datastorage")
	sizeOutput, err := sizeCmd.Output()
	if err == nil {
		_, _ = fmt.Fprintf(writer, "   📊 Image size: %s\n", string(sizeOutput))
	}

	return nil
}

func loadDataStorageImage(clusterName string, writer io.Writer) error {
	// Save image to tar (following Gateway pattern)
	// DD-TEST-001: Use service-specific tag
	saveCmd := exec.Command("podman", "save", "localhost/kubernaut-datastorage:e2e-test-datastorage", "-o", "/tmp/datastorage-e2e.tar")
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load image into Kind cluster
	loadCmd := exec.Command("kind", "load", "image-archive", "/tmp/datastorage-e2e.tar", "--name", clusterName)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer

	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image into Kind: %w", err)
	}

	// Clean up tar file
	_ = os.Remove("/tmp/datastorage-e2e.tar")

	// CRITICAL: Remove Podman image immediately to free disk space
	// Image is now in Kind, Podman copy is duplicate
	_, _ = fmt.Fprintln(writer, "   🗑️  Removing Podman image to free disk space...")
	rmiCmd := exec.Command("podman", "rmi", "-f", "localhost/kubernaut-datastorage:e2e-test-datastorage")
	rmiCmd.Stdout = writer
	rmiCmd.Stderr = writer
	if err := rmiCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ⚠️  Failed to remove Podman image (non-fatal): %v\n", err)
	} else {
		_, _ = fmt.Fprintln(writer, "   ✅ Podman image removed: localhost/kubernaut-datastorage:e2e-test-datastorage")
	}

	_, _ = fmt.Fprintln(writer, "   Data Storage image loaded into Kind cluster")
	return nil
}

// DataStorageInfrastructure manages the Data Storage Service test infrastructure
// This includes PostgreSQL, Redis, and the Data Storage Service itself
type DataStorageInfrastructure struct {
	PostgresContainer string
	RedisContainer    string
	ServiceContainer  string
	ConfigDir         string
	ServiceURL        string
	DB                *sql.DB
	RedisClient       *redis.Client
}

// DataStorageConfig contains configuration for the Data Storage Service
type DataStorageConfig struct {
	PostgresPort string // Default: "5433"
	RedisPort    string // Default: "6380"
	ServicePort  string // Default: "8085"
	DBName       string // Default: "action_history"
	DBUser       string // Default: "slm_user"
	DBPassword   string // Default: "test_password"
}

// DefaultDataStorageConfig returns default configuration
func DefaultDataStorageConfig() *DataStorageConfig {
	return &DataStorageConfig{
		PostgresPort: "5433",
		RedisPort:    "6380",
		ServicePort:  "8085",
		DBName:       "action_history",
		DBUser:       "slm_user",
		DBPassword:   "test_password",
	}
}

// StartDataStorageInfrastructure starts all Data Storage Service infrastructure
// Returns an infrastructure handle that can be used to stop the services
func StartDataStorageInfrastructure(cfg *DataStorageConfig, writer io.Writer) (*DataStorageInfrastructure, error) {
	if cfg == nil {
		cfg = DefaultDataStorageConfig()
	}

	infra := &DataStorageInfrastructure{
		PostgresContainer: "datastorage-postgres-test",
		RedisContainer:    "datastorage-redis-test",
		ServiceContainer:  "datastorage-service-test",
		ServiceURL:        fmt.Sprintf("http://localhost:%s", cfg.ServicePort),
	}

	_, _ = fmt.Fprintln(writer, "🔧 Setting up Data Storage Service infrastructure (ADR-016: Podman)")

	// 1. Start PostgreSQL
	_, _ = fmt.Fprintln(writer, "📦 Starting PostgreSQL container...")
	if err := startPostgreSQL(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start PostgreSQL: %w", err)
	}

	// 2. Start Redis
	_, _ = fmt.Fprintln(writer, "📦 Starting Redis container...")
	if err := startRedis(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start Redis: %w", err)
	}

	// 3. Connect to PostgreSQL
	_, _ = fmt.Fprintln(writer, "🔌 Connecting to PostgreSQL...")
	if err := connectPostgreSQL(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL: %w", err)
	}

	// 4. Apply migrations
	_, _ = fmt.Fprintln(writer, "📋 Applying schema migrations...")
	if err := applyMigrations(infra, writer); err != nil {
		return nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	// 5. Connect to Redis
	_, _ = fmt.Fprintln(writer, "🔌 Connecting to Redis...")
	if err := connectRedis(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// 6. Create config files
	_, _ = fmt.Fprintln(writer, "📝 Creating ADR-030 config files...")
	if err := createConfigFiles(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to create config files: %w", err)
	}

	// 7. Build Data Storage Service image
	_, _ = fmt.Fprintln(writer, "🏗️  Building Data Storage Service image...")
	if err := buildDataStorageService(writer); err != nil {
		return nil, fmt.Errorf("failed to build service: %w", err)
	}

	// 8. Start Data Storage Service
	_, _ = fmt.Fprintln(writer, "🚀 Starting Data Storage Service container...")
	if err := startDataStorageService(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start service: %w", err)
	}

	// 9. Wait for service to be ready
	_, _ = fmt.Fprintln(writer, "⏳ Waiting for Data Storage Service to be ready...")
	if err := waitForServiceReady(infra, writer); err != nil {
		return nil, fmt.Errorf("service not ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Data Storage Service infrastructure ready!")
	return infra, nil
}

// StopDataStorageInfrastructure stops all Data Storage Service infrastructure
func (infra *DataStorageInfrastructure) Stop(writer io.Writer) {
	_, _ = fmt.Fprintln(writer, "🧹 Cleaning up Data Storage Service infrastructure...")

	// Close connections
	if infra.DB != nil {
		_ = infra.DB.Close()
	}
	if infra.RedisClient != nil {
		_ = infra.RedisClient.Close()
	}

	// Stop and remove containers
	_ = exec.Command("podman", "stop", infra.ServiceContainer).Run()
	_ = exec.Command("podman", "rm", infra.ServiceContainer).Run()
	_ = exec.Command("podman", "stop", infra.PostgresContainer).Run()
	_ = exec.Command("podman", "rm", infra.PostgresContainer).Run()
	_ = exec.Command("podman", "stop", infra.RedisContainer).Run()
	_ = exec.Command("podman", "rm", infra.RedisContainer).Run()

	// Remove config directory
	if infra.ConfigDir != "" {
		_ = os.RemoveAll(infra.ConfigDir)
	}

	_, _ = fmt.Fprintln(writer, "✅ Data Storage Service infrastructure cleanup complete")
}

// Helper functions

func startPostgreSQL(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	_ = exec.Command("podman", "stop", infra.PostgresContainer).Run()
	_ = exec.Command("podman", "rm", infra.PostgresContainer).Run()

	// Start PostgreSQL
	cmd := exec.Command("podman", "run", "-d",
		"--name", infra.PostgresContainer,
		"-p", fmt.Sprintf("%s:5432", cfg.PostgresPort),
		"-e", fmt.Sprintf("POSTGRES_DB=%s", cfg.DBName),
		"-e", fmt.Sprintf("POSTGRES_USER=%s", cfg.DBUser),
		"-e", fmt.Sprintf("POSTGRES_PASSWORD=%s", cfg.DBPassword),
		"postgres:16-alpine")

	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Failed to start PostgreSQL: %s\n", output)
		return fmt.Errorf("PostgreSQL container failed to start: %w", err)
	}

	// Wait for PostgreSQL ready
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for PostgreSQL to be ready...")
	time.Sleep(3 * time.Second)

	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", infra.PostgresContainer, "pg_isready", "-U", cfg.DBUser)
		return testCmd.Run()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be ready")

	_, _ = fmt.Fprintln(writer, "  ✅ PostgreSQL started successfully")
	return nil
}

func startRedis(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	_ = exec.Command("podman", "stop", infra.RedisContainer).Run()
	_ = exec.Command("podman", "rm", infra.RedisContainer).Run()

	// Start Redis
	cmd := exec.Command("podman", "run", "-d",
		"--name", infra.RedisContainer,
		"-p", fmt.Sprintf("%s:6379", cfg.RedisPort),
		"quay.io/jordigilh/redis:7-alpine")

	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Failed to start Redis: %s\n", output)
		return fmt.Errorf("Redis container failed to start: %w", err)
	}

	// Wait for Redis ready
	time.Sleep(2 * time.Second)

	Eventually(func() error {
		testCmd := exec.Command("podman", "exec", infra.RedisContainer, "redis-cli", "ping")
		testOutput, err := testCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("Redis not ready: %v, output: %s", err, string(testOutput))
		}
		return nil
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "Redis should be ready")

	_, _ = fmt.Fprintln(writer, "  ✅ Redis started successfully")
	return nil
}

func connectPostgreSQL(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	connStr := fmt.Sprintf("host=localhost port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.PostgresPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)

	var err error
	infra.DB, err = sql.Open("pgx", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Wait for connection
	Eventually(func() error {
		return infra.DB.Ping()
	}, 30*time.Second, 1*time.Second).Should(Succeed(), "PostgreSQL should be connectable")

	_, _ = fmt.Fprintln(writer, "  ✅ PostgreSQL connection established")
	return nil
}

func applyMigrations(infra *DataStorageInfrastructure, writer io.Writer) error {
	// Drop and recreate schema
	_, _ = fmt.Fprintln(writer, "  🗑️  Dropping existing schema...")
	_, err := infra.DB.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;")
	if err != nil {
		return fmt.Errorf("failed to drop schema: %w", err)
	}

	// Apply migrations
	_, _ = fmt.Fprintln(writer, "  📜 Applying V1.0 migrations (label-only, no embeddings)...")
	// V1.0 Migration List (label-only architecture per DD-WORKFLOW-015)
	// Vector-dependent migrations (005, 007-010, 016) removed per TRIAGE_DS_MIGRATION_DEPENDENCIES_V1.0.md
	migrations := []string{
		"001_initial_schema.sql",
		"002_fix_partitioning.sql",
		"003_stored_procedures.sql",
		"004_add_effectiveness_assessment_due.sql",
		// NOTE: Migration 006 moved to migrations/v1.1/ (v1.1 feature, removed 2026-01-07)
		"012_adr033_multidimensional_tracking.sql",
		"013_create_audit_events_table.sql",
		"017_add_workflow_schema_fields.sql",
		"018_rename_execution_bundle_to_container_image.sql",
		"019_uuid_primary_key.sql",
		"020_add_workflow_label_columns.sql", // DD-WORKFLOW-001 v1.6: custom_labels + detected_labels
		"1000_create_audit_events_partitions.sql",
	}

	// Find workspace root once (project root with go.mod)
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	for _, migration := range migrations {
		// Use absolute path from project root (no relative path issues)
		migrationPath := filepath.Join(workspaceRoot, "migrations", migration)

		content, err := os.ReadFile(migrationPath)
		if err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ Migration file not found at %s: %v\n", migrationPath, err)
			return fmt.Errorf("migration file %s not found at %s: %w", migration, migrationPath, err)
		}

		// Remove CONCURRENTLY keyword for test environment
		migrationSQL := strings.ReplaceAll(string(content), "CONCURRENTLY ", "")

		// Extract only the UP migration (ignore DOWN section)
		if strings.Contains(migrationSQL, "-- +goose Down") {
			parts := strings.Split(migrationSQL, "-- +goose Down")
			migrationSQL = parts[0]
		}

		_, err = infra.DB.Exec(migrationSQL)
		if err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ Migration %s failed: %v\n", migration, err)
			return fmt.Errorf("migration %s failed: %w", migration, err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ Applied %s\n", migration)
	}

	// Grant permissions
	_, _ = fmt.Fprintln(writer, "  🔐 Granting permissions...")
	_, err = infra.DB.Exec(`
		GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO slm_user;
		GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO slm_user;
		GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA public TO slm_user;
	`)
	if err != nil {
		return fmt.Errorf("failed to grant permissions: %w", err)
	}

	// Wait for schema propagation
	_, _ = fmt.Fprintln(writer, "  ⏳ Waiting for PostgreSQL schema propagation (2s)...")
	time.Sleep(2 * time.Second)

	_, _ = fmt.Fprintln(writer, "  ✅ All migrations applied successfully")
	return nil
}

func connectRedis(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	infra.RedisClient = redis.NewClient(&redis.Options{
		Addr: fmt.Sprintf("localhost:%s", cfg.RedisPort),
		DB:   0,
	})

	// Verify connection
	err := infra.RedisClient.Ping(context.Background()).Err()
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Redis connection established")
	return nil
}

func createConfigFiles(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	var err error
	infra.ConfigDir, err = os.MkdirTemp("", "datastorage-config-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	// Get container IPs
	postgresIP := getContainerIP(infra.PostgresContainer)
	redisIP := getContainerIP(infra.RedisContainer)

	// Create config.yaml (ADR-030)
	configYAML := fmt.Sprintf(`
service:
  name: data-storage
  metricsPort: 9181
  logLevel: debug
  shutdownTimeout: 30s
server:
  port: 8080  # DD-AUTH-014: Direct access (no oauth-proxy)
  host: "0.0.0.0"
  read_timeout: 30s
  write_timeout: 30s
database:
  host: %s
  port: 5432
  name: %s
  user: %s
  sslMode: disable
  maxOpenConns: 50
  maxIdleConns: 10
  connMaxLifetime: 1h
  connMaxIdleTime: 10m
  secretsFile: "/etc/datastorage/secrets/db-secrets.yaml"
  usernameKey: "username"
  passwordKey: "password"
redis:
  addr: %s:6379
  db: 0
  dlqStreamName: dlq-stream
  dlqMaxLen: 1000
  dlqConsumerGroup: dlq-group
  secretsFile: "/etc/datastorage/secrets/redis-secrets.yaml"
  passwordKey: "password"
logging:
  level: debug
  format: json
`, postgresIP, cfg.DBName, cfg.DBUser, redisIP)

	configPath := filepath.Join(infra.ConfigDir, "config.yaml")
	err = os.WriteFile(configPath, []byte(configYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write config.yaml: %w", err)
	}

	// Create database secrets file
	dbSecretsYAML := fmt.Sprintf(`
username: %s
password: %s
`, cfg.DBUser, cfg.DBPassword)
	dbSecretsPath := filepath.Join(infra.ConfigDir, "db-secrets.yaml")
	err = os.WriteFile(dbSecretsPath, []byte(dbSecretsYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write db-secrets.yaml: %w", err)
	}

	// Create Redis secrets file
	redisSecretsYAML := `password: ""` // Redis without auth in test
	redisSecretsPath := filepath.Join(infra.ConfigDir, "redis-secrets.yaml")
	err = os.WriteFile(redisSecretsPath, []byte(redisSecretsYAML), 0644)
	if err != nil {
		return fmt.Errorf("failed to write redis-secrets.yaml: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Config files created in %s\n", infra.ConfigDir)
	return nil
}

func buildDataStorageService(writer io.Writer) error {
	// Find workspace root (go.mod location)
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Cleanup any existing image
	_ = exec.Command("podman", "rmi", "-f", "data-storage:test").Run()

	// Build image for ARM64 (local testing on Apple Silicon)
	// CRITICAL: --no-cache ensures latest code changes are included (DD-TEST-002)
	buildCmd := exec.Command("podman", "build",
		"--no-cache", // Force fresh build to include latest code changes
		"--build-arg", "GOARCH=arm64",
		"-t", "data-storage:test",
		"-f", "docker/data-storage.Dockerfile",
		".")
	buildCmd.Dir = workspaceRoot // Run from workspace root

	output, err := buildCmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Build output:\n%s\n", string(output))
		return fmt.Errorf("failed to build Data Storage Service image: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Data Storage Service image built successfully")
	return nil
}

// findWorkspaceRoot finds the workspace root by looking for go.mod
func findWorkspaceRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Walk up the directory tree looking for go.mod
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("could not find go.mod in any parent directory")
		}
		dir = parent
	}
}

func startDataStorageService(infra *DataStorageInfrastructure, cfg *DataStorageConfig, writer io.Writer) error {
	// Cleanup existing container
	_ = exec.Command("podman", "stop", infra.ServiceContainer).Run()
	_ = exec.Command("podman", "rm", infra.ServiceContainer).Run()

	// Mount config files (ADR-030)
	configMount := fmt.Sprintf("%s/config.yaml:/etc/datastorage/config.yaml:ro", infra.ConfigDir)
	secretsMount := fmt.Sprintf("%s:/etc/datastorage/secrets:ro", infra.ConfigDir)

	// Start service container with ADR-030 config
	startCmd := exec.Command("podman", "run", "-d",
		"--name", infra.ServiceContainer,
		"-p", fmt.Sprintf("%s:8080", cfg.ServicePort),
		"-v", configMount,
		"-v", secretsMount,
		"-e", "CONFIG_PATH=/etc/datastorage/config.yaml",
		"data-storage:test")

	output, err := startCmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "❌ Start output:\n%s\n", string(output))
		return fmt.Errorf("failed to start Data Storage Service container: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Data Storage Service container started")
	return nil
}

func waitForServiceReady(infra *DataStorageInfrastructure, writer io.Writer) error {
	// Wait up to 30 seconds for service to be ready
	var lastStatusCode int
	var lastError error

	Eventually(func() int {
		resp, err := http.Get(infra.ServiceURL + "/health")
		if err != nil {
			lastError = err
			lastStatusCode = 0
			_, _ = fmt.Fprintf(writer, "    Health check attempt failed: %v\n", err)
			return 0
		}
		if resp == nil {
			lastStatusCode = 0
			return 0
		}
		defer func() { _ = resp.Body.Close() }()
		lastStatusCode = resp.StatusCode
		if lastStatusCode != 200 {
			_, _ = fmt.Fprintf(writer, "    Health check returned status %d (expected 200)\n", lastStatusCode)
		}
		return lastStatusCode
	}, "30s", "1s").Should(Equal(200), "Data Storage Service should be healthy")

	// If we got here and status is not 200, print diagnostics
	if lastStatusCode != 200 {
		_, _ = fmt.Fprintf(writer, "\n❌ Data Storage Service health check failed\n")
		_, _ = fmt.Fprintf(writer, "  Last status code: %d\n", lastStatusCode)
		if lastError != nil {
			_, _ = fmt.Fprintf(writer, "  Last error: %v\n", lastError)
		}

		// Print container logs for debugging
		logs, logErr := exec.Command("podman", "logs", "--tail", "200", infra.ServiceContainer).CombinedOutput()
		if logErr == nil {
			_, _ = fmt.Fprintf(writer, "\n📋 Data Storage Service logs (last 200 lines):\n%s\n", string(logs))
		}

		// Check if container is running
		statusCmd := exec.Command("podman", "ps", "--filter", fmt.Sprintf("name=%s", infra.ServiceContainer), "--format", "{{.Status}}")
		statusOutput, _ := statusCmd.CombinedOutput()
		_, _ = fmt.Fprintf(writer, "  Container status: %s\n", strings.TrimSpace(string(statusOutput)))
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Data Storage Service ready at %s\n", infra.ServiceURL)
	return nil
}

func getContainerIP(containerName string) string {
	cmd := exec.Command("podman", "inspect", "-f", "{{.NetworkSettings.IPAddress}}", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		panic(fmt.Sprintf("Failed to get IP for container %s: %v", containerName, err))
	}
	ip := strings.TrimSpace(string(output))
	if ip == "" {
		panic(fmt.Sprintf("Container %s has no IP address", containerName))
	}
	return ip
}

// ═══════════════════════════════════════════════════════════════════════
// SHARED E2E HELPER FUNCTIONS (Per DD-TEST-001: Fresh builds with dynamic tags)
// ═══════════════════════════════════════════════════════════════════════

// buildDataStorageImageWithTag builds or resolves a DataStorage image for use in integration tests.
//
// CI/CD Optimization:
//   - If IMAGE_REGISTRY + IMAGE_TAG env vars are set: Returns the registry image name directly.
//     Podman will auto-pull from the registry when `podman run` is called with this image name.
//   - Otherwise: Builds locally with --no-cache and returns the original imageTag.
//
// Returns:
//   - string: The actual image name to use in `podman run` (may differ from imageTag in registry mode)
//   - error: Any errors during image build
//
// Per DD-TEST-001: Dynamic tags for parallel E2E isolation
func buildDataStorageImageWithTag(imageTag string, writer io.Writer) (string, error) {
	// CI/CD Optimization: Check if we can use a pre-built image from registry
	registry := os.Getenv("IMAGE_REGISTRY")
	tag := os.Getenv("IMAGE_TAG")
	if registry != "" && tag != "" {
		registryImage := fmt.Sprintf("%s/datastorage:%s", registry, tag)
		_, _ = fmt.Fprintf(writer, "  🔄 Registry mode: IMAGE_REGISTRY=%s IMAGE_TAG=%s\n", registry, tag)
		_, _ = fmt.Fprintf(writer, "  🔍 Verifying DataStorage image in registry: %s\n", registryImage)

		exists, err := VerifyImageExistsInRegistry(registryImage, writer)
		if err == nil && exists {
			_, _ = fmt.Fprintf(writer, "  ✅ DataStorage image found in registry: %s\n", registryImage)
			_, _ = fmt.Fprintf(writer, "  💡 Podman will auto-pull during container start (skipping local build)\n")
			return registryImage, nil
		}

		// Registry verification failed - fall back to local build
		_, _ = fmt.Fprintf(writer, "  ⚠️  Registry verification failed (err=%v, exists=%v), falling back to local build...\n", err, exists)
	}

	// Local build path
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return "", fmt.Errorf("failed to find workspace root: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  🔨 Building DataStorage with tag: %s\n", imageTag)

	// Build Data Storage image using Podman
	// CRITICAL: --no-cache ensures latest code changes are included (DD-TEST-002)
	buildArgs := []string{
		"build",
		"--no-cache",   // Force fresh build to include latest code changes
		"-t", imageTag, // Use dynamic tag for parallel isolation
		"-f", "docker/data-storage.Dockerfile",
	}

	// E2E Coverage Collection (E2E_COVERAGE_COLLECTION.md)
	// If E2E_COVERAGE=true, build with coverage instrumentation
	if os.Getenv("E2E_COVERAGE") == "true" {
		buildArgs = append(buildArgs, "--build-arg", "GOFLAGS=-cover")
		_, _ = fmt.Fprintln(writer, "     📊 Building with coverage instrumentation (GOFLAGS=-cover)")
	}

	buildArgs = append(buildArgs, ".")

	buildCmd := exec.Command("podman", buildArgs...)
	buildCmd.Dir = workspaceRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to build DataStorage image: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "     ✅ DataStorage image built: %s\n", imageTag)
	return imageTag, nil
}

// Removed: loadDataStorageImageWithTag (unused) - E2E tests now use loadImageToKind from shared_integration_utils.go

// InstallCertManager installs cert-manager into the Kind cluster for SOC2 E2E testing.
// This is ONLY needed for DataStorage SOC2 compliance tests to validate production
// certificate management flow.
//
// Installation:
// - Uses official cert-manager v1.13.0+ manifests
// - Installs into cert-manager namespace
// - Requires ~30 seconds for full deployment
//
// Usage: Call ONLY in test/e2e/datastorage/05_soc2_compliance_test.go
func InstallCertManager(kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "📦 Installing cert-manager (SOC2 E2E Only)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Use latest stable cert-manager version
	certManagerURL := "https://github.com/cert-manager/cert-manager/releases/download/v1.13.3/cert-manager.yaml"

	cmd := exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-f", certManagerURL)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install cert-manager: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ cert-manager installed (waiting for readiness...)")
	return nil
}

// WaitForCertManagerReady waits for cert-manager pods to become ready.
// This ensures cert-manager is fully operational before creating Certificate resources.
//
// Waits for:
// - cert-manager controller pod (ready)
// - cert-manager cainjector pod (ready)
// - cert-manager webhook pod (ready)
//
// Timeout: 120 seconds (cert-manager can take 60-90s for webhook registration)
func WaitForCertManagerReady(kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "⏳ Waiting for cert-manager to be ready...")

	// Wait for cert-manager deployment to be available
	checkCmd := exec.Command("kubectl", "wait",
		"--kubeconfig", kubeconfigPath,
		"--namespace", "cert-manager",
		"--for=condition=available",
		"--timeout=120s",
		"deployment/cert-manager",
		"deployment/cert-manager-cainjector",
		"deployment/cert-manager-webhook")
	checkCmd.Stdout = writer
	checkCmd.Stderr = writer

	if err := checkCmd.Run(); err != nil {
		return fmt.Errorf("cert-manager did not become ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ cert-manager is ready")
	return nil
}

// ApplyCertManagerIssuer creates the ClusterIssuer for self-signed certificate generation.
// This is used by DataStorage Certificate resources to request TLS certificates.
//
// Creates:
// - ClusterIssuer "selfsigned-issuer" (self-signed CA)
//
// Note: For production, this would be replaced with Let's Encrypt or organizational CA.
func ApplyCertManagerIssuer(kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "📋 Creating cert-manager ClusterIssuer...")

	// Get workspace root to find the issuer manifest
	workspaceRoot := os.Getenv("WORKSPACE_ROOT")
	if workspaceRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		// Navigate up from test/e2e/datastorage or test/infrastructure to repo root
		workspaceRoot = filepath.Dir(filepath.Dir(filepath.Dir(cwd)))
	}

	issuerPath := filepath.Join(workspaceRoot, "deploy", "cert-manager", "selfsigned-issuer.yaml")

	// Check if issuer manifest exists
	if _, err := os.Stat(issuerPath); os.IsNotExist(err) {
		return fmt.Errorf("ClusterIssuer manifest not found at %s", issuerPath)
	}

	cmd := exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-f", issuerPath)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create ClusterIssuer: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ ClusterIssuer 'selfsigned-issuer' created")
	return nil
}

// DeployCertManagerDataStorage deploys DataStorage with cert-manager Certificate resource.
// This is a specialized deployment function for SOC2 E2E tests that validates the
// production certificate management flow.
//
// Deploys:
// - DataStorage Deployment (with /etc/certs volumeMount)
// - DataStorage Service
// - Certificate resource (cert-manager managed)
//
// The Certificate resource triggers cert-manager to create a Secret with TLS cert/key,
// which DataStorage mounts at /etc/certs for audit export signing.
func DeployCertManagerDataStorage(ctx context.Context, kubeconfigPath, namespace, imageTag string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "📦 Deploying DataStorage with cert-manager (SOC2 E2E)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Get workspace root
	workspaceRoot := os.Getenv("WORKSPACE_ROOT")
	if workspaceRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
		workspaceRoot = filepath.Dir(filepath.Dir(filepath.Dir(cwd)))
	}

	// Apply Certificate resource first (cert-manager will create Secret)
	certPath := filepath.Join(workspaceRoot, "deploy", "data-storage", "certificate.yaml")
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return fmt.Errorf("Certificate manifest not found at %s", certPath)
	}

	_, _ = fmt.Fprintln(writer, "📋 Creating Certificate resource...")
	certCmd := exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-n", namespace,
		"-f", certPath)
	certCmd.Stdout = writer
	certCmd.Stderr = writer

	if err := certCmd.Run(); err != nil {
		return fmt.Errorf("failed to create Certificate: %w", err)
	}

	// Wait for cert-manager to create the Secret
	_, _ = fmt.Fprintln(writer, "⏳ Waiting for cert-manager to issue certificate...")
	waitSecretCmd := exec.Command("kubectl", "wait",
		"--kubeconfig", kubeconfigPath,
		"-n", namespace,
		"--for=condition=Ready",
		"--timeout=60s",
		"certificate/datastorage-signing-cert")
	waitSecretCmd.Stdout = writer
	waitSecretCmd.Stderr = writer

	if err := waitSecretCmd.Run(); err != nil {
		return fmt.Errorf("cert-manager did not issue certificate: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ Certificate issued by cert-manager")

	// Now deploy DataStorage using Kustomize (includes deployment with cert volume mount)
	kustomizePath := filepath.Join(workspaceRoot, "deploy", "data-storage")
	_, _ = fmt.Fprintln(writer, "📦 Deploying DataStorage via Kustomize...")

	// Use kubectl apply with kustomize
	deployCmd := exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-n", namespace,
		"-k", kustomizePath)
	deployCmd.Stdout = writer
	deployCmd.Stderr = writer

	// Set IMAGE_TAG environment variable for kustomize
	deployCmd.Env = append(os.Environ(), fmt.Sprintf("IMAGE_TAG=%s", imageTag))

	if err := deployCmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy DataStorage: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "✅ DataStorage deployed with cert-manager certificate")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}
