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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
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

	// 1. Create Kind cluster with AIAnalysis config
	fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createAIAnalysisKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Install AIAnalysis CRD
	fmt.Fprintln(writer, "📋 Installing AIAnalysis CRD...")
	if err := installAIAnalysisCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install AIAnalysis CRD: %w", err)
	}

	// 3. Deploy PostgreSQL
	fmt.Fprintln(writer, "🐘 Deploying PostgreSQL...")
	if err := deployPostgreSQL(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 4. Deploy Redis
	fmt.Fprintln(writer, "🔴 Deploying Redis...")
	if err := deployRedis(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 5. Build and deploy Data Storage
	fmt.Fprintln(writer, "💾 Building and deploying Data Storage...")
	if err := deployDataStorage(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Data Storage: %w", err)
	}

	// 6. Build and deploy HolmesGPT-API
	fmt.Fprintln(writer, "🤖 Building and deploying HolmesGPT-API...")
	if err := deployHolmesGPTAPI(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy HolmesGPT-API: %w", err)
	}

	// 7. Build and deploy AIAnalysis controller
	fmt.Fprintln(writer, "🧠 Building and deploying AIAnalysis controller...")
	if err := deployAIAnalysisController(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy AIAnalysis controller: %w", err)
	}

	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "✅ AIAnalysis E2E cluster ready!")
	fmt.Fprintln(writer, fmt.Sprintf("  • AIAnalysis API: http://localhost:%d", AIAnalysisHostPort))
	fmt.Fprintln(writer, fmt.Sprintf("  • AIAnalysis Metrics: http://localhost:%d/metrics", 9184))
	fmt.Fprintln(writer, fmt.Sprintf("  • Data Storage: http://localhost:%d", DataStorageHostPort))
	fmt.Fprintln(writer, fmt.Sprintf("  • HolmesGPT-API: http://localhost:%d", HolmesGPTAPIHostPort))
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

	// Check if cluster already exists
	checkCmd := exec.Command("kind", "get", "clusters")
	output, _ := checkCmd.Output()
	if containsCluster(string(output), clusterName) {
		fmt.Fprintln(writer, "  Cluster already exists, reusing...")
		return exportKubeconfig(clusterName, kubeconfigPath, writer)
	}

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
	crdPath := findCRDFile("aianalysis.kubernaut.io_aianalyses.yaml")
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
			"get", "crd", "aianalyses.aianalysis.kubernaut.io")
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

func deployPostgreSQL(kubeconfigPath string, writer io.Writer) error {
	// Create namespace
	createNamespaceCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"create", "namespace", "kubernaut-system", "--dry-run=client", "-o", "yaml")
	applyCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")

	// Use io.Pipe to connect stdout of create to stdin of apply
	pipeReader, pipeWriter := io.Pipe()
	createNamespaceCmd.Stdout = pipeWriter
	applyCmd.Stdin = pipeReader
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := createNamespaceCmd.Start(); err != nil {
		return err
	}
	if err := applyCmd.Start(); err != nil {
		return err
	}
	// Close pipe writer after create command finishes
	go func() {
		createNamespaceCmd.Wait()
		pipeWriter.Close()
	}()
	createNamespaceCmd.Wait()
	applyCmd.Wait()

	// Deploy PostgreSQL using manifest
	manifestPath := findManifest("postgres.yaml", "test/e2e/aianalysis/manifests")
	if manifestPath != "" {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
			"apply", "-f", manifestPath)
		cmd.Stdout = writer
		cmd.Stderr = writer
		return cmd.Run()
	}

	// Fallback: create inline deployment
	fmt.Fprintln(writer, "  Using inline PostgreSQL deployment...")
	return createInlinePostgreSQL(kubeconfigPath, writer)
}

