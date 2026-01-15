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
	"strings"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// SetupHAPIInfrastructure sets up HolmesGPT API E2E infrastructure
// Deploys: PostgreSQL + Redis + Data Storage + HAPI to Kind cluster
// Uses sequential builds to avoid OOM with Python pip install
//
// Port Allocations (per DD-TEST-001 v1.8):
// - HAPI: NodePort 30120 → Container 8080
// - Data Storage: NodePort 30098 → Container 8080
// - PostgreSQL: NodePort 30439 → Container 5432
// - Redis: NodePort 30387 → Container 6379
func SetupHAPIInfrastructure(ctx context.Context, clusterName, kubeconfigPath, namespace string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "🚀 HAPI E2E Infrastructure Setup")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "  Strategy: Sequential builds → Create cluster → Deploy services")
	_, _ = fmt.Fprintln(writer, "  Duration: ~5-7 minutes (sequential to avoid Python build OOM)")
	_, _ = fmt.Fprintln(writer, "  Per DD-TEST-001 v1.8: Dedicated HAPI ports")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	projectRoot := getProjectRoot()

	// Generate unique image tags per DD-TEST-001: {infrastructure}:{consumer}-{uuid}
	dataStorageImage := GenerateInfraImageName("datastorage", "holmesgpt-api")
	hapiImage := GenerateInfraImageName("holmesgpt-api", "holmesgpt-api")
	mockLLMImage := GenerateInfraImageName("mock-llm", "holmesgpt-api-e2e")

	_, _ = fmt.Fprintf(writer, "  📦 Data Storage image: %s\n", dataStorageImage)
	_, _ = fmt.Fprintf(writer, "  📦 HAPI image: %s\n", hapiImage)
	_, _ = fmt.Fprintf(writer, "  📦 Mock LLM image: %s\n", mockLLMImage)

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Build images SEQUENTIALLY (Data Storage, then HAPI)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 1: Building images sequentially...")
	_, _ = fmt.Fprintln(writer, "  (Sequential to avoid OOM - Python pip uses 2-3GB)")
	_, _ = fmt.Fprintln(writer, "  ├── Data Storage (1-2 min)")
	_, _ = fmt.Fprintln(writer, "  └── HolmesGPT-API (2-3 min)")

	// Build Data Storage
	_, _ = fmt.Fprintf(writer, "🔨 [%s] Building Data Storage...\n", time.Now().Format("15:04:05"))
	if err := buildImageOnly("Data Storage", dataStorageImage,
		"docker/data-storage.Dockerfile", projectRoot, writer); err != nil {
		return fmt.Errorf("failed to build datastorage image: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ [%s] Data Storage image built: %s\n", time.Now().Format("15:04:05"), dataStorageImage)

	// Build HAPI (using E2E Dockerfile with minimal dependencies)
	// Uses requirements-e2e.txt (no google-cloud-aiplatform 1.5GB)
	// Expected: 2-3 minutes (vs 5-15 minutes with full Dockerfile)
	_, _ = fmt.Fprintf(writer, "🔨 [%s] Building HolmesGPT-API (E2E - minimal deps)...\n", time.Now().Format("15:04:05"))
	if err := buildImageOnly("HolmesGPT-API (E2E)", hapiImage,
		"holmesgpt-api/Dockerfile.e2e", projectRoot, writer); err != nil {
		return fmt.Errorf("failed to build holmesgpt-api E2E image: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ [%s] HolmesGPT-API image built: %s\n", time.Now().Format("15:04:05"), hapiImage)

	// Build Mock LLM (standalone test service)
	// Zero external dependencies, minimal build time
	// Expected: <1 minute
	_, _ = fmt.Fprintf(writer, "🔨 [%s] Building Mock LLM...\n", time.Now().Format("15:04:05"))
	if err := buildImageOnly("Mock LLM", mockLLMImage,
		"test/services/mock-llm/Dockerfile", projectRoot, writer); err != nil {
		return fmt.Errorf("failed to build mock-llm image: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "✅ [%s] Mock LLM image built: %s\n", time.Now().Format("15:04:05"), mockLLMImage)

	_, _ = fmt.Fprintln(writer, "\n✅ All images built sequentially!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Create Kind cluster
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 2: Creating Kind cluster...")
	if err := createHAPIKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Load images in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 3: Loading images in parallel...")
	type imageLoadResult struct {
		name string
		err  error
	}
	loadResults := make(chan imageLoadResult, 3)

	go func() {
		err := loadImageToKind(clusterName, dataStorageImage, writer)
		loadResults <- imageLoadResult{"DataStorage", err}
	}()
	go func() {
		err := loadImageToKind(clusterName, hapiImage, writer)
		loadResults <- imageLoadResult{"HolmesGPT-API", err}
	}()
	go func() {
		err := loadImageToKind(clusterName, mockLLMImage, writer)
		loadResults <- imageLoadResult{"Mock LLM", err}
	}()

	for i := 0; i < 3; i++ {
		result := <-loadResults
		if result.err != nil {
			return fmt.Errorf("failed to load %s: %w", result.name, result.err)
		}
		_, _ = fmt.Fprintf(writer, "  ✅ %s image loaded\n", result.name)
	}
	_, _ = fmt.Fprintln(writer, "✅ All images loaded!")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Deploy services in PARALLEL (DD-TEST-002 MANDATE)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintln(writer, "\n📦 PHASE 4: Deploying services in parallel...")
	_, _ = fmt.Fprintln(writer, "  (Kubernetes will handle dependencies and reconciliation)")

	// Create namespace FIRST (required for all subsequent deployments)
	_, _ = fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	type deployResult struct {
		name string
		err  error
	}
	deployResults := make(chan deployResult, 6)

	// Launch ALL kubectl apply commands concurrently
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
	go func() {
		// Use NodePort 30098 for HAPI E2E (per DD-TEST-001 v1.8)
		err := deployDataStorageServiceInNamespaceWithNodePort(ctx, namespace, kubeconfigPath, dataStorageImage, 30098, writer)
		deployResults <- deployResult{"DataStorage", err}
	}()
	go func() {
		err := deployHAPIOnly(clusterName, kubeconfigPath, namespace, hapiImage, writer)
		deployResults <- deployResult{"HolmesGPT-API", err}
	}()
	go func() {
		// HAPI E2E: No workflow seeding, Mock LLM uses default UUIDs
		err := deployMockLLMInNamespace(ctx, namespace, kubeconfigPath, mockLLMImage, nil, writer)
		deployResults <- deployResult{"Mock LLM", err}
	}()

	// Collect ALL results before proceeding (MANDATORY)
	var deployErrors []error
	for i := 0; i < 6; i++ {
		result := <-deployResults
		if result.err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s deployment failed: %v\n", result.name, result.err)
			deployErrors = append(deployErrors, result.err)
		} else {
			_, _ = fmt.Fprintf(writer, "  ✅ %s manifests applied\n", result.name)
		}
	}

	if len(deployErrors) > 0 {
		return fmt.Errorf("one or more service deployments failed: %v", deployErrors)
	}
	_, _ = fmt.Fprintln(writer, "  ✅ All manifests applied! (Kubernetes reconciling...)")

	// Single wait for ALL services ready (Kubernetes handles dependencies)
	_, _ = fmt.Fprintln(writer, "\n⏳ Waiting for all services to be ready (Kubernetes reconciling dependencies)...")
	if err := waitForHAPIServicesReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	_, _ = fmt.Fprintln(writer, "✅ HAPI E2E Infrastructure Ready")
	_, _ = fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// createHAPIKindCluster creates a Kind cluster with HAPI-specific port mappings
// Per DD-TEST-001 v1.8
func createHAPIKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	kindConfig := `kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  # HolmesGPT API (HAPI) - Per DD-TEST-001 v1.8
  - containerPort: 30120
    hostPort: 30120
    protocol: TCP
  # Data Storage - Per DD-TEST-001 v1.8
  - containerPort: 30098
    hostPort: 30098
    protocol: TCP
  # PostgreSQL - Per DD-TEST-001 v1.8
  - containerPort: 30439
    hostPort: 30439
    protocol: TCP
  # Redis - Per DD-TEST-001 v1.8
  - containerPort: 30387
    hostPort: 30387
    protocol: TCP
`

	// Write kind config to temp file
	tmpfile, err := os.CreateTemp("", "kind-hapi-e2e-*.yaml")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	if _, err := tmpfile.Write([]byte(kindConfig)); err != nil {
		return err
	}
	if err := tmpfile.Close(); err != nil {
		return err
	}

	// Create Kind cluster
	cmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", tmpfile.Name(),
		"--kubeconfig", kubeconfigPath)
	cmd.Stdout = writer
	cmd.Stderr = writer

	return cmd.Run()
}

// deployDataStorageForHAPI deploys Data Storage service to Kind cluster
// Uses HAPI-specific NodePort (30098) per DD-TEST-001 v1.8
// deployHAPIOnly deploys HAPI service to Kind cluster
// Per DD-TEST-001 v1.8: NodePort 30120
func deployHAPIOnly(clusterName, kubeconfigPath, namespace, imageTag string, writer io.Writer) error {
	// ADR-030: Create HAPI ConfigMap with minimal config
	deployment := fmt.Sprintf(`apiVersion: v1
kind: ConfigMap
metadata:
  name: holmesgpt-api-config
  namespace: %s
data:
  config.yaml: |
    logging:
      level: "INFO"
    llm:
      provider: "openai"
      model: "mock-model"
      endpoint: "http://mock-llm:8080"
    data_storage:
      url: "http://datastorage:8080"
    audit:
      flush_interval_seconds: 0.1
      buffer_size: 10000
      batch_size: 50
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
  namespace: %s
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
        - name: LLM_ENDPOINT
          value: "http://mock-llm:8080"
        - name: LLM_MODEL
          value: "mock-model"
        - name: LLM_PROVIDER
          value: "openai"
        - name: OPENAI_API_KEY
          value: "mock-api-key-for-e2e"
        - name: DATA_STORAGE_URL
          value: "http://datastorage:8080"
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
  namespace: %s
spec:
  type: NodePort
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30120
  selector:
    app: holmesgpt-api
`, namespace, namespace, imageTag, namespace)

	// Apply manifest
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(deployment)
	cmd.Stdout = writer
	cmd.Stderr = writer

	return cmd.Run()
}

// waitForHAPIServicesReady waits for DataStorage and HolmesGPT-API pods to be ready
// Per DD-TEST-002: Single readiness check after parallel deployment
func waitForHAPIServicesReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
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
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=holmesgpt-api",
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
	}, 3*time.Minute, 5*time.Second).Should(BeTrue(), "HolmesGPT-API pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ HolmesGPT-API ready\n")

	return nil
}

