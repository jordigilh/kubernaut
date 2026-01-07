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
	"runtime"
	"strings"
	"time"
)

func CreateNotificationCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "Notification E2E Cluster Setup - Hybrid Parallel (DD-TEST-002)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 0. Create E2E file output directory (platform-specific)
	e2eDir, err := GetE2EFileOutputDir()
	if err != nil {
		return fmt.Errorf("failed to get E2E file output directory: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "📁 Creating E2E file output directory: %s\n", e2eDir)
	if err := os.MkdirAll(e2eDir, 0755); err != nil {
		return fmt.Errorf("failed to create E2E file output directory: %w", err)
	}

	// ============================================================
	// PHASE 1: Build Docker image (BEFORE cluster creation)
	// ============================================================
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "PHASE 1: Building Notification Controller Docker image...")
	_, _ = fmt.Fprintln(writer, "  • This ensures fresh build with latest code changes")
	_, _ = fmt.Fprintln(writer, "  • Prevents stale image caching issues")
	if err := buildNotificationImageOnly(writer); err != nil {
		return fmt.Errorf("failed to build Notification Controller image: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "✅ PHASE 1 Complete: Image built successfully")

	// ============================================================
	// PHASE 2: Create Kind cluster (after build completes)
	// ============================================================
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "PHASE 2: Creating Kind cluster...")
	_, _ = fmt.Fprintln(writer, "  • Cluster created AFTER build prevents idle timeout")
	extraMounts := []ExtraMount{
		{
			HostPath:      e2eDir,
			ContainerPath: "/tmp/e2e-notifications",
			ReadOnly:      false,
		},
	}
	if err := CreateKindClusterWithExtraMounts(
		clusterName,
		kubeconfigPath,
		"test/infrastructure/kind-notification-config.yaml",
		extraMounts,
		writer,
	); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "✅ PHASE 2 Complete: Cluster created")

	// ============================================================
	// PHASE 3: Load image into cluster
	// ============================================================
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "PHASE 3: Loading Notification Controller image into Kind cluster...")
	_, _ = fmt.Fprintln(writer, "  • Fresh cluster + fresh image = reliable loading")
	if err := loadNotificationImageOnly(clusterName, writer); err != nil {
		return fmt.Errorf("failed to load Notification Controller image: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "✅ PHASE 3 Complete: Image loaded")

	// ============================================================
	// PHASE 4: Install CRDs
	// ============================================================
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "PHASE 4: Installing NotificationRequest CRD...")
	if err := installNotificationCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install NotificationRequest CRD: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "✅ PHASE 4 Complete: CRDs installed")

	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "✅ Hybrid Parallel Setup Complete - tests can now deploy controller")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