func createInlinePostgreSQL(kubeconfigPath string, writer io.Writer) error {
	manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: postgres
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: quay.io/jordigilh/pgvector:pg16
        ports:
        - containerPort: 5432
        env:
        - name: POSTGRES_DB
          value: action_history
        - name: POSTGRES_USER
          value: slm_user
        - name: POSTGRES_PASSWORD
          value: test_password
---
apiVersion: v1
kind: Service
metadata:
  name: postgres
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: postgres
  ports:
  - port: 5432
    targetPort: 5432
    nodePort: 30433
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func deployRedis(kubeconfigPath string, writer io.Writer) error {
	manifest := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: kubernaut-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
      - name: redis
        image: quay.io/jordigilh/redis:7-alpine
        ports:
        - containerPort: 6379
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: redis
  ports:
  - port: 6379
    targetPort: 6379
    nodePort: 30380
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = stringReader(manifest)
	cmd.Stdout = writer
	cmd.Stderr = writer
	return cmd.Run()
}

func deployDataStorage(clusterName, kubeconfigPath string, writer io.Writer) error {
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
		// Try docker as fallback
		buildCmd = exec.Command("docker", "build", "-t", "kubernaut-datastorage:latest",
			"-f", "docker/data-storage.Dockerfile", ".")
		buildCmd.Dir = projectRoot
		buildCmd.Stdout = writer
		buildCmd.Stderr = writer
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build Data Storage: %w", err)
		}
	}

	// Load into Kind
	fmt.Fprintln(writer, "  Loading Data Storage image into Kind...")
	if err := loadImageToKind(clusterName, "kubernaut-datastorage:latest", writer); err != nil {
		return fmt.Errorf("failed to load image: %w", err)
	}

	// Deploy manifest
	manifest := `
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
        image: kubernaut-datastorage:latest
        imagePullPolicy: Never
        ports:
        - containerPort: 8080
        env:
        - name: POSTGRES_HOST
          value: postgres
        - name: POSTGRES_PORT
          value: "5432"
        - name: POSTGRES_USER
          value: slm_user
        - name: POSTGRES_PASSWORD
          value: test_password
        - name: POSTGRES_DB
          value: action_history
        - name: REDIS_HOST
          value: redis
        - name: REDIS_PORT
          value: "6379"
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
		// Try docker as fallback
		buildCmd = exec.Command("docker", "build", "-t", "kubernaut-holmesgpt-api:latest",
			"-f", "holmesgpt-api/Dockerfile", ".")
		buildCmd.Dir = projectRoot
		buildCmd.Stdout = writer
		buildCmd.Stderr = writer
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build HolmesGPT-API: %w", err)
		}
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
        image: kubernaut-holmesgpt-api:latest
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
		// Try docker as fallback
		buildCmd = exec.Command("docker", "build", "-t", "kubernaut-aianalysis:latest",
			"-f", "docker/aianalysis.Dockerfile", ".")
		buildCmd.Dir = projectRoot
		buildCmd.Stdout = writer
		buildCmd.Stderr = writer
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build AIAnalysis controller: %w", err)
		}
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
- apiGroups: ["aianalysis.kubernaut.io"]
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
        image: kubernaut-aianalysis:latest
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

	// Try Podman first (macOS)
	fmt.Fprintf(writer, "  Exporting image %s...\n", imageName)
	saveCmd := exec.Command("podman", "save", "-o", tmpFile.Name(), "localhost/"+imageName)
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer
	if err := saveCmd.Run(); err != nil {
		// Try Docker as fallback
		saveCmd = exec.Command("docker", "save", "-o", tmpFile.Name(), imageName)
		saveCmd.Stdout = writer
		saveCmd.Stderr = writer
		if err := saveCmd.Run(); err != nil {
			return fmt.Errorf("failed to save image %s: %w", imageName, err)
		}
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

func findManifest(name, dir string) string {
	candidates := []string{
		filepath.Join(dir, name),
		filepath.Join("../", dir, name),
		filepath.Join("../../", dir, name),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			absPath, _ := filepath.Abs(path)
			return absPath
		}
	}
	return ""
}

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