// deployMockLLMInNamespace deploys the standalone Mock LLM service to a namespace
// This is the V2.0 Mock LLM service extracted from HAPI business code
// Uses ClusterIP for internal access only (no NodePort needed for E2E)
func deployMockLLMInNamespace(ctx context.Context, namespace, kubeconfigPath, imageTag string, workflowUUIDs map[string]string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "   📦 Deploying Mock LLM service (image: %s)...\n", imageTag)

	// Use the manifests from deploy/mock-llm/ with the provided image tag
	deployment := fmt.Sprintf(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: mock-llm
  namespace: %s
  labels:
    app: mock-llm
    component: test-infrastructure
spec:
  replicas: 1
  selector:
    matchLabels:
      app: mock-llm
  template:
    metadata:
      labels:
        app: mock-llm
        component: test-infrastructure
    spec:
      containers:
      - name: mock-llm
        image: %s
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        env:
        - name: MOCK_LLM_HOST
          value: "0.0.0.0"
        - name: MOCK_LLM_PORT
          value: "8080"
        - name: MOCK_LLM_FORCE_TEXT
          value: "false"
        - name: MOCK_LLM_CONFIG_PATH
          value: "/config/scenarios.yaml"
        volumeMounts:
        - name: scenarios-config
          mountPath: /config
          readOnly: true
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
            scheme: HTTP
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 3
          successThreshold: 1
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
            scheme: HTTP
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 3
          successThreshold: 1
          failureThreshold: 3
        resources:
          requests:
            memory: "64Mi"
            cpu: "100m"
          limits:
            memory: "128Mi"
            cpu: "200m"
        securityContext:
          allowPrivilegeEscalation: false
          runAsNonRoot: true
          runAsUser: 1001
          capabilities:
            drop:
            - ALL
      volumes:
      - name: scenarios-config
        configMap:
          name: mock-llm-scenarios
      securityContext:
        fsGroup: 1001
      restartPolicy: Always
---
apiVersion: v1
kind: Service
metadata:
  name: mock-llm
  namespace: %s
  labels:
    app: mock-llm
    component: test-infrastructure
spec:
  type: ClusterIP
  ports:
  - port: 8080
    targetPort: 8080
    protocol: TCP
    name: http
  selector:
    app: mock-llm
`, namespace, imageTag, namespace)

	cmd := exec.CommandContext(ctx, "kubectl", "apply", "-f", "-", "--kubeconfig", kubeconfigPath)
	cmd.Stdin = strings.NewReader(deployment)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Mock LLM: %w", err)
	}

	// Get Kubernetes client
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}

	// Wait for Mock LLM pod to be ready
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for Mock LLM pod to be ready...\n")
	Eventually(func() bool {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: "app=mock-llm",
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
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Mock LLM pod should become ready")
	_, _ = fmt.Fprintf(writer, "   ✅ Mock LLM ready\n")

	return nil
}
