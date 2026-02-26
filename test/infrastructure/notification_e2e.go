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
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
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
			EnableCoverage:   os.Getenv("E2E_COVERAGE") == "true",
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

	// DD-TEST-007: Create coverdata directory for E2E coverage collection
	var coverMounts []ExtraMount
	if os.Getenv("E2E_COVERAGE") == "true" {
		projectRoot := getProjectRoot()
		coverdataPath := filepath.Join(projectRoot, "coverdata")
		_, _ = fmt.Fprintf(writer, "📁 Creating coverage directory: %s\n", coverdataPath)
		if err := os.MkdirAll(coverdataPath, 0777); err != nil {
			return "", fmt.Errorf("failed to create coverdata directory: %w", err)
		}
		coverMounts = []ExtraMount{
			{
				HostPath:      coverdataPath,
				ContainerPath: "/coverdata",
				ReadOnly:      false,
			},
		}
	}

	if err := CreateKindClusterWithExtraMounts(
		clusterName,
		kubeconfigPath,
		"test/infrastructure/kind-notification-config.yaml",
		coverMounts, // DD-TEST-007: Mount /coverdata for E2E binary coverage
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

// DeployNotificationController deploys the Notification controller and all its resources
// using a single inline YAML template. Standardized: same pattern as AA, SP, RO, EM, WE, HAPI.
func DeployNotificationController(ctx context.Context, namespace, kubeconfigPath, notificationImageName string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "Deploying Notification Controller in Namespace: %s\n", namespace)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	if notificationImageName == "" {
		return fmt.Errorf("notificationImageName parameter is required")
	}

	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "📁 Creating default namespace (for E2E tests)...\n")
	if err := createTestNamespace("default", kubeconfigPath, writer); err != nil {
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "alreadyexists") && !strings.Contains(errMsg, "already exists") {
			return fmt.Errorf("failed to create default namespace: %w", err)
		}
		_, _ = fmt.Fprintf(writer, "   ℹ️  default namespace already exists\n")
	}

	_, _ = fmt.Fprintf(writer, "🚀 Deploying Notification resources via inline YAML template...\n")
	slackURL := resolveSlackWebhookURL(writer)
	enableCoverage := os.Getenv("E2E_COVERAGE") == "true"
	manifest := notificationControllerManifest(namespace, notificationImageName, slackURL, enableCoverage)

	cmd := exec.Command("kubectl", "apply", "--kubeconfig", kubeconfigPath, "-n", namespace, "-f", "-")
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl apply Notification manifest failed: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "🔐 Adding DataStorage access for notification-controller SA (DD-AUTH-014)...\n")
	if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, "notification-controller", writer); err != nil {
		return fmt.Errorf("failed to create DataStorage access RoleBinding: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage access RoleBinding created\n")

	_, _ = fmt.Fprintf(writer, "⏳ Waiting for controller pod to be created...\n")
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
		}
	}

	_, _ = fmt.Fprintf(writer, "⏳ Waiting for controller pod ready...\n")
	waitCmd := exec.CommandContext(ctx, "kubectl", "wait",
		"-n", namespace,
		"--for=condition=ready",
		"pod",
		"-l", "app=notification-controller",
		"--timeout=300s")
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

// resolveSlackWebhookURL resolves the Slack webhook URL from environment or config file.
func resolveSlackWebhookURL(writer io.Writer) string {
	slackURL := os.Getenv("SLACK_WEBHOOK_URL")
	if slackURL != "" {
		_, _ = fmt.Fprintf(writer, "   Slack webhook URL loaded from SLACK_WEBHOOK_URL env var\n")
		return slackURL
	}
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		slackFilePath := filepath.Join(homeDir, ".kubernaut", "notification", "slack-webhook.url")
		if data, readErr := os.ReadFile(slackFilePath); readErr == nil {
			url := strings.TrimSpace(string(data))
			if url != "" {
				_, _ = fmt.Fprintf(writer, "   Slack webhook URL loaded from %s\n", slackFilePath)
				return url
			}
		}
	}
	return "http://mock-slack:8080/webhook"
}

