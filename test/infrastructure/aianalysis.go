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
	"net"
	"net/http"
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

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// AIAnalysis E2E Infrastructure
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Port Allocation (per DD-TEST-001):
//   AIAnalysis:    Host 8084  → NodePort 30084 (API), 30184 (metrics), 30284 (health)
//   Data Storage:  Host 8081  → NodePort 30081 (API)
//   HolmesGPT-API: Host 8088  → NodePort 30088 (API)
//   PostgreSQL:    Host 5433  → NodePort 30433
//   Redis:         Host 6380  → NodePort 30380
//
// Dependencies:
//   AIAnalysis → HolmesGPT-API (AI analysis)
//   AIAnalysis → Data Storage (audit events)
//   HolmesGPT-API → Data Storage (workflow catalog, audit)
//   Data Storage → PostgreSQL (persistence)
//   Data Storage → Redis (caching/DLQ)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	// AIAnalysis ports per DD-TEST-001
	AIAnalysisHostPort    = 8084
	AIAnalysisNodePort    = 30084
	AIAnalysisMetricsPort = 30184
	AIAnalysisHealthPort  = 30284

	// Data Storage ports
	DataStorageHostPort = 8081
	DataStorageNodePort = 30081

	// HolmesGPT-API ports
	HolmesGPTAPIHostPort = 8088
	HolmesGPTAPINodePort = 30088

	// PostgreSQL ports
	PostgreSQLHostPort = 5433
	PostgreSQLNodePort = 30433

	// Redis ports
	RedisHostPort = 6380
	RedisNodePort = 30380
)

// CreateAIAnalysisCluster creates a Kind cluster for AIAnalysis E2E testing.
// This sets up the complete dependency chain:
// 1. Kind cluster with port mappings
// 2. AIAnalysis CRD
// 3. PostgreSQL + Redis (for Data Storage)
// 4. Data Storage service
// 5. HolmesGPT-API service
// 6. AIAnalysis controller
//
// Time: ~2-3 minutes
func CreateAIAnalysisCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "AIAnalysis E2E Cluster Setup")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Dependencies:")
	fmt.Fprintln(writer, "  • PostgreSQL (port 5433) - Data Storage persistence")
	fmt.Fprintln(writer, "  • Redis (port 6380) - Data Storage caching")
	fmt.Fprintln(writer, "  • Data Storage (port 8081) - Audit trail")
	fmt.Fprintln(writer, "  • HolmesGPT-API (port 8088) - AI analysis")
	fmt.Fprintln(writer, "  • AIAnalysis (port 8084) - CRD controller")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Create context for infrastructure deployment
	ctx := context.Background()
	namespace := "kubernaut-system"

	// 1. Create Kind cluster with AIAnalysis config
	fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createAIAnalysisKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Create namespace for deployments (ignore if already exists)
	fmt.Fprintln(writer, "📁 Creating namespace...")
	createNsCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"create", "namespace", namespace)
	nsOutput := &strings.Builder{}
	createNsCmd.Stdout = io.MultiWriter(writer, nsOutput)
	createNsCmd.Stderr = io.MultiWriter(writer, nsOutput)
	if err := createNsCmd.Run(); err != nil {
		// Ignore if namespace already exists
		if !strings.Contains(nsOutput.String(), "AlreadyExists") {
			return fmt.Errorf("failed to create namespace: %w", err)
		}
		fmt.Fprintln(writer, "  (namespace already exists, continuing...)")
	}

	// 3. Install AIAnalysis CRD
	fmt.Fprintln(writer, "📋 Installing AIAnalysis CRD...")
	if err := installAIAnalysisCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install AIAnalysis CRD: %w", err)
	}

	// 4. Deploy PostgreSQL (using shared function from datastorage.go)
	fmt.Fprintln(writer, "🐘 Deploying PostgreSQL...")
	if err := deployPostgreSQLInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 5. Deploy Redis (using shared function from datastorage.go)
	fmt.Fprintln(writer, "🔴 Deploying Redis...")
	if err := deployRedisInNamespace(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 5. Wait for infrastructure to be ready
	fmt.Fprintln(writer, "⏳ Waiting for PostgreSQL and Redis to be ready...")
	if err := waitForAIAnalysisInfraReady(ctx, namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("infrastructure not ready: %w", err)
	}

	// 7. Build and deploy Data Storage (now safe - dependencies ready)
	fmt.Fprintln(writer, "💾 Building and deploying Data Storage...")
	if err := deployDataStorage(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage: %w", err)
	}

	// 8. Build and deploy HolmesGPT-API
	fmt.Fprintln(writer, "🤖 Building and deploying HolmesGPT-API...")
	if err := deployHolmesGPTAPI(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy HolmesGPT-API: %w", err)
	}

	// 9. Build and deploy AIAnalysis controller
	fmt.Fprintln(writer, "🧠 Building and deploying AIAnalysis controller...")
	if err := deployAIAnalysisController(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy AIAnalysis controller: %w", err)
	}

	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "✅ AIAnalysis E2E cluster ready!")
	fmt.Fprintf(writer, "  • AIAnalysis API: http://localhost:%d\n", AIAnalysisHostPort)
	fmt.Fprintf(writer, "  • AIAnalysis Metrics: http://localhost:%d/metrics\n", 9184)
	fmt.Fprintf(writer, "  • Data Storage: http://localhost:%d\n", DataStorageHostPort)
	fmt.Fprintf(writer, "  • HolmesGPT-API: http://localhost:%d\n", HolmesGPTAPIHostPort)
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// DeleteAIAnalysisCluster deletes the Kind cluster
func DeleteAIAnalysisCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "🗑️  Deleting AIAnalysis E2E cluster...")

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

	fmt.Fprintln(writer, "✅ Cluster deleted")
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Internal helper functions
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

func createAIAnalysisKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Find config file
	configPath := findAIAnalysisKindConfig()
	if configPath == "" {
		return fmt.Errorf("kind-aianalysis-config.yaml not found")
	}

	// Check if cluster already exists (with cleanup if partial state)
	checkCmd := exec.Command("kind", "get", "clusters")
	output, _ := checkCmd.Output()
	if containsCluster(string(output), clusterName) {
		fmt.Fprintln(writer, "  Cluster already exists, reusing...")
		return exportKubeconfig(clusterName, kubeconfigPath, writer)
	}

	// Clean up any leftover Podman containers from previous failed runs
	// This fixes issues on macOS where Kind/Podman can leave orphaned containers
	fmt.Fprintln(writer, "  Cleaning up any leftover containers...")
	cleanupContainers := []string{
		clusterName + "-control-plane",
		clusterName + "-worker",
	}
	for _, containerName := range cleanupContainers {
		cleanupCmd := exec.Command("podman", "rm", "-f", containerName)
		_ = cleanupCmd.Run() // Ignore errors - container may not exist
	}

	// Ensure kubeconfig directory exists
	kubeconfigDir := filepath.Dir(kubeconfigPath)
	if err := os.MkdirAll(kubeconfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	// Remove any leftover kubeconfig lock file
	lockFile := kubeconfigPath + ".lock"
	_ = os.Remove(lockFile) // Ignore errors - file may not exist

	// Create cluster
	cmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", configPath,
		"--kubeconfig", kubeconfigPath,
	)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	// Wait for cluster to be ready
	return waitForClusterReady(kubeconfigPath, writer)
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
	fmt.Fprintln(writer, "  Waiting for cluster to be ready...")
	for i := 0; i < 60; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "nodes", "-o", "jsonpath={.items[*].status.conditions[?(@.type=='Ready')].status}")
		output, err := cmd.Output()
		if err == nil && containsReady(string(output)) {
			fmt.Fprintln(writer, "  Cluster nodes ready")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("timeout waiting for cluster")
}

func containsReady(s string) bool {
	return len(s) > 0 && s != "" && (s == "True" || s == "True True")
}