func DeployNotificationController(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Deploying Notification Controller in Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 1. Create test namespace
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 2. Create default namespace (E2E tests create NotificationRequests here)
	_, _ = fmt.Fprintf(writer, "📁 Creating default namespace (for E2E tests)...\n")
	if err := createTestNamespace("default", kubeconfigPath, writer); err != nil {
		// Ignore error if namespace already exists (case-insensitive check for different K8s error formats)
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "alreadyexists") && !strings.Contains(errMsg, "already exists") {
			return fmt.Errorf("failed to create default namespace: %w", err)
		}
		_, _ = fmt.Fprintf(writer, "   ℹ️  default namespace already exists\n")
	}

	// 2. Deploy RBAC
	_, _ = fmt.Fprintf(writer, "🔐 Deploying RBAC (ServiceAccount, Role, RoleBinding)...\n")
	if err := deployNotificationRBAC(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy RBAC: %w", err)
	}

	// 3. Deploy ConfigMap (if needed for configuration)
	_, _ = fmt.Fprintf(writer, "📄 Deploying ConfigMap...\n")
	if err := deployNotificationConfigMap(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy ConfigMap: %w", err)
	}

	// 3.5 Deploy NodePort Service for metrics (E2E test access)
	_, _ = fmt.Fprintf(writer, "🌐 Deploying NodePort Service for metrics...\n")
	if err := deployNotificationService(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy NodePort Service: %w", err)
	}

	// 4. Deploy Notification Controller
	_, _ = fmt.Fprintf(writer, "🚀 Deploying Notification Controller...\n")
	if err := deployNotificationControllerOnly(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Notification Controller: %w", err)
	}

	// 5. Wait for controller pod ready (use kubectl wait like gateway does)
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for controller pod to be created...\n")
	// First wait for pod to exist (kubectl wait fails if resource doesn't exist yet)
	podCreatedCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	for {
		checkCmd := exec.CommandContext(podCreatedCtx, "kubectl", "get",
			"-n", namespace,
			"pod",
			"-l", "app=notification-controller",
			"--no-headers")
		checkCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
		output, err := checkCmd.CombinedOutput()

		if err == nil && len(output) > 0 {
			_, _ = fmt.Fprintf(writer, "   ✅ Controller pod created\n")
			break
		}

		select {
		case <-podCreatedCtx.Done():
			return fmt.Errorf("timeout waiting for controller pod to be created")
		case <-time.After(2 * time.Second):
			// Continue polling
		}
	}

	_, _ = fmt.Fprintf(writer, "⏳ Waiting for controller pod ready...\n")
	waitCmd := exec.CommandContext(ctx, "kubectl", "wait",
		"-n", namespace,
		"--for=condition=ready",
		"pod",
		"-l", "app=notification-controller",
		"--timeout=120s")
	waitCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("controller pod did not become ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ Controller pod ready\n")

	_, _ = fmt.Fprintf(writer, "✅ Notification Controller deployed and ready in namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

func DeployNotificationAuditInfrastructure(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Deploying Audit Infrastructure in Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Generate consistent image name for build, load, and deploy
	// DD-TEST-001: Use composite tag (service-uuid) for parallel test isolation
	dataStorageImage := GenerateInfraImageName("datastorage", "notification")
	_, _ = fmt.Fprintf(writer, "📦 DataStorage image: %s\n", dataStorageImage)

	// 1+2. Build and Load Data Storage image with dynamic tag (consolidated)
	// REFACTORED: Now uses consolidated BuildAndLoadImageToKind() (Phase 3)
	// Authority: docs/handoff/TEST_INFRASTRUCTURE_PHASE3_PLAN_JAN07.md
	// BUG FIX: Capture returned image name to ensure deployment uses correct tag
	clusterName := "notification-e2e" // Matches CreateNotificationCluster
	_, _ = fmt.Fprintf(writer, "🔨 Building and loading Data Storage image...\n")
	cfg := E2EImageConfig{
		ServiceName:      "datastorage",
		ImageName:        "kubernaut/datastorage",
		DockerfilePath:   "docker/data-storage.Dockerfile",
		KindClusterName:  clusterName,
		BuildContextPath: "", // Empty = use project root (default)
		EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
	}
	actualImageName, err := BuildAndLoadImageToKind(cfg, writer)
	if err != nil {
		return fmt.Errorf("failed to build+load Data Storage image: %w", err)
	}
	// Use actual built image name instead of pre-generated one
	dataStorageImage = actualImageName
	_, _ = fmt.Fprintf(writer, "✅ Using actual image: %s\n", dataStorageImage)

	// 3. Deploy shared Data Storage infrastructure with Notification-specific NodePort 30090
	// CRITICAL: Must match kind-notification-config.yaml port mapping
	_, _ = fmt.Fprintf(writer, "📦 Deploying Data Storage infrastructure (NodePort 30090)...\n")
	if err := DeployNotificationDataStorageServices(ctx, namespace, kubeconfigPath, dataStorageImage, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage infrastructure: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ Data Storage infrastructure deployed\n")

	// 4. Wait for DataStorage to be fully ready before tests emit audit events
	// CRITICAL: DD-E2E-001 - Prevents connection reset by peer errors
	//
	// Problem: E2E tests emit audit events immediately after DataStorage deployment,
	// but DataStorage pod may accept connections before internal components (DB pool,
	// audit handler) are ready, resulting in RST/EOF errors (see NT_E2E_AUDIT_CLIENT_LOGS_EVIDENCE_DEC_27_2025.md)
	//
	// Solution: Add readiness delay + health check to ensure DataStorage is truly ready
	_, _ = fmt.Fprintf(writer, "\n⏳ Waiting for DataStorage to be ready...\n")
	_, _ = fmt.Fprintf(writer, "   (Adding 5s startup buffer for internal component initialization)\n")
	time.Sleep(5 * time.Second)

	// Verify DataStorage health endpoint is responding
	// NodePort 30090 is exposed by kind-notification-config.yaml for E2E tests
	dataStorageHealthURL := "http://127.0.0.1:30090/health"
	_, _ = fmt.Fprintf(writer, "   🔍 Checking DataStorage health endpoint: %s\n", dataStorageHealthURL)
	if err := WaitForHTTPHealth(dataStorageHealthURL, 60*time.Second, writer); err != nil {
		return fmt.Errorf("DataStorage health check failed: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ DataStorage ready and healthy\n")

	_, _ = fmt.Fprintf(writer, "\n✅ Audit infrastructure ready in namespace %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

func GetE2EFileOutputDir() (string, error) {
	if runtime.GOOS == "darwin" {
		// macOS: Podman VM limitation - use home directory
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		return filepath.Join(homeDir, ".kubernaut", "e2e-notifications"), nil
	}
	// Linux/CI: Direct /tmp access works
	return "/tmp/kubernaut-e2e-notifications", nil
}

func DeleteNotificationCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "Cleaning up Notification E2E Cluster")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Check if cluster exists before attempting deletion
	// This prevents podman provider hang when cluster doesn't exist
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer checkCancel()

	checkCmd := exec.CommandContext(checkCtx, "kind", "get", "clusters")
	output, err := checkCmd.CombinedOutput()
	clusterExists := false

	if err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  Warning: Failed to check for existing clusters: %v\n", err)
		// Assume cluster might exist, attempt deletion anyway
		clusterExists = true
	} else {
		for _, line := range strings.Split(string(output), "\n") {
			if strings.TrimSpace(line) == clusterName {
				clusterExists = true
				break
			}
		}
	}

	if !clusterExists {
		_, _ = fmt.Fprintf(writer, "ℹ️  Cluster %s does not exist, skipping deletion\n", clusterName)
	} else {
		deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer deleteCancel()

		deleteCmd := exec.CommandContext(deleteCtx, "kind", "delete", "cluster", "--name", clusterName)
		deleteCmd.Stdout = writer
		deleteCmd.Stderr = writer

		if err := deleteCmd.Run(); err != nil {
			_, _ = fmt.Fprintf(writer, "⚠️  Warning: Failed to delete Kind cluster %s: %v\n", clusterName, err)
			// Don't return error - best effort cleanup
		} else {
			_, _ = fmt.Fprintf(writer, "✅ Kind cluster %s deleted\n", clusterName)
		}
	}

	// Clean up kubeconfig file (uses passed path for consistency with Gateway pattern)
	if err := os.Remove(kubeconfigPath); err != nil && !os.IsNotExist(err) {
		_, _ = fmt.Fprintf(writer, "⚠️  Warning: Failed to remove kubeconfig: %v\n", err)
	} else {
		_, _ = fmt.Fprintf(writer, "✅ Kubeconfig removed: %s\n", kubeconfigPath)
	}

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

func buildNotificationImageOnly(writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	buildCmd := exec.Command("podman", "build",
		"--no-cache", // DD-TEST-002: Force fresh build to include latest code changes
		"-t", "localhost/kubernaut-notification:e2e-test",
		"-f", "docker/notification-controller-ubi9.Dockerfile",
		".",
	)
	buildCmd.Dir = workspaceRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("podman build failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "   Notification Controller image built: localhost/kubernaut-notification:e2e-test")
	return nil
}

func loadNotificationImageOnly(clusterName string, writer io.Writer) error {
	// Save image to tar
	saveCmd := exec.Command("podman", "save", "localhost/kubernaut-notification:e2e-test", "-o", "/tmp/notification-e2e.tar")
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load image into Kind cluster
	loadCmd := exec.Command("kind", "load", "image-archive", "/tmp/notification-e2e.tar", "--name", clusterName)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer

	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image into Kind: %w", err)
	}

	// Clean up tar file
	_ = os.Remove("/tmp/notification-e2e.tar")

	// CRITICAL: Remove Podman image immediately to free disk space
	// Image is now in Kind, Podman copy is duplicate
	_, _ = fmt.Fprintln(writer, "   🗑️  Removing Podman image to free disk space...")
	rmiCmd := exec.Command("podman", "rmi", "-f", "localhost/kubernaut-notification:e2e-test")
	rmiCmd.Stdout = writer
	rmiCmd.Stderr = writer
	if err := rmiCmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "   ⚠️  Failed to remove Podman image (non-fatal): %v\n", err)
	} else {
		_, _ = fmt.Fprintln(writer, "   ✅ Podman image removed: localhost/kubernaut-notification:e2e-test")
	}

	_, _ = fmt.Fprintln(writer, "   Notification Controller image loaded into Kind cluster")
	return nil
}

func installNotificationCRD(kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Updated path after API group migration to kubernaut.ai (Dec 16, 2025)
	crdPath := filepath.Join(workspaceRoot, "config", "crd", "bases", "kubernaut.ai_notificationrequests.yaml")
	if _, err := os.Stat(crdPath); os.IsNotExist(err) {
		return fmt.Errorf("NotificationRequest CRD not found at %s", crdPath)
	}

	applyCmd := exec.Command("kubectl", "apply", "-f", crdPath)
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply NotificationRequest CRD: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "   NotificationRequest CRD installed")
	return nil
}

