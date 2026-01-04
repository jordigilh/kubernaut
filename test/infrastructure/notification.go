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

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Notification E2E Infrastructure: Kind Cluster + Controller Deployment
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// GetE2EFileOutputDir returns the platform-appropriate directory for E2E file delivery tests
// - Linux/CI: /tmp/kubernaut-e2e-notifications (direct access)
// - macOS: $HOME/.kubernaut/e2e-notifications (Podman VM only mounts home directory)
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

// CreateNotificationCluster creates a Kind cluster for Notification E2E testing
// This is called ONCE in SynchronizedBeforeSuite (first parallel process only)
//
// ** HYBRID PARALLEL SETUP (DD-TEST-002) **:
// PHASE 1: Build Docker image (BEFORE cluster creation)
// PHASE 2: Create Kind cluster (after build completes)
// PHASE 3: Load image into cluster
// PHASE 4: Install CRDs
//
// This prevents:
// - Stale Docker image caching (build fresh image first)
// - Cluster idle timeout (no waiting for builds)
// - Image loading failures (fresh cluster, fresh image)
//
// Time: ~2-3 minutes (with optimized Dockerfile)
func CreateNotificationCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Notification E2E Cluster Setup - Hybrid Parallel (DD-TEST-002)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 0. Create E2E file output directory (platform-specific)
	e2eDir, err := GetE2EFileOutputDir()
	if err != nil {
		return fmt.Errorf("failed to get E2E file output directory: %w", err)
	}
	fmt.Fprintf(writer, "📁 Creating E2E file output directory: %s\n", e2eDir)
	if err := os.MkdirAll(e2eDir, 0755); err != nil {
		return fmt.Errorf("failed to create E2E file output directory: %w", err)
	}

	// ============================================================
	// PHASE 1: Build Docker image (BEFORE cluster creation)
	// ============================================================
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "PHASE 1: Building Notification Controller Docker image...")
	fmt.Fprintln(writer, "  • This ensures fresh build with latest code changes")
	fmt.Fprintln(writer, "  • Prevents stale image caching issues")
	if err := buildNotificationImageOnly(writer); err != nil {
		return fmt.Errorf("failed to build Notification Controller image: %w", err)
	}
	fmt.Fprintln(writer, "✅ PHASE 1 Complete: Image built successfully")

	// ============================================================
	// PHASE 2: Create Kind cluster (after build completes)
	// ============================================================
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "PHASE 2: Creating Kind cluster...")
	fmt.Fprintln(writer, "  • Cluster created AFTER build prevents idle timeout")
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
	fmt.Fprintln(writer, "✅ PHASE 2 Complete: Cluster created")

	// ============================================================
	// PHASE 3: Load image into cluster
	// ============================================================
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "PHASE 3: Loading Notification Controller image into Kind cluster...")
	fmt.Fprintln(writer, "  • Fresh cluster + fresh image = reliable loading")
	if err := loadNotificationImageOnly(clusterName, writer); err != nil {
		return fmt.Errorf("failed to load Notification Controller image: %w", err)
	}
	fmt.Fprintln(writer, "✅ PHASE 3 Complete: Image loaded")

	// ============================================================
	// PHASE 4: Install CRDs
	// ============================================================
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "PHASE 4: Installing NotificationRequest CRD...")
	if err := installNotificationCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install NotificationRequest CRD: %w", err)
	}
	fmt.Fprintln(writer, "✅ PHASE 4 Complete: CRDs installed")

	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "✅ Hybrid Parallel Setup Complete - tests can now deploy controller")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// createNotificationKindCluster is deprecated - use CreateKindClusterWithExtraMounts instead
// This function is kept temporarily for reference and will be removed in future commits