// notificationControllerManifest generates the full Notification controller multi-document
// YAML manifest as an inline template. This consolidates what was previously 4 separate
// static YAML files (RBAC, ConfigMap, Service, Deployment) into a single atomic apply.
//
// Standardization: same pattern as AA, SP, RO, EM, WE, HAPI, Gateway.
func notificationControllerManifest(namespace, imageName, slackWebhookURL string, enableCoverage bool) string {
	pullPolicy := GetImagePullPolicy()

	coverageEnvYAML := ""
	coverageVolumeMountYAML := ""
	coverageVolumeYAML := ""
	coverageSecurityContextYAML := ""

	if enableCoverage {
		coverageEnvYAML = `
        - name: GOCOVERDIR
          value: /coverdata`
		coverageVolumeMountYAML = `
        - name: coverdata
          mountPath: /coverdata`
		coverageVolumeYAML = `
      - name: coverdata
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate`
		coverageSecurityContextYAML = `
      securityContext:
        runAsUser: 0
        runAsGroup: 0`
	}

	return fmt.Sprintf(`---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: notification-controller
  labels:
    app: notification-controller
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: notification-controller-role
  labels:
    app: notification-controller
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["notificationrequests"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["kubernaut.ai"]
  resources: ["notificationrequests/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["notificationrequests/finalizers"]
  verbs: ["update"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
- apiGroups: [""]
  resources: ["configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: notification-controller-rolebinding
  labels:
    app: notification-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: notification-controller-role
subjects:
- kind: ServiceAccount
  name: notification-controller
  namespace: %s
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: notification-controller-config
data:
  config.yaml: |
    controller:
      metricsAddr: ":9186"
      healthProbeAddr: ":8081"
      leaderElection: false
      leaderElectionId: "notification.kubernaut.ai"
    delivery:
      console:
        enabled: true
      file:
        outputDir: "/tmp/notifications"
        format: "json"
        timeout: 5s
      log:
        enabled: true
        format: "json"
      slack:
        timeout: 10s
    datastorage:
      url: "http://data-storage-service.%s.svc.cluster.local:8080"
      timeout: 10s
      buffer:
        bufferSize: 10000
        batchSize: 100
        flushInterval: 1s
        maxRetries: 3
---
apiVersion: v1
kind: Service
metadata:
  name: notification-metrics
  labels:
    app: notification-controller
spec:
  type: NodePort
  selector:
    app: notification-controller
    control-plane: controller-manager
  ports:
  - name: metrics
    protocol: TCP
    port: 9090
    targetPort: 9186
    nodePort: 30186
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: notification-controller
  labels:
    app: notification-controller
    control-plane: controller-manager
spec:
  replicas: 1
  selector:
    matchLabels:
      app: notification-controller
      control-plane: controller-manager
  template:
    metadata:
      labels:
        app: notification-controller
        control-plane: controller-manager
    spec:
      serviceAccountName: notification-controller%s
      nodeSelector:
        node-role.kubernetes.io/control-plane: ""
      tolerations:
        - key: node-role.kubernetes.io/control-plane
          operator: Exists
          effect: NoSchedule
      containers:
      - name: manager
        image: %s
        imagePullPolicy: %s
        env:
        - name: CONFIG_PATH
          value: "/etc/notification/config.yaml"
        - name: KUBERNAUT_CONTROLLER_NAMESPACE
          value: "%s"
        - name: SLACK_WEBHOOK_URL
          value: "%s"%s
        args:
        - "-config"
        - "$(CONFIG_PATH)"
        ports:
        - containerPort: 9186
          name: metrics
          protocol: TCP
        - containerPort: 8081
          name: health
          protocol: TCP
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 20
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 15
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 6
        resources:
          limits:
            cpu: 500m
            memory: 512Mi
          requests:
            cpu: 100m
            memory: 128Mi
        volumeMounts:
        - name: config
          mountPath: /etc/notification
          readOnly: true
        - name: notification-output
          mountPath: /tmp/notifications%s
      volumes:
      - name: config
        configMap:
          name: notification-controller-config
      - name: notification-output
        emptyDir: {}%s
      terminationGracePeriodSeconds: 10
`, namespace, namespace, namespace, coverageSecurityContextYAML, imageName, pullPolicy, slackWebhookURL, coverageEnvYAML, coverageVolumeMountYAML, coverageVolumeYAML)
}

// DeployNotificationDataStorageServices deploys DataStorage with OAuth2-Proxy for Notification E2E.
// TD-E2E-001 Phase 1: oauth2ProxyImage parameter added for SOC2 architecture parity.
func DeployNotificationDataStorageServices(ctx context.Context, namespace, kubeconfigPath, dataStorageImage string, writer io.Writer) error {
	// Deploy DataStorage with OAuth2-Proxy and Notification-specific NodePort 30090 (TD-E2E-001 Phase 1)
	return DeployDataStorageTestServicesWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, 30090, writer)
}