func deployNotificationRBAC(namespace, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	rbacPath := filepath.Join(workspaceRoot, "test", "e2e", "notification", "manifests", "notification-rbac.yaml")
	if _, err := os.Stat(rbacPath); os.IsNotExist(err) {
		return fmt.Errorf("RBAC manifest not found at %s", rbacPath)
	}

	// Apply RBAC with namespace override for ServiceAccount
	// ClusterRole and ClusterRoleBinding are cluster-scoped
	applyCmd := exec.Command("kubectl", "apply", "-f", rbacPath, "-n", namespace)
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply RBAC: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   RBAC deployed (ClusterRole + ClusterRoleBinding) in namespace: %s\n", namespace)
	return nil
}

func deployNotificationConfigMap(namespace, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	configMapPath := filepath.Join(workspaceRoot, "test", "e2e", "notification", "manifests", "notification-configmap.yaml")
	if _, err := os.Stat(configMapPath); os.IsNotExist(err) {
		// ConfigMap is optional - controller may use defaults
		_, _ = fmt.Fprintf(writer, "   ConfigMap manifest not found (optional): %s\n", configMapPath)
		return nil
	}

	applyCmd := exec.Command("kubectl", "apply", "-f", configMapPath, "-n", namespace)
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply ConfigMap: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ConfigMap deployed in namespace: %s\n", namespace)
	return nil
}

func deployNotificationService(namespace, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	servicePath := filepath.Join(workspaceRoot, "test", "e2e", "notification", "manifests", "notification-service.yaml")
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		return fmt.Errorf("service manifest not found at %s", servicePath)
	}

	applyCmd := exec.Command("kubectl", "apply", "-f", servicePath, "-n", namespace)
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply service: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   NodePort Service deployed (metrics: localhost:8081 → NodePort 30081)\n")
	return nil
}

func deployNotificationControllerOnly(namespace, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	deploymentPath := filepath.Join(workspaceRoot, "test", "e2e", "notification", "manifests", "notification-deployment.yaml")
	if _, err := os.Stat(deploymentPath); os.IsNotExist(err) {
		return fmt.Errorf("deployment manifest not found at %s", deploymentPath)
	}

	applyCmd := exec.Command("kubectl", "apply", "-f", deploymentPath, "-n", namespace)
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply deployment: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   Notification Controller deployment applied in namespace: %s\n", namespace)
	return nil
}

func DeployNotificationDataStorageServices(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, writer io.Writer) error {
	// Deploy DataStorage with Notification-specific NodePort 30090
	return DeployDataStorageTestServicesWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, 30090, writer)
}

