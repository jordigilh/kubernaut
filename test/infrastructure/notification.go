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
// Steps:
// 1. Create Kind cluster with production-like configuration
// 2. Export kubeconfig to ~/.kube/notification-e2e-config
// 3. Install NotificationRequest CRD (cluster-wide resource)
// 4. Build and load Notification Controller Docker image
//
// Time: ~40 seconds
func CreateNotificationCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Notification E2E Cluster Setup (ONCE)")
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

	// 1. Create Kind cluster with extraMounts for E2E file delivery
	fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createNotificationKindCluster(clusterName, kubeconfigPath, e2eDir, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Install NotificationRequest CRD (cluster-wide)
	fmt.Fprintln(writer, "📋 Installing NotificationRequest CRD...")
	if err := installNotificationCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install NotificationRequest CRD: %w", err)
	}

	// 3. Build Notification Controller Docker image
	fmt.Fprintln(writer, "🔨 Building Notification Controller Docker image...")
	if err := buildNotificationImageOnly(writer); err != nil {
		return fmt.Errorf("failed to build Notification Controller image: %w", err)
	}

	// 4. Load Notification Controller image into Kind
	fmt.Fprintln(writer, "📦 Loading Notification Controller image into Kind cluster...")
	if err := loadNotificationImageOnly(clusterName, writer); err != nil {
		return fmt.Errorf("failed to load Notification Controller image: %w", err)
	}

	fmt.Fprintln(writer, "✅ Cluster ready - tests can now deploy controller per-namespace")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// createNotificationKindCluster creates a Kind cluster with dynamically added extraMounts for E2E file delivery
func createNotificationKindCluster(clusterName, kubeconfigPath, hostPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Read base Kind config
	baseConfigPath := filepath.Join(workspaceRoot, "test", "infrastructure", "kind-notification-config.yaml")
	configData, err := os.ReadFile(baseConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read Kind config: %w", err)
	}

	// Add extraMounts dynamically (append to control-plane node)
	extraMountsYAML := fmt.Sprintf(`
  extraMounts:
  - hostPath: %s
    containerPath: /tmp/e2e-notifications
    readOnly: false`, hostPath)

	// Insert extraMounts after "extraPortMappings:" section
	configStr := string(configData)
	updatedConfig := strings.Replace(configStr, "  kubeadmConfigPatches:", extraMountsYAML+"\n  kubeadmConfigPatches:", 1)

	// Write temporary config file
	tmpConfig, err := os.CreateTemp("", "kind-notification-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp config: %w", err)
	}
	defer os.Remove(tmpConfig.Name())

	if _, err := tmpConfig.WriteString(updatedConfig); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}
	if err := tmpConfig.Close(); err != nil {
		return fmt.Errorf("failed to close temp config: %w", err)
	}

	// Create Kind cluster with temporary config
	fmt.Fprintf(writer, "   Using HostPath: %s → /tmp/e2e-notifications\n", hostPath)
	cmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", tmpConfig.Name(),
		"--kubeconfig", kubeconfigPath)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	return nil
}

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
	if err := createNamespaceOnly(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 2. Create default namespace (E2E tests create NotificationRequests here)
	fmt.Fprintf(writer, "📁 Creating default namespace (for E2E tests)...\n")
	if err := createNamespaceOnly("default", kubeconfigPath, writer); err != nil {
		// Ignore error if namespace already exists
		if !strings.Contains(err.Error(), "AlreadyExists") {
			return fmt.Errorf("failed to create default namespace: %w", err)
		}
		fmt.Fprintf(writer, "   default namespace already exists\n")
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

	deleteCmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
	deleteCmd.Stdout = writer
	deleteCmd.Stderr = writer

	if err := deleteCmd.Run(); err != nil {
		fmt.Fprintf(writer, "⚠️  Warning: Failed to delete Kind cluster %s: %v\n", clusterName, err)
		// Don't return error - best effort cleanup
	} else {
		fmt.Fprintf(writer, "✅ Kind cluster %s deleted\n", clusterName)
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
// 1. Deploy PostgreSQL with pgvector
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

	// 1. Deploy PostgreSQL with pgvector
	fmt.Fprintf(writer, "🚀 Deploying PostgreSQL with pgvector...\n")
	if err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 2. Apply audit migrations using shared library
	fmt.Fprintf(writer, "📋 Applying audit migrations...\n")
	if err := ApplyAuditMigrations(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to apply audit migrations: %w", err)
	}

	// 3. Build and load Data Storage image (if not already loaded)
	fmt.Fprintf(writer, "🔨 Building Data Storage image...\n")
	if err := buildDataStorageImage(writer); err != nil {
		return fmt.Errorf("failed to build Data Storage image: %w", err)
	}

	// 4. Load Data Storage image into Kind cluster
	fmt.Fprintf(writer, "📦 Loading Data Storage image into Kind cluster...\n")
	clusterName := "notification-e2e" // Matches CreateNotificationCluster
	if err := loadDataStorageImage(clusterName, writer); err != nil {
		return fmt.Errorf("failed to load Data Storage image: %w", err)
	}

	// 5. Deploy Data Storage Service
	fmt.Fprintf(writer, "🚀 Deploying Data Storage Service...\n")
	if err := deployDataStorageServiceInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage Service: %w", err)
	}

	// 6. Wait for Data Storage ready
	fmt.Fprintf(writer, "⏳ Waiting for Data Storage Service ready...\n")
	if err := waitForDataStorageServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("Data Storage not ready: %w", err)
	}

	fmt.Fprintf(writer, "✅ Audit infrastructure ready in namespace %s\n", namespace)
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

	crdPath := filepath.Join(workspaceRoot, "config", "crd", "bases", "notification.kubernaut.ai_notificationrequests.yaml")
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
