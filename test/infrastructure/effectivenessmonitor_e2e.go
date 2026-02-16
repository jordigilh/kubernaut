/*
Copyright 2026 Jordi Gil.

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
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ============================================================================
// Effectiveness Monitor E2E Infrastructure
// ============================================================================
//
// Sets up a Kind cluster with all dependencies needed for EM E2E testing:
//   - EM controller (watches EffectivenessAssessment CRDs)
//   - DataStorage (PostgreSQL + Redis + API for audit event storage/queries)
//   - AuthWebhook (DD-AUTH-014: ServiceAccount token validation)
//   - Prometheus (real instance for metric comparison via OTLP)
//   - AlertManager (real instance for alert resolution queries)
//
// Port Allocation (DD-TEST-001 v2.8):
//   - EM:           NodePort 30089 (API), 30189 (Metrics)
//   - DataStorage:  NodePort 30081
//   - Prometheus:   NodePort 30190
//   - AlertManager: NodePort 30193
//
// References:
//   - ADR-EM-001: Effectiveness Monitor Service Integration
//   - DD-TEST-001: Port Allocation Strategy
//   - TESTING_GUIDELINES.md v2.6.0 Section 4a: Prom/AM real instances in E2E
// ============================================================================

const (
	// emE2ENamespace is the namespace where EM and dependencies are deployed
	emE2ENamespace = "kubernaut-system"

	// emE2EKindConfig is the Kind cluster configuration file for EM E2E tests
	emE2EKindConfig = "test/infrastructure/kind-effectivenessmonitor-config.yaml"

	// emDataStorageHostPort is the host port for DataStorage in EM E2E (DD-TEST-001)
	emDataStorageHostPort = 8091

	// emPrometheusReadyTimeout is the max time to wait for Prometheus to be ready
	emPrometheusReadyTimeout = 60 * time.Second

	// emAlertManagerReadyTimeout is the max time to wait for AlertManager to be ready
	emAlertManagerReadyTimeout = 60 * time.Second
)

// DataStorageEMHostPort returns the host port for DataStorage in EM E2E tests.
// Per DD-TEST-001 v2.8: EM -> DataStorage dependency uses host port 8091.
func DataStorageEMHostPort() int {
	return emDataStorageHostPort
}

// SetupEMInfrastructure creates the complete E2E infrastructure for the
// Effectiveness Monitor service using the hybrid parallel strategy (DD-TEST-002).
//
// Strategy:
//  1. Build images in PARALLEL (EM, DataStorage, AuthWebhook)
//  2. Create Kind cluster AFTER builds complete
//  3. Load images into cluster
//  4. Install CRDs (EffectivenessAssessment, RemediationRequest)
//  5. Deploy services (PostgreSQL, Redis, DataStorage, AuthWebhook, Prometheus, AlertManager, EM)
//
// Parameters:
//   - ctx: Context for cancellation
//   - clusterName: Kind cluster name (e.g., "em-e2e")
//   - kubeconfigPath: Path to write kubeconfig (e.g., "~/.kube/em-e2e-config")
//   - writer: Output writer for progress logging
func SetupEMInfrastructure(ctx context.Context, clusterName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  EffectivenessMonitor E2E Infrastructure (HYBRID PARALLEL)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build parallel -> Create cluster -> Load -> Deploy")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-001 v2.8: EM 30089, DS 30081, Prom 30190, AM 30193")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	namespace := emE2ENamespace
	projectRoot := getProjectRoot()

	// DD-TEST-007: Create coverdata directory for coverage collection
	coverdataPath := filepath.Join(projectRoot, "coverdata")
	_, _ = fmt.Fprintf(writer, "  Creating coverage directory: %s\n", coverdataPath)
	if err := os.MkdirAll(coverdataPath, 0777); err != nil {
		return fmt.Errorf("failed to create coverdata directory: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images in PARALLEL
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n  PHASE 1: Building images in parallel...")

	type imageBuildResult struct {
		name  string
		image string
		err   error
	}

	buildResults := make(chan imageBuildResult, 3)
	enableCoverage := os.Getenv("E2E_COVERAGE") == "true" || os.Getenv("GOCOVERDIR") != ""

	// Build EM controller
	go func() {
		cfg := E2EImageConfig{
			ServiceName:    "effectivenessmonitor",
			ImageName:      "kubernaut/effectivenessmonitor",
			DockerfilePath: "docker/effectivenessmonitor-controller.Dockerfile",
			EnableCoverage: enableCoverage,
		}
		image, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{name: "EffectivenessMonitor", image: image, err: err}
	}()

	// Build DataStorage
	go func() {
		cfg := E2EImageConfig{
			ServiceName:    "datastorage",
			ImageName:      "kubernaut/datastorage",
			DockerfilePath: "docker/data-storage.Dockerfile",
			EnableCoverage: enableCoverage,
		}
		image, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{name: "DataStorage", image: image, err: err}
	}()

	// Build AuthWebhook (DD-AUTH-014)
	go func() {
		cfg := E2EImageConfig{
			ServiceName:    "authwebhook",
			ImageName:      "authwebhook",
			DockerfilePath: "docker/authwebhook.Dockerfile",
			EnableCoverage: enableCoverage,
		}
		image, err := BuildImageForKind(cfg, writer)
		buildResults <- imageBuildResult{name: "AuthWebhook", image: image, err: err}
	}()

	// Collect build results
	_, _ = fmt.Fprintln(writer, "  Waiting for all builds to complete...")
	builtImages := make(map[string]string)
	var buildErrors []error
	for i := 0; i < 3; i++ {
		result := <-buildResults
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "    %s build failed: %v\n", result.name, result.err)
			buildErrors = append(buildErrors, result.err)
		} else {
			_, _ = fmt.Fprintf(writer, "    %s built: %s\n", result.name, result.image)
			builtImages[result.name] = result.image
		}
	}
	if len(buildErrors) > 0 {
		return fmt.Errorf("image builds failed: %v", buildErrors)
	}
	_, _ = fmt.Fprintln(writer, "  All images built successfully")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n  PHASE 2: Creating Kind cluster...")

	extraMounts := []ExtraMount{
		{HostPath: coverdataPath, ContainerPath: "/coverdata", ReadOnly: false},
	}

	if err := CreateKindClusterWithExtraMounts(
		clusterName, kubeconfigPath, emE2EKindConfig, extraMounts, writer,
	); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images + Install CRDs
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n  PHASE 3: Loading images and installing CRDs...")

	// Load images into Kind
	for name, image := range builtImages {
		if err := LoadImageToKind(image, strings.ToLower(name), clusterName, writer); err != nil {
			return fmt.Errorf("failed to load %s image: %w", name, err)
		}
	}

	// Load Prometheus and AlertManager images
	for _, img := range []struct{ image, name string }{
		{PrometheusImage, "prometheus"},
		{AlertManagerImage, "alertmanager"},
	} {
		pullCmd := exec.CommandContext(ctx, "podman", "pull", img.image)
		pullCmd.Stdout = writer
		pullCmd.Stderr = writer
		if err := pullCmd.Run(); err != nil {
			return fmt.Errorf("failed to pull %s: %w", img.image, err)
		}
		if err := LoadImageToKind(img.image, img.name, clusterName, writer); err != nil {
			return fmt.Errorf("failed to load %s image: %w", img.image, err)
		}
	}

	// Install CRDs
	crdFiles := []string{
		"kubernaut.ai_effectivenessassessments.yaml",
		"kubernaut.ai_remediationrequests.yaml", // For ownerRef testing
	}
	for _, crdFile := range crdFiles {
		crdPath := filepath.Join(projectRoot, "config/crd/bases", crdFile)
		_, _ = fmt.Fprintf(writer, "    Installing %s...\n", crdFile)
		crdCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", crdPath)
		crdCmd.Stdout = writer
		crdCmd.Stderr = writer
		if err := crdCmd.Run(); err != nil {
			return fmt.Errorf("failed to install %s: %w", crdFile, err)
		}
	}

	// Create namespace
	nsCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
		"create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
	var nsManifest bytes.Buffer
	nsCmd.Stdout = &nsManifest
	nsCmd.Stderr = writer
	if err := nsCmd.Run(); err != nil {
		return fmt.Errorf("failed to generate namespace manifest: %w", err)
	}
	applyCmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	applyCmd.Stdin = &nsManifest
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer
	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy services
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n  PHASE 4: Deploying services...")

	// Deploy DataStorage infrastructure (PostgreSQL + Redis + DS API)
	dsImage := builtImages["DataStorage"]
	if err := DeployDataStorageTestServicesWithNodePort(ctx, namespace, kubeconfigPath, dsImage, 30081, writer); err != nil {
		return fmt.Errorf("failed to deploy DataStorage services: %w", err)
	}

	// Deploy AuthWebhook (DD-AUTH-014)
	awImage := builtImages["AuthWebhook"]
	if err := DeployAuthWebhookManifestsOnly(ctx, clusterName, namespace, kubeconfigPath, awImage, writer); err != nil {
		return fmt.Errorf("failed to deploy AuthWebhook: %w", err)
	}

	// Deploy Prometheus (real instance for metric comparison)
	if err := DeployPrometheus(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Prometheus: %w", err)
	}

	// Deploy AlertManager (real instance for alert resolution)
	if err := DeployAlertManager(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy AlertManager: %w", err)
	}

	// DD-AUTH-014: Deploy data-storage-client ClusterRole and create RoleBinding for EM controller.
	// DeployDataStorageTestServicesWithNodePort does NOT deploy the client ClusterRole,
	// so we must deploy it explicitly before creating the RoleBinding.
	// The EM controller's ServiceAccount needs this so that SAR checks pass when
	// the audit manager writes events to DataStorage.
	_, _ = fmt.Fprintln(writer, "  🔐 Deploying data-storage-client ClusterRole (DD-AUTH-014)...")
	if err := deployDataStorageClientClusterRole(ctx, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy data-storage-client ClusterRole: %w", err)
	}
	_, _ = fmt.Fprintln(writer, "  🔐 Creating DataStorage client RoleBinding for EM controller (DD-AUTH-014)...")
	if err := CreateDataStorageAccessRoleBinding(ctx, namespace, kubeconfigPath, "effectivenessmonitor-controller", writer); err != nil {
		return fmt.Errorf("failed to create EM DataStorage client RoleBinding: %w", err)
	}

	// Deploy EM controller
	emImage := builtImages["EffectivenessMonitor"]
	if err := DeployEMController(ctx, namespace, kubeconfigPath, emImage, writer); err != nil {
		return fmt.Errorf("failed to deploy EM controller: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5: Wait for all services to be ready
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n  PHASE 5: Waiting for services to be ready...")

	promURL := fmt.Sprintf("http://127.0.0.1:%d", PrometheusHostPort)
	if err := WaitForPrometheusReady(promURL, emPrometheusReadyTimeout, writer); err != nil {
		return fmt.Errorf("Prometheus not ready: %w", err)
	}

	amURL := fmt.Sprintf("http://127.0.0.1:%d", AlertManagerHostPort)
	if err := WaitForAlertManagerReady(amURL, emAlertManagerReadyTimeout, writer); err != nil {
		return fmt.Errorf("AlertManager not ready: %w", err)
	}

	// Wait for EM controller pod to be ready
	if err := waitForDeploymentReadyWithTimeout(ctx, kubeconfigPath, namespace, "effectivenessmonitor-controller", 120*time.Second, writer); err != nil {
		return fmt.Errorf("EM controller not ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  EM E2E Infrastructure Ready")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// DeployEMController deploys the Effectiveness Monitor controller into the Kind cluster.
func DeployEMController(ctx context.Context, namespace, kubeconfigPath, imageName string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "    Deploying EM controller (%s)...\n", imageName)

	pullPolicy := GetImagePullPolicy()

	manifest := fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: effectivenessmonitor-config
  namespace: %[1]s
data:
  effectivenessmonitor.yaml: |
    assessment:
      stabilizationWindow: 30s
      validityWindow: 120s
    audit:
      dataStorageUrl: http://data-storage-service:8080
      timeout: 10s
      buffer:
        bufferSize: 100
        batchSize: 10
        flushInterval: 1s
        maxRetries: 3
    controller:
      leaderElection: false
    external:
      prometheusUrl: http://prometheus-svc:9090
      prometheusEnabled: true
      alertManagerUrl: http://alertmanager-svc:9093
      alertManagerEnabled: true
      connectionTimeout: 10s
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: effectivenessmonitor-controller
  namespace: %[1]s
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: effectivenessmonitor-controller
rules:
- apiGroups: ["kubernaut.ai"]
  resources: ["effectivenessassessments"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["effectivenessassessments/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["kubernaut.ai"]
  resources: ["remediationrequests"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: effectivenessmonitor-controller
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: effectivenessmonitor-controller
subjects:
- kind: ServiceAccount
  name: effectivenessmonitor-controller
  namespace: %[1]s
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: effectivenessmonitor-controller
  namespace: %[1]s
  labels:
    app: effectivenessmonitor-controller
spec:
  replicas: 1
  selector:
    matchLabels:
      app: effectivenessmonitor-controller
  template:
    metadata:
      labels:
        app: effectivenessmonitor-controller
    spec:
      serviceAccountName: effectivenessmonitor-controller
      containers:
      - name: controller
        image: %[2]s
        imagePullPolicy: %[3]s
        args:
        - "--config=/etc/effectivenessmonitor/effectivenessmonitor.yaml"
        - "--metrics-bind-address=:9090"
        - "--health-probe-bind-address=:8081"
        ports:
        - containerPort: 9090
          name: metrics
        - containerPort: 8081
          name: health
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 10
          periodSeconds: 10
        volumeMounts:
        - name: config
          mountPath: /etc/effectivenessmonitor
        - name: coverdata
          mountPath: /coverdata
        env:
        - name: GOCOVERDIR
          value: /coverdata
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "256Mi"
            cpu: "500m"
      volumes:
      - name: config
        configMap:
          name: effectivenessmonitor-config
      - name: coverdata
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate
`, namespace, imageName, pullPolicy)

	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = bytes.NewBufferString(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy EM controller: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "    EM controller deployed")
	return nil
}

// waitForDeploymentReadyWithTimeout waits for a deployment to have at least one ready replica.
func waitForDeploymentReadyWithTimeout(ctx context.Context, kubeconfigPath, namespace, name string, timeout time.Duration, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "    Waiting for %s to be ready...\n", name)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath,
			"get", "deployment", name, "-n", namespace,
			"-o", "jsonpath={.status.readyReplicas}")
		out, err := cmd.Output()
		if err == nil && string(out) == "1" {
			_, _ = fmt.Fprintf(writer, "    %s is ready\n", name)
			return nil
		}
		time.Sleep(3 * time.Second)
	}

	return fmt.Errorf("timeout waiting for deployment %s/%s after %v", namespace, name, timeout)
}