// DeployNotificationController deploys the Notification Controller in a test namespace
// This is called in BeforeEach for each test file (or shared setup)
//
// Steps:
// 1. Create namespace
// 2. Deploy RBAC (ServiceAccount, Role, RoleBinding)
// 3. Deploy Notification Controller (1 pod)
// 4. Wait for controller ready
//
// Time: ~10 seconds
func DeployNotificationController(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Deploying Notification Controller in Namespace: %s\n", namespace)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 1. Create test namespace
	fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 2. Create default namespace (E2E tests create NotificationRequests here)
	fmt.Fprintf(writer, "📁 Creating default namespace (for E2E tests)...\n")
	if err := createTestNamespace("default", kubeconfigPath, writer); err != nil {
		// Ignore error if namespace already exists (case-insensitive check for different K8s error formats)
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "alreadyexists") && !strings.Contains(errMsg, "already exists") {
			return fmt.Errorf("failed to create default namespace: %w", err)
		}
		fmt.Fprintf(writer, "   ℹ️  default namespace already exists\n")
	}

	// 2. Deploy RBAC
	fmt.Fprintf(writer, "🔐 Deploying RBAC (ServiceAccount, Role, RoleBinding)...\n")
	if err := deployNotificationRBAC(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy RBAC: %w", err)
	}

	// 3. Deploy ConfigMap (if needed for configuration)
	fmt.Fprintf(writer, "📄 Deploying ConfigMap...\n")
	if err := deployNotificationConfigMap(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy ConfigMap: %w", err)
	}

	// 3.5 Deploy NodePort Service for metrics (E2E test access)
	fmt.Fprintf(writer, "🌐 Deploying NodePort Service for metrics...\n")
	if err := deployNotificationService(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy NodePort Service: %w", err)
	}

	// 4. Deploy Notification Controller
	fmt.Fprintf(writer, "🚀 Deploying Notification Controller...\n")
	if err := deployNotificationControllerOnly(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Notification Controller: %w", err)
	}

	// 5. Wait for controller pod ready (use kubectl wait like gateway does)
	fmt.Fprintf(writer, "⏳ Waiting for controller pod to be created...\n")
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
			fmt.Fprintf(writer, "   ✅ Controller pod created\n")
			break
		}

		select {
		case <-podCreatedCtx.Done():
			return fmt.Errorf("timeout waiting for controller pod to be created")
		case <-time.After(2 * time.Second):
			// Continue polling
		}
	}

	fmt.Fprintf(writer, "⏳ Waiting for controller pod ready...\n")
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
	fmt.Fprintf(writer, "   ✅ Controller pod ready\n")

	fmt.Fprintf(writer, "✅ Notification Controller deployed and ready in namespace: %s\n", namespace)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

// DeleteNotificationCluster deletes the Kind cluster
// This is called ONCE in SynchronizedAfterSuite (last parallel process only)
//
// Parameters:
// - clusterName: Kind cluster name to delete
// - kubeconfigPath: Path to kubeconfig file (e.g., ~/.kube/notification-e2e-config)
// - writer: Output writer for progress messages
func DeleteNotificationCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Cleaning up Notification E2E Cluster")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Check if cluster exists before attempting deletion
	// This prevents podman provider hang when cluster doesn't exist
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer checkCancel()

	checkCmd := exec.CommandContext(checkCtx, "kind", "get", "clusters")
	output, err := checkCmd.CombinedOutput()
	clusterExists := false

	if err != nil {
		fmt.Fprintf(writer, "⚠️  Warning: Failed to check for existing clusters: %v\n", err)
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
		fmt.Fprintf(writer, "ℹ️  Cluster %s does not exist, skipping deletion\n", clusterName)
	} else {
		deleteCtx, deleteCancel := context.WithTimeout(context.Background(), 120*time.Second)
		defer deleteCancel()

		deleteCmd := exec.CommandContext(deleteCtx, "kind", "delete", "cluster", "--name", clusterName)
		deleteCmd.Stdout = writer
		deleteCmd.Stderr = writer

		if err := deleteCmd.Run(); err != nil {
			fmt.Fprintf(writer, "⚠️  Warning: Failed to delete Kind cluster %s: %v\n", clusterName, err)
			// Don't return error - best effort cleanup
		} else {
			fmt.Fprintf(writer, "✅ Kind cluster %s deleted\n", clusterName)
		}
	}

	// Clean up kubeconfig file (uses passed path for consistency with Gateway pattern)
	if err := os.Remove(kubeconfigPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(writer, "⚠️  Warning: Failed to remove kubeconfig: %v\n", err)
	} else {
		fmt.Fprintf(writer, "✅ Kubeconfig removed: %s\n", kubeconfigPath)
	}

	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// DeployNotificationAuditInfrastructure deploys PostgreSQL and Data Storage for audit E2E tests