func installAIAnalysisCRD(kubeconfigPath string, writer io.Writer) error {
	// Find CRD file
	crdPath := findCRDFile("aianalysis.kubernaut.ai_aianalyses.yaml")
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
	fmt.Fprintln(writer, "  Waiting for CRD to be established...")
	for i := 0; i < 30; i++ {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"get", "crd", "aianalyses.aianalysis.kubernaut.ai")
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("timeout waiting for CRD")
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

// NOTE: deployPostgreSQL, createInlinePostgreSQL, and deployRedis functions removed
// Now using shared functions from datastorage.go:
// - deployPostgreSQLInNamespace (used by Gateway, SignalProcessing, Notification, DataStorage)
// - deployRedisInNamespace (used by Gateway, SignalProcessing, Notification, DataStorage)
// This reduces code duplication and ensures consistent infrastructure across all services

func deployDataStorage(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Apply database migrations BEFORE deploying Data Storage
	// This ensures audit_events and workflow_catalog tables exist
	// Using shared migration library from DS_E2E_MIGRATION_LIBRARY_IMPLEMENTATION_SCHEDULE.md
	fmt.Fprintln(writer, "  📋 Applying database migrations (shared library)...")
	ctx := context.Background()

	// AIAnalysis needs audit_events + workflow catalog
	config := DefaultMigrationConfig("kubernaut-system", kubeconfigPath)
	config.Tables = []string{"audit_events", "remediation_workflow_catalog"}
	if err := ApplyMigrationsWithConfig(ctx, config, writer); err != nil {
		fmt.Fprintf(writer, "  ⚠️  Migration warning (may already be applied): %v\n", err)
		// Don't fail - tables might already exist or DS handles migrations
	}

	// Verify critical tables exist
	verifyConfig := DefaultMigrationConfig("kubernaut-system", kubeconfigPath)
	verifyConfig.Tables = []string{"audit_events"}
	if err := VerifyMigrations(ctx, verifyConfig, writer); err != nil {
		fmt.Fprintf(writer, "  ⚠️  Verification warning: %v\n", err)
		// Continue anyway - DS may self-heal
	}

	// Get project root for build context
	projectRoot := getProjectRoot()

	// Build Data Storage image
	fmt.Fprintln(writer, "  Building Data Storage image...")
	// Try podman build first (macOS)
	buildCmd := exec.Command("podman", "build", "-t", "kubernaut-datastorage:latest",
		"-f", "docker/data-storage.Dockerfile", ".")
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build Data Storage with podman: %w", err)
	}

	// Load into Kind
	fmt.Fprintln(writer, "  Loading Data Storage image into Kind...")
	if err := loadImageToKind(clusterName, "kubernaut-datastorage:latest", writer); err != nil {
		return fmt.Errorf("failed to load image: %w", err)
	}

	// Deploy manifest with ConfigMap (ADR-030 authoritative pattern)
	manifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
  namespace: kubernaut-system
data:
  config.yaml: |
    server:
      host: 0.0.0.0
      port: 8080
      timeout: 30s
      graceful_shutdown_timeout: 30s
    postgres:
      host: postgres
      port: 5432
      database: action_history
      max_connections: 25
      secrets_file: /etc/datastorage/secrets/db-secrets.yaml
    redis:
      host: redis
      port: 6379
      secrets_file: /etc/datastorage/secrets/redis-secrets.yaml
    cache:
      default_ttl: 300s
      embedding_ttl: 3600s
    logging:
      level: debug
      format: json
---
apiVersion: v1
kind: Secret
metadata:
  name: datastorage-secret
  namespace: kubernaut-system
stringData:
  db-secrets.yaml: |
    username: slm_user
    password: test_password
  redis-secrets.yaml: |
    password: ""
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datastorage
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: datastorage
  template:
    metadata:
      labels:
        app: datastorage
    spec:
      containers:
      - name: datastorage
        image: localhost/kubernaut-datastorage:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
        env:
        - name: CONFIG_PATH
          value: /etc/datastorage/config.yaml
        volumeMounts:
        - name: config
          mountPath: /etc/datastorage
          readOnly: true
        - name: secrets
          mountPath: /etc/datastorage/secrets
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: secrets
        secret:
          secretName: datastorage-secret
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: datastorage
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: 30081
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func deployHolmesGPTAPI(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Get project root for build context
	projectRoot := getProjectRoot()

	// Build HolmesGPT-API image
	fmt.Fprintln(writer, "  Building HolmesGPT-API image...")
	// Try podman build first (macOS)
	buildCmd := exec.Command("podman", "build", "-t", "kubernaut-holmesgpt-api:latest",
		"-f", "holmesgpt-api/Dockerfile", ".")
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build HolmesGPT-API with podman: %w", err)
	}

	// Load into Kind
	fmt.Fprintln(writer, "  Loading HolmesGPT-API image into Kind...")
	if err := loadImageToKind(clusterName, "kubernaut-holmesgpt-api:latest", writer); err != nil {
		return fmt.Errorf("failed to load HolmesGPT-API image: %w", err)
	}

	// Deploy manifest with mock LLM
	manifest := `
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
        image: localhost/kubernaut-holmesgpt-api:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
        env:
        - name: LLM_PROVIDER
          value: mock
        - name: MOCK_LLM_ENABLED
          value: "true"
        - name: DATASTORAGE_URL
          value: http://datastorage:8080
        - name: LOG_LEVEL
          value: INFO
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
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func deployAIAnalysisController(clusterName, kubeconfigPath string, writer io.Writer) error {
	// Get project root for build context
	projectRoot := getProjectRoot()

	// Build AIAnalysis controller image
	fmt.Fprintln(writer, "  Building AIAnalysis controller image...")
	// Try podman build first (macOS)
	buildCmd := exec.Command("podman", "build", "-t", "kubernaut-aianalysis:latest",
		"-f", "docker/aianalysis.Dockerfile", ".")
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build AIAnalysis controller with podman: %w", err)
	}

	// Load into Kind
	fmt.Fprintln(writer, "  Loading AIAnalysis image into Kind...")
	if err := loadImageToKind(clusterName, "kubernaut-aianalysis:latest", writer); err != nil {
		return fmt.Errorf("failed to load AIAnalysis image: %w", err)
	}

	// Deploy controller with RBAC
	manifest := `
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
- apiGroups: ["aianalysis.kubernaut.ai"]
  resources: ["aianalyses", "aianalyses/status"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
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
      - name: controller
        image: localhost/kubernaut-aianalysis:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 8081
          name: health
        - containerPort: 9090
          name: metrics
        env:
        - name: HOLMESGPT_API_URL
          value: http://holmesgpt-api:8080
        - name: DATASTORAGE_URL
          value: http://datastorage:8080
        - name: REGO_POLICY_PATH
          value: /etc/rego/approval.rego
        volumeMounts:
        - name: rego-policies
          mountPath: /etc/rego
      volumes:
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
  - name: health
    port: 8081
    targetPort: 8081
    nodePort: 30284
  - name: metrics
    port: 9090
    targetPort: 9090
    nodePort: 30184
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return err
	}

	// Deploy Rego policy ConfigMap
	return deployRegoPolicyConfigMap(kubeconfigPath, writer)
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
	cmd.Wait()
	pipeWriter2.Close()
	return applyCmd.Wait()
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

// loadImageToKind loads a container image into the Kind cluster
// Handles both Podman (localhost/ prefix) and Docker image naming
func loadImageToKind(clusterName, imageName string, writer io.Writer) error {
	// For Podman on macOS, kind load docker-image doesn't work well
	// Use podman save + kind load image-archive as workaround

	// Create temp file for image archive
	tmpFile, err := os.CreateTemp("", "kind-image-*.tar")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Save image with podman
	fmt.Fprintf(writer, "  Exporting image %s...\n", imageName)
	saveCmd := exec.Command("podman", "save", "-o", tmpFile.Name(), "localhost/"+imageName)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image %s with podman: %w", imageName, err)
	}

	// Load image archive into Kind
	fmt.Fprintf(writer, "  Loading image archive into Kind...\n")
	loadCmd := exec.Command("kind", "load", "image-archive", tmpFile.Name(), "--name", clusterName)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image %s: %w", imageName, err)
	}

	return nil
}

