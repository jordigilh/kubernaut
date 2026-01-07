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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func CreateAIAnalysisClusterHybrid(clusterName, kubeconfigPath string, writer io.Writer) error {
	ctx := context.Background()
	namespace := "kubernaut-system" // Infrastructure always in kubernaut-system; tests use dynamic namespaces

	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 AIAnalysis E2E Infrastructure (HYBRID PARALLEL + DISK OPTIMIZATION)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Build → Export → Prune → Cluster → Load → Deploy")
	_, _ = fmt.Fprintln(writer, "  Benefits: Fast builds + Aggressive cleanup + Disk tracking")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-002: Hybrid Parallel Setup Standard")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-008: Disk Space Management")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Track initial disk space
	LogDiskSpace("START", writer)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images IN PARALLEL (before cluster creation)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images in parallel...")
	_, _ = fmt.Fprintln(writer, "  ├── Data Storage (1-2 min)")
	_, _ = fmt.Fprintln(writer, "  ├── HolmesGPT-API (2-3 min)")
	_, _ = fmt.Fprintln(writer, "  └── AIAnalysis controller (3-4 min)")

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
			_, _ = fmt.Fprintf(writer, "   📊 Building AIAnalysis with coverage (GOFLAGS=-cover)\n")
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
		_, _ = fmt.Fprintf(writer, "  ✅ %s image built\n", result.name)
	}
	_, _ = fmt.Fprintln(writer, "\n✅ All images built! (~3-4 min parallel)")
	LogDiskSpace("IMAGES_BUILT", writer)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2-3: Export images to .tar and aggressive Podman cleanup
	// ═══════════════════════════════════════════════════════════════════════
	// This frees ~5-9 GB of disk space by removing Podman cache and intermediate layers
	tarFiles, err := ExportImagesAndPrune(builtImages, "/tmp", writer)
	if err != nil {
		return fmt.Errorf("failed to export images and prune: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Create Kind cluster (AFTER cleanup to maximize available space)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4: Creating Kind cluster...")
	if err := createAIAnalysisKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "📁 Creating namespace...")
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

	_, _ = fmt.Fprintln(writer, "📋 Installing AIAnalysis CRD...")
	if err := installAIAnalysisCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install AIAnalysis CRD: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 5-6: Load images from .tar into Kind and cleanup .tar files
	// ═══════════════════════════════════════════════════════════════════════
	// Uses shared helpers for efficient loading and cleanup
	if err := LoadImagesAndCleanup(clusterName, tarFiles, writer); err != nil {
		return fmt.Errorf("failed to load images and cleanup: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 7: Deploy services IN PARALLEL (let Kubernetes reconcile)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 7: Deploying services in parallel...")
	_, _ = fmt.Fprintln(writer, "  ├── Data Storage infrastructure (PostgreSQL + Redis + DataStorage + Migrations)")
	_, _ = fmt.Fprintln(writer, "  ├── HolmesGPT-API")
	_, _ = fmt.Fprintln(writer, "  └── AIAnalysis controller")
	_, _ = fmt.Fprintln(writer, "  ⏱️  Kubernetes will handle dependencies via readiness probes")

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
	// NOTE: Images already loaded in Phase 5-6, skip image loading in deployment
	go func() {
		err := deployHolmesGPTAPIManifestOnly(kubeconfigPath, builtImages["holmesgpt-api"], writer)
		deployResults <- deployResult{"HolmesGPT-API", err}
	}()

	// Deploy AIAnalysis controller (service under test)
	// NOTE: Images already loaded in Phase 5-6, skip image loading in deployment
	go func() {
		err := deployAIAnalysisControllerManifestOnly(kubeconfigPath, builtImages["aianalysis"], writer)
		deployResults <- deployResult{"AIAnalysis", err}
	}()

	// Collect deployment results (kubectl apply results)
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for manifest applications...")
	for i := 0; i < 3; i++ {
		result := <-deployResults
		if result.err != nil {
			return fmt.Errorf("failed to deploy %s: %w", result.name, result.err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s deployed\n", result.name)
	}
	_, _ = fmt.Fprintln(writer, "✅ All services deployed! (Kubernetes reconciling...)")

	// Wait for ALL services to be ready (handles dependencies via readiness probes)
	// Per DD-TEST-002: Coverage-instrumented binaries take longer to start (2-5 min vs 30s)
	// Kubernetes reconciles dependencies:
	// - DataStorage waits for PostgreSQL + Redis (retry logic + readiness probe)
	// - HolmesGPT-API waits for PostgreSQL (retry logic + readiness probe)
	// - AIAnalysis waits for HAPI + DataStorage (retry logic + readiness probe)
	// This single wait point validates the entire dependency chain
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for all services to be ready (Kubernetes reconciling dependencies)...")
	if err := waitForAllServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ AIAnalysis E2E Infrastructure Ready (DD-TEST-002 + DD-TEST-008)")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	LogDiskSpace("FINAL", writer)
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

func DeleteAIAnalysisCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "🗑️  Deleting AIAnalysis E2E cluster...")

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

	_, _ = fmt.Fprintln(writer, "✅ Cluster deleted")
	return nil
}

func createAIAnalysisKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	// REFACTORED: Now uses shared CreateKindClusterWithConfig() helper
	// Authority: docs/handoff/TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md (Phase 1)
	opts := KindClusterOptions{
		ClusterName:               clusterName,
		KubeconfigPath:            kubeconfigPath,
		ConfigPath:                "test/infrastructure/kind-aianalysis-config.yaml",
		WaitTimeout:               "60s",
		DeleteExisting:            false,
		ReuseExisting:             true,                  // Original behavior: reuse if exists
		CleanupOrphanedContainers: true,                  // Original behavior: cleanup Podman containers on macOS
	}
	if err := CreateKindClusterWithConfig(opts, writer); err != nil {
		return err
	}

	// Wait for cluster to be ready (original behavior preserved)
	return waitForClusterReady(kubeconfigPath, writer)
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
	_, _ = fmt.Fprintln(writer, "  Waiting for CRD to be established...")
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

func deployHolmesGPTAPIManifestOnly(kubeconfigPath, imageName string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  Applying HolmesGPT-API manifest (image already in Kind)...")
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
    audit:
      flush_interval_seconds: 0.1
      buffer_size: 10000
      batch_size: 50
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
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func deployAIAnalysisControllerManifestOnly(kubeconfigPath, imageName string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "  Applying AIAnalysis controller manifest (image already in Kind)...")
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
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return err
	}

	// Deploy Rego policy ConfigMap
	return deployRegoPolicyConfigMap(kubeconfigPath, writer)
}

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
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for DataStorage pod to be ready...\n")
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
	_, _ = fmt.Fprintf(writer, "   ✅ DataStorage ready\n")

	// Wait for HolmesGPT-API pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for HolmesGPT-API pod to be ready...\n")

	// Track polling attempts for debugging
	pollCount := 0
	maxPolls := int((2 * time.Minute) / (5 * time.Second)) // 24 polls expected

	Eventually(func() bool {
		pollCount++
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=holmesgpt-api",
		})
		if err != nil {
			_, _ = fmt.Fprintf(writer, "      [Poll %d/%d] Error listing HAPI pods: %v\n", pollCount, maxPolls, err)
			return false
		}
		if len(pods.Items) == 0 {
			_, _ = fmt.Fprintf(writer, "      [Poll %d/%d] No HAPI pods found\n", pollCount, maxPolls)
			return false
		}

		// Debug: Show pod status every 4 polls (~20 seconds)
		for _, pod := range pods.Items {
			if pollCount%4 == 0 {
				_, _ = fmt.Fprintf(writer, "      [Poll %d/%d] HAPI pod '%s': Phase=%s, Ready=",
					pollCount, maxPolls, pod.Name, pod.Status.Phase)
				isReady := false
				for _, condition := range pod.Status.Conditions {
					if condition.Type == corev1.PodReady {
						_, _ = fmt.Fprintf(writer, "%s", condition.Status)
						if condition.Status != corev1.ConditionTrue {
							_, _ = fmt.Fprintf(writer, " (Reason: %s, Message: %s)", condition.Reason, condition.Message)
						}
						isReady = condition.Status == corev1.ConditionTrue
						break
					}
				}
				if !isReady {
					// Show container statuses for debugging
					for _, containerStatus := range pod.Status.ContainerStatuses {
						if !containerStatus.Ready {
							_, _ = fmt.Fprintf(writer, "\n         Container '%s': Ready=%t, RestartCount=%d",
								containerStatus.Name, containerStatus.Ready, containerStatus.RestartCount)
							if containerStatus.State.Waiting != nil {
								_, _ = fmt.Fprintf(writer, ", Waiting: %s (%s)",
									containerStatus.State.Waiting.Reason, containerStatus.State.Waiting.Message)
							}
							if containerStatus.State.Terminated != nil {
								_, _ = fmt.Fprintf(writer, ", Terminated: ExitCode=%d, Reason=%s",
									containerStatus.State.Terminated.ExitCode, containerStatus.State.Terminated.Reason)
							}
						}
					}
				}
				_, _ = fmt.Fprintf(writer, "\n")
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
	_, _ = fmt.Fprintf(writer, "   ✅ HolmesGPT-API ready\n")

	// Wait for AIAnalysis controller pod to be ready
	// Note: Coverage-instrumented binaries may take longer to start
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for AIAnalysis controller pod to be ready...\n")
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
	_, _ = fmt.Fprintf(writer, "   ✅ AIAnalysis controller ready\n")

	return nil
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
	_, _ = fmt.Fprintln(writer, "  Waiting for cluster to be ready...")
	for i := 0; i < 60; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "nodes", "-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}")
		output, err := cmd.Output()
		if err == nil && containsReady(string(output)) {
			_, _ = fmt.Fprintln(writer, "  Cluster nodes ready")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for cluster")
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
	_ = pipeWriter2.Close()
	return applyCmd.Wait()
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

func containsReady(s string) bool {
	return len(s) > 0 && s != "" && (s == "True" || s == "True True")
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
	cmd.Stdin = strings.NewReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}