// This enables real audit event persistence instead of mocks (BR-NOT-062, BR-NOT-063, BR-NOT-064)
//
// Steps:
// 1. Deploy PostgreSQL (V1.0 label-only, no vector extension needed)
// 2. Apply audit migrations using shared library
// 3. Deploy Data Storage Service
// 4. Wait for services ready
//
// Time: ~60 seconds
//
// Usage:
//
//	// In E2E test BeforeSuite or BeforeEach:
//	err := infrastructure.DeployNotificationAuditInfrastructure(ctx, namespace, kubeconfigPath, GinkgoWriter)
func DeployNotificationAuditInfrastructure(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Deploying Audit Infrastructure in Namespace: %s\n", namespace)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Generate consistent image name for build, load, and deploy
	// DD-TEST-001: Use composite tag (service-uuid) for parallel test isolation
	dataStorageImage := GenerateInfraImageName("datastorage", "notification")
	fmt.Fprintf(writer, "📦 DataStorage image: %s\n", dataStorageImage)

	// 1. Build Data Storage image with dynamic tag
	fmt.Fprintf(writer, "🔨 Building Data Storage image...\n")
	if err := buildDataStorageImageWithTag(dataStorageImage, writer); err != nil {
		return fmt.Errorf("failed to build Data Storage image: %w", err)
	}

	// 2. Load Data Storage image into Kind cluster with same tag
	clusterName := "notification-e2e" // Matches CreateNotificationCluster
	fmt.Fprintf(writer, "📦 Loading Data Storage image into Kind cluster...\n")
	if err := loadDataStorageImageWithTag(clusterName, dataStorageImage, writer); err != nil {
		return fmt.Errorf("failed to load Data Storage image: %w", err)
	}

	// 3. Deploy shared Data Storage infrastructure with Notification-specific NodePort 30090
	// CRITICAL: Must match kind-notification-config.yaml port mapping
	fmt.Fprintf(writer, "📦 Deploying Data Storage infrastructure (NodePort 30090)...\n")
	if err := DeployNotificationDataStorageServices(ctx, namespace, kubeconfigPath, dataStorageImage, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage infrastructure: %w", err)
	}
	fmt.Fprintf(writer, "✅ Data Storage infrastructure deployed\n")

	// 4. Wait for DataStorage to be fully ready before tests emit audit events
	// CRITICAL: DD-E2E-001 - Prevents connection reset by peer errors
	//
	// Problem: E2E tests emit audit events immediately after DataStorage deployment,
	// but DataStorage pod may accept connections before internal components (DB pool,
	// audit handler) are ready, resulting in RST/EOF errors (see NT_E2E_AUDIT_CLIENT_LOGS_EVIDENCE_DEC_27_2025.md)
	//
	// Solution: Add readiness delay + health check to ensure DataStorage is truly ready
	fmt.Fprintf(writer, "\n⏳ Waiting for DataStorage to be ready...\n")
	fmt.Fprintf(writer, "   (Adding 5s startup buffer for internal component initialization)\n")
	time.Sleep(5 * time.Second)

	// Verify DataStorage health endpoint is responding
	// NodePort 30090 is exposed by kind-notification-config.yaml for E2E tests
	dataStorageHealthURL := "http://127.0.0.1:30090/health"
	fmt.Fprintf(writer, "   🔍 Checking DataStorage health endpoint: %s\n", dataStorageHealthURL)
	if err := WaitForHTTPHealth(dataStorageHealthURL, 60*time.Second, writer); err != nil {
		return fmt.Errorf("DataStorage health check failed: %w", err)
	}
	fmt.Fprintf(writer, "✅ DataStorage ready and healthy\n")

	fmt.Fprintf(writer, "\n✅ Audit infrastructure ready in namespace %s\n", namespace)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

// GetDataStorageServiceURL returns the Data Storage Service URL for E2E tests
// Used by tests to configure audit store to point to real Data Storage
func GetDataStorageServiceURL(namespace string) string {
	return fmt.Sprintf("http://datastorage.%s.svc.cluster.local:8080", namespace)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Internal Helper Functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// installNotificationCRD installs the NotificationRequest CRD
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

	fmt.Fprintln(writer, "   NotificationRequest CRD installed")
	return nil
}

// buildNotificationImageOnly builds the Notification Controller Docker image using Podman
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

	fmt.Fprintln(writer, "   Notification Controller image built: localhost/kubernaut-notification:e2e-test")
	return nil
}