// getProjectRoot returns the absolute path to the project root directory
func getProjectRoot() string {
	_, currentFile, _, ok := runtime.Caller(0)
	if ok {
		// Go up from test/infrastructure/ to project root
		return filepath.Dir(filepath.Dir(filepath.Dir(currentFile)))
	}

	// Fallback: try to find go.mod
	candidates := []string{".", "..", "../..", "../../.."}
	for _, dir := range candidates {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			absPath, _ := filepath.Abs(dir)
			return absPath
		}
	}
	return "."
}

func createInlineRegoPolicyConfigMap(kubeconfigPath string, writer io.Writer) error {
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
    default require_approval := false
    default reason := "Auto-approved"
    require_approval if {
        input.environment == "production"
    }
    reason := "Production environment requires approval" if {
        input.environment == "production"
    }
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

// NOTE: findManifest function removed - no longer needed with shared deployment functions

// stringReader creates an io.Reader from a string
type stringReaderImpl struct {
	data string
	pos  int
}

func stringReader(s string) io.Reader {
	return &stringReaderImpl{data: s}
}

func (r *stringReaderImpl) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// HAPI Container Infrastructure (for Integration Tests)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Per NOTICE_INTEGRATION_TEST_INFRASTRUCTURE_OWNERSHIP.md:
// - AIAnalysis integration tests connect to HAPI via HTTP
// - HAPI is started programmatically (not via shared podman-compose)
// - HAPI requires Data Storage HTTP API to be running (owned by DS team)
//
// Port Allocation (per DD-TEST-001):
//   HAPI: Default 18081 (can be overridden for parallel tests)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

const (
	// DefaultHAPIPort is the default port for HAPI in integration tests
	DefaultHAPIPort = 18081

	// HAPIImageName is the image name for HAPI container
	HAPIImageName = "kubernaut-holmesgpt-api:test"
)

// AIAnalysis Integration Test Ports (per DD-TEST-001)
const (
	// PostgreSQL port for AIAnalysis integration tests
	AIAnalysisIntegrationPostgresPort = 15434

	// Redis port for AIAnalysis integration tests
	AIAnalysisIntegrationRedisPort = 16380

	// DataStorage API port for AIAnalysis integration tests
	AIAnalysisIntegrationDataStoragePort = 18091

	// HolmesGPT API port for AIAnalysis integration tests
	AIAnalysisIntegrationHAPIPort = 18120

	// Compose project name for AIAnalysis integration tests
	AIAnalysisIntegrationComposeProject = "aianalysis-integration"

	// Compose file path relative to project root
	AIAnalysisIntegrationComposeFile = "test/integration/aianalysis/podman-compose.yml"
)

// HAPIContainerConfig holds configuration for starting HAPI container
type HAPIContainerConfig struct {
	// ContainerName is the unique name for the container
	ContainerName string
	// Port is the host port to expose HAPI on (container always uses 8080)
	Port int
	// DataStorageURL is the URL of the Data Storage service
	DataStorageURL string
	// BuildImage if true, builds the HAPI image before starting
	BuildImage bool
}

// StartHAPIContainer starts a HAPI container for integration testing
//
// This starts HolmesGPT-API with MOCK_LLM_MODE=true for deterministic testing
// without incurring LLM costs.
//
// Prerequisites:
// - Data Storage must be running and accessible at DataStorageURL
// - Podman machine must be running
//
// Parameters:
// - config: Container configuration
// - writer: io.Writer for progress output
//
// Returns:
// - int: The actual port HAPI is running on
// - error: Any errors during container creation
func StartHAPIContainer(config HAPIContainerConfig, writer io.Writer) (int, error) {
	if config.ContainerName == "" {
		config.ContainerName = "aianalysis-hapi-integration"
	}
	if config.Port == 0 {
		config.Port = DefaultHAPIPort
	}
	if config.DataStorageURL == "" {
		config.DataStorageURL = "http://host.containers.internal:18090"
	}

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Starting HAPI Container for Integration Tests\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  Container: %s\n", config.ContainerName)
	fmt.Fprintf(writer, "  Port: %d\n", config.Port)
	fmt.Fprintf(writer, "  Data Storage: %s\n", config.DataStorageURL)
	fmt.Fprintf(writer, "  Mock LLM: ENABLED\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Check if port is available
	if !isHAPIPortAvailable(config.Port) {
		return 0, fmt.Errorf("port %d is already in use", config.Port)
	}

	// Check if container already exists and is running
	checkCmd := exec.Command("podman", "ps", "--filter", fmt.Sprintf("name=%s", config.ContainerName), "--format", "{{.Names}}")
	output, _ := checkCmd.CombinedOutput()
	if strings.TrimSpace(string(output)) == config.ContainerName {
		fmt.Fprintf(writer, "✅ HAPI container '%s' already running on port %d\n", config.ContainerName, config.Port)
		return config.Port, nil
	}

	// Check if container exists but stopped
	checkStoppedCmd := exec.Command("podman", "ps", "-a", "--filter", fmt.Sprintf("name=%s", config.ContainerName), "--format", "{{.Names}}")
	stoppedOutput, _ := checkStoppedCmd.CombinedOutput()
	if strings.TrimSpace(string(stoppedOutput)) == config.ContainerName {
		// Container exists but stopped, remove it
		fmt.Fprintf(writer, "🗑️  Removing stopped HAPI container '%s'...\n", config.ContainerName)
		rmCmd := exec.Command("podman", "rm", config.ContainerName)
		_ = rmCmd.Run()
	}

	// Build image if requested
	if config.BuildImage {
		fmt.Fprintf(writer, "🔨 Building HAPI image...\n")
		if err := buildHAPIImage(writer); err != nil {
			return 0, fmt.Errorf("failed to build HAPI image: %w", err)
		}
	}

	// Start container with mock LLM mode
	fmt.Fprintf(writer, "🚀 Starting HAPI container...\n")
	cmd := exec.Command("podman", "run", "-d",
		"--name", config.ContainerName,
		"-p", fmt.Sprintf("%d:8080", config.Port),
		"-e", "MOCK_LLM_MODE=true",
		"-e", fmt.Sprintf("DATASTORAGE_URL=%s", config.DataStorageURL),
		"-e", "LOG_LEVEL=INFO",
		"--add-host", "host.containers.internal:host-gateway",
		HAPIImageName,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("failed to start HAPI container: %w, output: %s", err, string(output))
	}

	// Wait for HAPI to be ready
	fmt.Fprintf(writer, "⏳ Waiting for HAPI to be ready...\n")
	if err := waitForHAPIHealth(config.Port, 60*time.Second); err != nil {
		// Get container logs for debugging
		logsCmd := exec.Command("podman", "logs", "--tail", "50", config.ContainerName)
		logs, _ := logsCmd.CombinedOutput()
		return 0, fmt.Errorf("HAPI not ready: %w\nContainer logs:\n%s", err, string(logs))
	}

	fmt.Fprintf(writer, "✅ HAPI container '%s' started on port %d\n", config.ContainerName, config.Port)
	return config.Port, nil
}

// StopHAPIContainer stops and removes a HAPI container
func StopHAPIContainer(containerName string, writer io.Writer) error {
	if containerName == "" {
		containerName = "aianalysis-hapi-integration"
	}

	fmt.Fprintf(writer, "🛑 Stopping HAPI container '%s'...\n", containerName)

	// Check if container exists
	checkCmd := exec.Command("podman", "ps", "-a", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, _ := checkCmd.CombinedOutput()
	if strings.TrimSpace(string(output)) != containerName {
		fmt.Fprintf(writer, "✅ HAPI container '%s' does not exist (already cleaned up)\n", containerName)
		return nil
	}

	// Stop container
	stopCmd := exec.Command("podman", "stop", containerName)
	if err := stopCmd.Run(); err != nil {
		fmt.Fprintf(writer, "⚠️  Warning: Failed to stop HAPI container '%s': %v\n", containerName, err)
	}

	// Remove container
	rmCmd := exec.Command("podman", "rm", containerName)
	if err := rmCmd.Run(); err != nil {
		return fmt.Errorf("failed to remove HAPI container: %w", err)
	}

	fmt.Fprintf(writer, "✅ HAPI container '%s' stopped and removed\n", containerName)
	return nil
}

// buildHAPIImage builds the HAPI Docker image
func buildHAPIImage(writer io.Writer) error {
	projectRoot := getProjectRoot()

	cmd := exec.Command("podman", "build",
		"-t", HAPIImageName,
		"-f", "holmesgpt-api/Dockerfile",
		".",
	)
	cmd.Dir = projectRoot
	cmd.Stdout = writer
	cmd.Stderr = writer

	return cmd.Run()
}

// waitForHAPIHealth waits for HAPI health endpoint to respond
func waitForHAPIHealth(port int, timeout time.Duration) error {
	healthURL := fmt.Sprintf("http://localhost:%d/health", port)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := http.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("timeout waiting for HAPI health at %s", healthURL)
}

// isHAPIPortAvailable checks if a TCP port is available for binding
func isHAPIPortAvailable(port int) bool {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// AIAnalysis Integration Test Infrastructure (Podman Compose)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// StartAIAnalysisIntegrationInfrastructure starts the full podman-compose stack for AIAnalysis integration tests
// This includes: PostgreSQL, Redis, DataStorage API, and HolmesGPT API
func StartAIAnalysisIntegrationInfrastructure(writer io.Writer) error {
	projectRoot := getProjectRoot()
	composeFile := filepath.Join(projectRoot, AIAnalysisIntegrationComposeFile)

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Starting AIAnalysis Integration Test Infrastructure\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "  PostgreSQL:     localhost:%d\n", AIAnalysisIntegrationPostgresPort)
	fmt.Fprintf(writer, "  Redis:          localhost:%d\n", AIAnalysisIntegrationRedisPort)
	fmt.Fprintf(writer, "  DataStorage:    http://localhost:%d\n", AIAnalysisIntegrationDataStoragePort)
	fmt.Fprintf(writer, "  HolmesGPT API:  http://localhost:%d\n", AIAnalysisIntegrationHAPIPort)
	fmt.Fprintf(writer, "  Compose File:   %s\n", AIAnalysisIntegrationComposeFile)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Check if podman-compose is available
	if err := exec.Command("podman-compose", "--version").Run(); err != nil {
		return fmt.Errorf("podman-compose not found: %w (install via: pip install podman-compose)", err)
	}

	// Start services
	cmd := exec.Command("podman-compose",
		"-f", composeFile,
		"-p", AIAnalysisIntegrationComposeProject,
		"up", "-d", "--build",
	)
	cmd.Dir = projectRoot
	cmd.Stdout = writer
	cmd.Stderr = writer

	fmt.Fprintf(writer, "⏳ Starting containers...\n")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start podman-compose stack: %w", err)
	}

	// Wait for services to be healthy
	fmt.Fprintf(writer, "⏳ Waiting for services to be healthy...\n")

	// Wait for DataStorage
	if err := waitForHTTPHealth(
		fmt.Sprintf("http://localhost:%d/health", AIAnalysisIntegrationDataStoragePort),
		60*time.Second,
	); err != nil {
		return fmt.Errorf("DataStorage failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "✅ DataStorage is healthy\n")

	// Wait for HolmesGPT API
	if err := waitForHTTPHealth(
		fmt.Sprintf("http://localhost:%d/health", AIAnalysisIntegrationHAPIPort),
		60*time.Second,
	); err != nil {
		return fmt.Errorf("HolmesGPT API failed to become healthy: %w", err)
	}
	fmt.Fprintf(writer, "✅ HolmesGPT API is healthy\n")

	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "✅ AIAnalysis Integration Infrastructure Ready\n")
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	return nil
}

// StopAIAnalysisIntegrationInfrastructure stops and cleans up the AIAnalysis integration test infrastructure
func StopAIAnalysisIntegrationInfrastructure(writer io.Writer) error {
	projectRoot := getProjectRoot()
	composeFile := filepath.Join(projectRoot, AIAnalysisIntegrationComposeFile)

	fmt.Fprintf(writer, "🛑 Stopping AIAnalysis Integration Infrastructure...\n")

	// Stop and remove containers
	cmd := exec.Command("podman-compose",
		"-f", composeFile,
		"-p", AIAnalysisIntegrationComposeProject,
		"down", "-v",
	)
	cmd.Dir = projectRoot
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(writer, "⚠️  Warning: Error stopping infrastructure: %v\n", err)
		return err
	}

	fmt.Fprintf(writer, "✅ AIAnalysis Integration Infrastructure stopped and cleaned up\n")
	return nil
}

// waitForAIAnalysisInfraReady waits for PostgreSQL and Redis to be ready
// This ensures Data Storage can successfully connect when it starts
// Pattern adapted from waitForDataStorageServicesReady in datastorage.go
func waitForAIAnalysisInfraReady(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	// Build Kubernetes clientset
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create clientset: %w", err)
	}

	// Wait for PostgreSQL pod to be ready
	fmt.Fprintf(writer, "   ⏳ Waiting for PostgreSQL pod to be ready...\n")
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
	}, 3*time.Minute, 5*time.Second).Should(BeTrue(), "PostgreSQL pod should become ready")
	fmt.Fprintf(writer, "   ✅ PostgreSQL ready\n")

	// Wait for Redis pod to be ready
	fmt.Fprintf(writer, "   ⏳ Waiting for Redis pod to be ready...\n")
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
	}, 2*time.Minute, 5*time.Second).Should(BeTrue(), "Redis pod should become ready")
	fmt.Fprintf(writer, "   ✅ Redis ready\n")

	return nil
}
