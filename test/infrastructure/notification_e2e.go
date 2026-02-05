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

func CreateNotificationCluster(clusterName, kubeconfigPath string, writer io.Writer) (string, error) {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "Notification E2E Cluster Setup - Hybrid Parallel (DD-TEST-002)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// ============================================================
	// PHASE 1: Build images IN PARALLEL (BEFORE cluster creation)
	// ============================================================
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	_, _ = fmt.Fprintln(writer, "  ├── Notification Controller")
	_, _ = fmt.Fprintln(writer, "  └── AuthWebhook (FOR SOC2 CC8.1)")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~2 minutes (parallel)")
	_, _ = fmt.Fprintln(writer, "  Authority: RO/WE hybrid parallel pattern")

	type buildResult struct {
		name      string
		imageName string
		err       error
	}

	buildResults := make(chan buildResult, 2)
	builtImages := make(map[string]string)

	// Build Notification Controller in parallel
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "notification", // Operator SDK convention: no -controller suffix in image name
			ImageName:        "kubernaut/notification",
			DockerfilePath:   "docker/notification-controller-ubi9.Dockerfile", // Dockerfile can have suffix
			BuildContextPath: "",
			EnableCoverage:   false,
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- buildResult{name: "Notification", imageName: imageName, err: err}
	}()

	// Build AuthWebhook in parallel
	go func() {
		cfg := E2EImageConfig{
			ServiceName:      "authwebhook",
			ImageName:        "authwebhook",
			DockerfilePath:   "docker/authwebhook.Dockerfile",
			BuildContextPath: "",
			EnableCoverage:   false,
		}
		imageName, err := BuildImageForKind(cfg, writer)
		buildResults <- buildResult{name: "AuthWebhook", imageName: imageName, err: err}
	}()

	// Wait for both builds to complete
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for all builds to complete...")
	var buildErrors []error
	for i := 0; i < 2; i++ {
		result := <-buildResults
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s build failed: %v\n", result.name, result.err)
			buildErrors = append(buildErrors, result.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s build completed: %s\n", result.name, result.imageName)
			builtImages[result.name] = result.imageName
		}
	}

	if len(buildErrors) > 0 {
		return "", fmt.Errorf("image builds failed: %v", buildErrors)
	}

	_, _ = fmt.Fprintln(writer, "\n✅ All images built successfully!")
	notificationImageName := builtImages["Notification"]

	// ============================================================
	// PHASE 2: Create Kind cluster (after build completes)
	// ============================================================
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "PHASE 2: Creating Kind cluster...")
	_, _ = fmt.Fprintln(writer, "  • Cluster created AFTER build prevents idle timeout")
	// No extraMounts needed - tests use emptyDir volume and kubectl exec to read files
	// This avoids hostPath permission issues in CI/CD (Linux)
	if err := CreateKindClusterWithExtraMounts(
		clusterName,
		kubeconfigPath,
		"test/infrastructure/kind-notification-config.yaml",
		nil, // No extraMounts - using emptyDir
		writer,
	); err != nil {
		return "", fmt.Errorf("failed to create Kind cluster: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "✅ PHASE 2 Complete: Cluster created")

	// ============================================================
	// PHASE 3: Load images into Kind cluster IN PARALLEL
	// ============================================================
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images into Kind cluster...")
	_, _ = fmt.Fprintln(writer, "  ├── Notification Controller")
	_, _ = fmt.Fprintln(writer, "  └── AuthWebhook (SOC2)")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Expected: ~30 seconds")

	type loadResult struct {
		name string
		err  error
	}

	loadResults := make(chan loadResult, 2)

	// Load Notification image
	go func() {
		err := LoadImageToKind(notificationImageName, "notification", clusterName, writer)
		loadResults <- loadResult{name: "Notification", err: err}
	}()

	// Load AuthWebhook image
	go func() {
		awImage := builtImages["AuthWebhook"]
		err := LoadImageToKind(awImage, "authwebhook", clusterName, writer)
		loadResults <- loadResult{name: "AuthWebhook", err: err}
	}()

	// Wait for both loads to complete
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for images to load...")
	var loadErrors []error
	for i := 0; i < 2; i++ {
		result := <-loadResults
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s load failed: %v\n", result.name, result.err)
			loadErrors = append(loadErrors, result.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s loaded\n", result.name)
		}
	}

	if len(loadErrors) > 0 {
		return "", fmt.Errorf("image loads failed: %v", loadErrors)
	}

	_, _ = fmt.Fprintln(writer, "\n✅ All images loaded successfully!")

	// Store AuthWebhook image for later use in suite
	_ = os.Setenv("E2E_AUTHWEBHOOK_IMAGE", builtImages["AuthWebhook"])

	// ============================================================
	// PHASE 4: Install CRDs
	// ============================================================
	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "PHASE 4: Installing NotificationRequest CRD...")
	if err := installNotificationCRD(kubeconfigPath, writer); err != nil {
		return "", fmt.Errorf("failed to install NotificationRequest CRD: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "✅ PHASE 4 Complete: CRDs installed")

	_, _ = fmt.Fprintln(writer, "")
	_, _ = fmt.Fprintln(writer, "✅ Hybrid Parallel Setup Complete - tests can now deploy controller")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return notificationImageName, nil
}

func DeployNotificationController(ctx context.Context, namespace, kubeconfigPath, notificationImageName string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Deploying Notification Controller in Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	if notificationImageName == "" {
		return fmt.Errorf("notificationImageName parameter is required")
	}

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

	// 2b. Add DataStorage access for audit emission (DD-AUTH-014, same pattern as WE/AuthWebhook)
	_, _ = fmt.Fprintf(writer, "🔐 Adding DataStorage access for notification-controller SA (DD-AUTH-014)...\n")
	if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, "notification-controller", writer); err != nil {
		return fmt.Errorf("failed to create DataStorage access RoleBinding: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage access RoleBinding created\n")

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

	// 4. Deploy Notification Controller with image from setup phase
	_, _ = fmt.Fprintf(writer, "🚀 Deploying Notification Controller...\n")
	if err := deployNotificationControllerOnly(namespace, kubeconfigPath, notificationImageName, writer); err != nil {
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

	// 3. Deploy shared Data Storage infrastructure with OAuth2-Proxy and Notification-specific NodePort 30090
	// CRITICAL: Must match kind-notification-config.yaml port mapping
	_, _ = fmt.Fprintf(writer, "📦 Deploying Data Storage infrastructure with OAuth2-Proxy (NodePort 30090)...\n")
	if err := DeployNotificationDataStorageServices(ctx, namespace, kubeconfigPath, dataStorageImage, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage infrastructure: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ Data Storage infrastructure deployed\n")

	// 3.5. Deploy data-storage-client ClusterRole (DD-AUTH-014)
	// CRITICAL: This must be deployed BEFORE RoleBindings that reference it
	_, _ = fmt.Fprintf(writer, "\n🔐 Deploying data-storage-client ClusterRole (DD-AUTH-014)...\n")
	if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy client ClusterRole: %w", err)
	}

	// 3.6. Create RoleBinding for Notification controller to access DataStorage
	// DD-AUTH-011-E2E-RBAC-ISSUE: DataStorage is in SAME namespace as Notification in E2E
	// Authority: DD-AUTH-011-E2E-RBAC-ISSUE.md
	_, _ = fmt.Fprintf(writer, "🔐 Creating RoleBinding for Notification controller (DD-AUTH-014)...\n")
	if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, "notification-controller", writer); err != nil {
		return fmt.Errorf("failed to create DataStorage access RoleBinding: %w", err)
	}

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

func DeleteNotificationCluster(clusterName, kubeconfigPath string, testsFailed bool, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "Cleaning up Notification E2E Cluster")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Use shared cleanup function with log export on failure
	if err := DeleteCluster(clusterName, "notification", testsFailed, writer); err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  Warning: Failed to delete cluster: %v\n", err)
		// Don't return error - best effort cleanup
	}

	// Clean up kubeconfig file (uses passed path for consistency with Gateway pattern)
	if err := os.Remove(kubeconfigPath); err != nil && !os.IsNotExist(err) {
		_, _ = fmt.Fprintf(writer, "⚠️  Warning: Failed to remove kubeconfig: %v\n", err)
	} else {
		_, _ = fmt.Fprintln(writer, "✅ Kubeconfig removed")
	}

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
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

	// Use envsubst to replace ${NAMESPACE} placeholder in ConfigMap
	// This allows data_storage_url to dynamically reference the correct namespace
	envsubstCmd := exec.Command("sh", "-c", fmt.Sprintf("export NAMESPACE=%s && envsubst < %s | kubectl apply -n %s -f -", namespace, configMapPath, namespace))
	envsubstCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	envsubstCmd.Stdout = writer
	envsubstCmd.Stderr = writer

	if err := envsubstCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply ConfigMap with envsubst: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   ConfigMap deployed in namespace: %s (envsubst applied)\n", namespace)
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

func deployNotificationControllerOnly(namespace, kubeconfigPath, notificationImageName string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	if notificationImageName == "" {
		return fmt.Errorf("notificationImageName parameter is required (no hardcoded fallback)")
	}

	_, _ = fmt.Fprintf(writer, "   Using Notification image: %s\n", notificationImageName)

	// Read deployment manifest and replace hardcoded tag with actual tag
	deploymentPath := filepath.Join(workspaceRoot, "test", "e2e", "notification", "manifests", "notification-deployment.yaml")
	deploymentContent, err := os.ReadFile(deploymentPath)
	if err != nil {
		return fmt.Errorf("failed to read deployment file: %w", err)
	}

	// Replace hardcoded tag with actual unique tag
	updatedContent := strings.ReplaceAll(string(deploymentContent),
		"localhost/kubernaut-notification:e2e-test",
		notificationImageName)

	// Replace hardcoded imagePullPolicy with dynamic value
	// CI/CD mode (IMAGE_REGISTRY set): Use IfNotPresent (allows pulling from GHCR)
	// Local mode: Use Never (uses images loaded into Kind)
	updatedContent = strings.ReplaceAll(updatedContent,
		"imagePullPolicy: Never",
		fmt.Sprintf("imagePullPolicy: %s", GetImagePullPolicy()))

	// Create temporary modified deployment file
	tmpDeployment := filepath.Join(os.TempDir(), "notification-deployment-e2e.yaml")
	if err := os.WriteFile(tmpDeployment, []byte(updatedContent), 0644); err != nil {
		return fmt.Errorf("failed to write temp deployment: %w", err)
	}
	defer func() { _ = os.Remove(tmpDeployment) }()

	// Apply the modified deployment
	applyCmd := exec.Command("kubectl", "apply", "-f", tmpDeployment, "-n", namespace)
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply deployment: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   Notification Controller deployment applied in namespace: %s\n", namespace)
	return nil
}

// DeployNotificationDataStorageServices deploys DataStorage with OAuth2-Proxy for Notification E2E.
// TD-E2E-001 Phase 1: oauth2ProxyImage parameter added for SOC2 architecture parity.
func DeployNotificationDataStorageServices(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, writer io.Writer) error {
	// Deploy DataStorage with OAuth2-Proxy and Notification-specific NodePort 30090 (TD-E2E-001 Phase 1)
	return DeployDataStorageTestServicesWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, 30090, writer)
}