// loadNotificationImageOnly loads the Notification Controller image into the Kind cluster
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
	fmt.Fprintln(writer, "   🗑️  Removing Podman image to free disk space...")
	rmiCmd := exec.Command("podman", "rmi", "-f", "localhost/kubernaut-notification:e2e-test")
	rmiCmd.Stdout = writer
	rmiCmd.Stderr = writer
	if err := rmiCmd.Run(); err != nil {
		fmt.Fprintf(writer, "   ⚠️  Failed to remove Podman image (non-fatal): %v\n", err)
	} else {
		fmt.Fprintln(writer, "   ✅ Podman image removed: localhost/kubernaut-notification:e2e-test")
	}

	fmt.Fprintln(writer, "   Notification Controller image loaded into Kind cluster")
	return nil
}

// deployNotificationRBAC deploys RBAC resources for the Notification Controller
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

	fmt.Fprintf(writer, "   RBAC deployed (ClusterRole + ClusterRoleBinding) in namespace: %s\n", namespace)
	return nil
}

// deployNotificationConfigMap deploys ConfigMap for the Notification Controller
func deployNotificationConfigMap(namespace, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	configMapPath := filepath.Join(workspaceRoot, "test", "e2e", "notification", "manifests", "notification-configmap.yaml")
	if _, err := os.Stat(configMapPath); os.IsNotExist(err) {
		// ConfigMap is optional - controller may use defaults
		fmt.Fprintf(writer, "   ConfigMap manifest not found (optional): %s\n", configMapPath)
		return nil
	}

	applyCmd := exec.Command("kubectl", "apply", "-f", configMapPath, "-n", namespace)
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply ConfigMap: %w", err)
	}

	fmt.Fprintf(writer, "   ConfigMap deployed in namespace: %s\n", namespace)
	return nil
}

// deployNotificationService deploys the NodePort Service for metrics
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

	fmt.Fprintf(writer, "   NodePort Service deployed (metrics: localhost:8081 → NodePort 30081)\n")
	return nil
}

// deployNotificationControllerOnly deploys the Notification Controller deployment
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

	fmt.Fprintf(writer, "   Notification Controller deployment applied in namespace: %s\n", namespace)
	return nil
}

// waitForNotificationControllerReady waits for the Notification Controller pod to become ready
func waitForNotificationControllerReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	timeout := 60 * time.Second
	interval := 2 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		cmd := exec.CommandContext(ctx, "kubectl", "get", "pods",
			"-n", namespace,
			"-l", "app=notification-controller",
			"-o", "jsonpath={.items[0].status.phase}")
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))

		output, err := cmd.Output()
		if err == nil && string(output) == "Running" {
			// Double-check with ready condition
			readyCmd := exec.CommandContext(ctx, "kubectl", "get", "pods",
				"-n", namespace,
				"-l", "app=notification-controller",
				"-o", "jsonpath={.items[0].status.conditions[?(@.type=='Ready')].status}")
			readyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))

			readyOutput, readyErr := readyCmd.Output()
			if readyErr == nil && string(readyOutput) == "True" {
				fmt.Fprintf(writer, "   Controller pod ready (Phase=Running, Ready=True)\n")
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
			// Continue waiting
		}
	}

	return fmt.Errorf("timeout waiting for controller pod to become ready after %v", timeout)
}

// DeployNotificationDataStorageServices deploys complete DataStorage infrastructure with NodePort 30090
// This is a Notification-specific variant that uses DeployDataStorageTestServicesWithNodePort
// to match kind-notification-config.yaml port mapping (30090 instead of default 30081)
func DeployNotificationDataStorageServices(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, writer io.Writer) error {
	// Deploy DataStorage with Notification-specific NodePort 30090
	return DeployDataStorageTestServicesWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, 30090, writer)
}

// deployDataStorageServiceForNotification deploys Data Storage with NodePort 30090
// Notification E2E tests use a different port (30090) than other services (30081)
// per kind-notification-config.yaml extraPortMappings
// DEPRECATED: Use DeployNotificationDataStorageServices instead
func deployDataStorageServiceForNotification(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// 1. Create ConfigMap for service configuration
	configYAML := fmt.Sprintf(`service:
  name: data-storage
  metricsPort: 9090
  logLevel: debug
  shutdownTimeout: 30s
server:
  port: 8080
  host: "0.0.0.0"
  read_timeout: 30s
  write_timeout: 30s
database:
  host: postgresql.%s.svc.cluster.local
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
  addr: redis.%s.svc.cluster.local:6379
  db: 0
  dlq_stream_name: dlq-stream
  dlq_max_len: 1000
  dlq_consumer_group: dlq-group
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
		return fmt.Errorf("failed to create Data Storage Secret: %w", err)
	}

	// 3. Create Service with NodePort 30090 (Notification-specific)
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datastorage",
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
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					NodePort:   30090, // Notification-specific: matches kind-notification-config.yaml
					Protocol:   corev1.ProtocolTCP,
				},
				{
					Name:       "metrics",
					Port:       9090,
					TargetPort: intstr.FromInt(9090),
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
					Containers: []corev1.Container{
					{
						Name:  "datastorage",
						Image: "localhost/kubernaut-datastorage:e2e-test-datastorage", // Matches buildDataStorageImage tag
						Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
								},
								{
									Name:          "metrics",
									ContainerPort: 9090,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "CONFIG_PATH",
									Value: "/etc/datastorage/config.yaml",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
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
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(8080),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(8080),
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
					},
				},
			},
		},
	}

	_, err = clientset.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create Data Storage Deployment: %w", err)
	}

	fmt.Fprintf(writer, "   Data Storage Service deployed (NodePort 30090 → localhost:30090)\n")
	return nil
}

// SetupNotificationInfrastructureHybrid implements DD-TEST-002 Hybrid Parallel Setup:
// Consolidates CreateNotificationCluster + DeployNotificationController + DeployNotificationAuditInfrastructure
// PHASE 1: Build images in parallel (Notification + DataStorage)
// PHASE 2: Create Kind cluster AFTER builds complete
// PHASE 3: Load images in parallel
// PHASE 4: Deploy all services in parallel (CRD, RBAC, Controller, DataStorage infra)
//
// This replaces the 3 sequential function calls with a single parallel deployment.
func SetupNotificationInfrastructureHybrid(clusterName, kubeconfigPath, namespace string, writer io.Writer) error {
	ctx := context.Background()

	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "🚀 Notification E2E Infrastructure (HYBRID PARALLEL - DD-TEST-002)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "  Strategy: Build parallel → Create cluster → Load → Deploy")
	fmt.Fprintln(writer, "  Consolidates: CreateCluster + DeployController + DeployAudit")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 0. Create E2E file output directory (platform-specific)
	e2eDir, err := GetE2EFileOutputDir()
	if err != nil {
		return fmt.Errorf("failed to get E2E file output directory: %w", err)
	}
	fmt.Fprintf(writer, "📁 Creating E2E file output directory: %s\n", e2eDir)
	if err := os.MkdirAll(e2eDir, 0755); err != nil {
		return fmt.Errorf("failed to create E2E file output directory: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 0: Generate dynamic image tags (BEFORE building)
	// ═══════════════════════════════════════════════════════════════════════
	// Generate DataStorage image tag ONCE (non-idempotent, timestamp-based)
	// This ensures each service builds its OWN DataStorage with LATEST code
	// Per DD-TEST-001: Dynamic tags for parallel E2E isolation
	dataStorageImageName := GenerateInfraImageName("datastorage", "notification")
	fmt.Fprintf(writer, "📛 DataStorage dynamic tag: %s\n", dataStorageImageName)
	fmt.Fprintln(writer, "   (Ensures fresh build with latest DataStorage code)")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL (before cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	fmt.Fprintln(writer, "  ├── Notification Controller")
	fmt.Fprintln(writer, "  └── DataStorage (WITH DYNAMIC TAG)")

	type imageBuildResult struct {
		name string
		err  error
	}

	buildResults := make(chan imageBuildResult, 2)

	go func() {
		err := buildNotificationImageOnly(writer)
		buildResults <- imageBuildResult{"Notification", err}
	}()
	go func() {
		err := buildDataStorageImageWithTag(dataStorageImageName, writer)
		buildResults <- imageBuildResult{"DataStorage", err}
	}()

	for i := 0; i < 2; i++ {
		result := <-buildResults
		if result.err != nil {
			return fmt.Errorf("failed to build %s image: %w", result.name, result.err)
		}
		fmt.Fprintf(writer, "  ✅ %s image built\n", result.name)
	}
	fmt.Fprintln(writer, "\n✅ All images built!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster (AFTER builds complete)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster...")
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

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images in parallel
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images in parallel...")
	fmt.Fprintln(writer, "  ├── Notification Controller")
	fmt.Fprintln(writer, "  └── DataStorage (with dynamic tag)")
	type imageLoadResult struct {
		name string
		err  error
	}
	loadResults := make(chan imageLoadResult, 2)

	go func() {
		err := loadNotificationImageOnly(clusterName, writer)
		loadResults <- imageLoadResult{"Notification", err}
	}()
	go func() {
		err := loadDataStorageImageWithTag(clusterName, dataStorageImageName, writer)
		loadResults <- imageLoadResult{"DataStorage", err}
	}()

	for i := 0; i < 2; i++ {
		result := <-loadResults
		if result.err != nil {
			return fmt.Errorf("failed to load %s: %w", result.name, result.err)
		}
		fmt.Fprintf(writer, "  ✅ %s image loaded\n", result.name)
	}
	fmt.Fprintln(writer, "✅ All images loaded!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy services in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying services in parallel...")
	fmt.Fprintln(writer, "  (Kubernetes will handle dependencies and reconciliation)")

	type deployResult struct {
		name string
		err  error
	}
	deployResults := make(chan deployResult, 8)

	// Launch ALL kubectl apply commands concurrently
	go func() {
		err := installNotificationCRD(kubeconfigPath, writer)
		deployResults <- deployResult{"NotificationRequest CRD", err}
	}()
	go func() {
		err := deployNotificationRBAC(namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"RBAC", err}
	}()
	go func() {
		err := deployNotificationConfigMap(namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"ConfigMap", err}
	}()
	go func() {
		err := deployNotificationService(namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"Service", err}
	}()
	go func() {
		err := deployNotificationControllerOnly(namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"NotificationController", err}
	}()
	go func() {
		err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"PostgreSQL", err}
	}()
	go func() {
		err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"Redis", err}
	}()
	go func() {
		err := ApplyAllMigrations(ctx, namespace, kubeconfigPath, writer)
		deployResults <- deployResult{"Migrations", err}
	}()
	// Note: DataStorage deployed after services are ready to avoid race with migrations

	// Collect ALL results before proceeding (MANDATORY)
	var deployErrors []error
	for i := 0; i < 8; i++ {
		result := <-deployResults
		if result.err != nil {
			fmt.Fprintf(writer, "  ❌ %s deployment failed: %v\n", result.name, result.err)
			deployErrors = append(deployErrors, result.err)
		} else {
			fmt.Fprintf(writer, "  ✅ %s manifests applied\n", result.name)
		}
	}

	if len(deployErrors) > 0 {
		return fmt.Errorf("one or more service deployments failed: %v", deployErrors)
	}
	fmt.Fprintln(writer, "  ✅ All manifests applied! (Kubernetes reconciling...)")

	// Deploy DataStorage after migrations complete
	fmt.Fprintln(writer, "\n📦 Deploying DataStorage service...")
	fmt.Fprintf(writer, "   Using dynamic tag from Phase 0: %s\n", dataStorageImageName)
	if err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, dataStorageImageName, writer); err != nil {
		return fmt.Errorf("failed to deploy DataStorage: %w", err)
	}

	// Single wait for ALL services ready (Kubernetes handles dependencies)
	fmt.Fprintln(writer, "\n⏳ Waiting for all services to be ready (Kubernetes reconciling dependencies)...")
	if err := waitForNotificationServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "✅ Notification E2E Infrastructure Ready (DD-TEST-002 Compliant)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// waitForNotificationServicesReady waits for Notification Controller and DataStorage pods to be ready
// Per DD-TEST-002: Single readiness check after parallel deployment
func waitForNotificationServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
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

	// Wait for Notification Controller pod to be ready
	fmt.Fprintf(writer, "   ⏳ Waiting for Notification Controller pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=notification-controller",
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
	}, 3*time.Minute, 5*time.Second).Should(BeTrue(), "Notification Controller pod should become ready")
	fmt.Fprintf(writer, "   ✅ Notification Controller ready\n")

	return nil
}
