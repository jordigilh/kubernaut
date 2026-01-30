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
	"strings"
	"time"
)

// CreateWorkflowExecutionClusterParallel creates a Kind cluster for WorkflowExecution E2E tests
// with parallel infrastructure setup to reduce total setup time.
//
// Phase 2 E2E Stabilization: Parallel Infrastructure Optimization
// Reference: docs/handoff/WE_E2E_INFRASTRUCTURE_STABILIZATION_PLAN.md (Phase 2)
// Pattern: test/infrastructure/signalprocessing.go:246 (SetupSignalProcessingInfrastructureParallel)
//
// Parallel Execution Strategy:
//
//	Phase 1 (Sequential): Create Kind cluster (~1 min)
//	Phase 2 (PARALLEL):   Tekton install | PostgreSQL+Redis | Build DS image (~5 min → ~4 min)
//	Phase 3 (Sequential): Deploy DS + migrations (~2 min)
//	Phase 4 (Sequential): Namespace + pull secrets (~30s)
//
// Total time: ~7.5 minutes (vs ~9 minutes sequential)
// Savings: ~1.5 minutes (15-20% faster)
func CreateWorkflowExecutionClusterParallel(clusterName, kubeconfigPath string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "🚀 WorkflowExecution E2E Cluster (PARALLEL MODE)\n")
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "  Parallel optimization: ~1.5 min saved per E2E run (15-20%% faster)\n")
	_, _ = fmt.Fprintf(output, "  Reference: SignalProcessing parallel infrastructure pattern\n")
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "  Cluster: %s\n", clusterName)
	_, _ = fmt.Fprintf(output, "  Kubeconfig: %s\n", kubeconfigPath)
	_, _ = fmt.Fprintf(output, "  Tekton Version: %s\n", TektonPipelinesVersion)
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Find config file
	configPath, err := findKindConfig("kind-workflowexecution-config.yaml")
	if err != nil {
		return fmt.Errorf("failed to find Kind config: %w", err)
	}
	_, _ = fmt.Fprintf(output, "Using Kind config: %s\n", configPath)

	// DD-TEST-007: Create coverdata directory before Kind cluster creation
	// Kind config references ./coverdata as extraMount, must exist before mount
	if os.Getenv("E2E_COVERAGE") == "true" {
		_, _ = fmt.Fprintf(output, "\n📊 DD-TEST-007: Creating coverdata directory for E2E coverage...\n")
		projectRoot, err := findProjectRoot()
		if err != nil {
			return fmt.Errorf("failed to find project root: %w", err)
		}
		coverDataPath := filepath.Join(projectRoot, "test/e2e/workflowexecution/coverdata")
		if err := os.MkdirAll(coverDataPath, 0755); err != nil {
			return fmt.Errorf("failed to create coverdata directory: %w", err)
		}
		_, _ = fmt.Fprintf(output, "   ✅ Created %s\n", coverDataPath)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 1: Create Kind cluster (Sequential - must be first)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(output, "\n📦 PHASE 1: Creating Kind cluster...\n")

	createCmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", configPath,
		"--kubeconfig", kubeconfigPath,
	)
	createCmd.Stdout = output
	createCmd.Stderr = output
	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}
	_, _ = fmt.Fprintf(output, "✅ Kind cluster created\n")

	// Create kubernaut-system namespace (required by PostgreSQL deployment in Phase 2)
	_, _ = fmt.Fprintf(output, "\n📁 Creating controller namespace %s...\n", WorkflowExecutionNamespace)
	nsCmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath,
		"create", "namespace", WorkflowExecutionNamespace)
	if err := nsCmd.Run(); err != nil {
		// Ignore if already exists
		_, _ = fmt.Fprintf(output, "  ⚠️  Namespace creation skipped (may already exist)\n")
	} else {
		_, _ = fmt.Fprintf(output, "✅ Namespace %s created\n", WorkflowExecutionNamespace)
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 2: Parallel infrastructure setup
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(output, "\n⚡ PHASE 2: Parallel infrastructure setup...\n")
	_, _ = fmt.Fprintf(output, "  ├── Installing Tekton Pipelines\n")
	_, _ = fmt.Fprintf(output, "  ├── Deploying PostgreSQL + Redis\n")
	_, _ = fmt.Fprintf(output, "  └── Building Data Storage image\n")

	ctx := context.Background()

	type result struct {
		name string
		err  error
	}

	results := make(chan result, 3)

	// Goroutine 1: Install Tekton Pipelines
	go func() {
		_, _ = fmt.Fprintf(output, "\n🔧 [Goroutine 1] Installing Tekton Pipelines %s...\n", TektonPipelinesVersion)
		err := installTektonPipelines(kubeconfigPath, output)
		if err != nil {
			err = fmt.Errorf("Tekton installation failed: %w", err)
		} else {
			_, _ = fmt.Fprintf(output, "✅ [Goroutine 1] Tekton Pipelines installed\n")
		}
		results <- result{name: "Tekton Pipelines", err: err}
	}()

	// Goroutine 2: Deploy PostgreSQL and Redis
	go func() {
		_, _ = fmt.Fprintf(output, "\n🗄️  [Goroutine 2] Deploying PostgreSQL + Redis...\n")
		var err error

		// Deploy PostgreSQL
		_, _ = fmt.Fprintf(output, "  🐘 [Goroutine 2] Deploying PostgreSQL...\n")
		if postgresErr := deployPostgreSQLInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, output); postgresErr != nil {
			err = fmt.Errorf("PostgreSQL deployment failed: %w", postgresErr)
			results <- result{name: "PostgreSQL+Redis", err: err}
			return
		}

		// Deploy Redis
		_, _ = fmt.Fprintf(output, "  🔴 [Goroutine 2] Deploying Redis...\n")
		if redisErr := deployRedisInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, output); redisErr != nil {
			err = fmt.Errorf("Redis deployment failed: %w", redisErr)
			results <- result{name: "PostgreSQL+Redis", err: err}
			return
		}

		// Wait for both to be ready
		_, _ = fmt.Fprintf(output, "  ⏳ [Goroutine 2] Waiting for PostgreSQL to be ready...\n")
		if waitErr := waitForDeploymentReady(kubeconfigPath, "postgresql", output); waitErr != nil {
			err = fmt.Errorf("PostgreSQL not ready: %w", waitErr)
			results <- result{name: "PostgreSQL+Redis", err: err}
			return
		}

		_, _ = fmt.Fprintf(output, "  ⏳ [Goroutine 2] Waiting for Redis to be ready...\n")
		if waitErr := waitForDeploymentReady(kubeconfigPath, "redis", output); waitErr != nil {
			err = fmt.Errorf("Redis not ready: %w", waitErr)
			results <- result{name: "PostgreSQL+Redis", err: err}
			return
		}

		_, _ = fmt.Fprintf(output, "✅ [Goroutine 2] PostgreSQL + Redis ready\n")
		results <- result{name: "PostgreSQL+Redis", err: nil}
	}()

	// Goroutine 3: Pre-build Data Storage image (can happen while other infrastructure deploys)
	go func() {
		_, _ = fmt.Fprintf(output, "\n💾 [Goroutine 3] Building Data Storage image...\n")
		err := buildDataStorageImage(output)
		if err != nil {
			err = fmt.Errorf("Data Storage image build failed: %w", err)
		} else {
			_, _ = fmt.Fprintf(output, "✅ [Goroutine 3] Data Storage image built\n")
		}
		results <- result{name: "DS image build", err: err}
	}()

	// Collect results from all 3 goroutines
	_, _ = fmt.Fprintf(output, "\n⏳ Waiting for parallel tasks to complete...\n")
	var errors []error
	for i := 0; i < 3; i++ {
		res := <-results
		if res.err != nil {
			_, _ = fmt.Fprintf(output, "❌ %s: %v\n", res.name, res.err)
			errors = append(errors, res.err)
		} else {
			_, _ = fmt.Fprintf(output, "✅ %s completed\n", res.name)
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("parallel setup failed with %d errors: %v", len(errors), errors)
	}

	_, _ = fmt.Fprintf(output, "✅ All parallel tasks completed successfully\n")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 3: Deploy Data Storage + migrations (Sequential - requires Phase 2)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(output, "\n💾 PHASE 3: Deploying Data Storage + migrations...\n")

	// Deploy Data Storage (PostgreSQL/Redis already ready from Phase 2)
	_, _ = fmt.Fprintf(output, "  💾 Deploying Data Storage service...\n")
	if err := deployDataStorageWithConfig(clusterName, kubeconfigPath, output); err != nil {
		return fmt.Errorf("failed to deploy Data Storage: %w", err)
	}

	// Wait for DS to be ready
	_, _ = fmt.Fprintf(output, "  ⏳ Waiting for Data Storage to be ready...\n")
	if err := waitForWEDataStorageReady(kubeconfigPath, output); err != nil {
		return fmt.Errorf("Data Storage did not become ready: %w", err)
	}
	_, _ = fmt.Fprintf(output, "✅ Data Storage deployed and ready\n")

	// Apply ALL migrations (audit + workflow catalog + all schema updates)
	// Using ApplyAllMigrations ensures we get the complete schema including:
	// - 015: Create workflow catalog table
	// - 017-019: Workflow schema updates (UUID primary key, workflow_name, etc.)
	// - 020-022: Label columns and status fields
	// - 013-014: Audit events table + partitions
	_, _ = fmt.Fprintf(output, "\n📋 Applying ALL migrations (auto-discovered)...\n")
	if err := ApplyAllMigrations(context.Background(), WorkflowExecutionNamespace, kubeconfigPath, output); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Verify critical tables exist
	_, _ = fmt.Fprintf(output, "\n🔍 Verifying critical tables...\n")
	verifyConfig := DefaultMigrationConfig(WorkflowExecutionNamespace, kubeconfigPath)
	verifyConfig.PostgresService = "postgresql"
	verifyConfig.Tables = []string{"audit_events", "remediation_workflow_catalog"}
	if err := VerifyMigrations(context.Background(), verifyConfig, output); err != nil {
		return fmt.Errorf("migration verification failed: %w", err)
	}
	_, _ = fmt.Fprintf(output, "✅ All migrations applied and verified\n")

	// Build and register test workflow bundles
	// This creates OCI bundles for test workflows and registers them in DataStorage
	// Per DD-WORKFLOW-005 v1.0: OpenAPI client workflow registration
	_, _ = fmt.Fprintf(output, "\n🎯 Building and registering test workflow bundles...\n")

	// DD-AUTH-014: Create ServiceAccount for workflow registration with DataStorage
	workflowRegSAName := "workflow-registration-sa"
	_, _ = fmt.Fprintf(output, "🔐 Creating ServiceAccount for workflow registration (DD-AUTH-014)...\n")
	if err := CreateE2EServiceAccountWithDataStorageAccess(ctx, WorkflowExecutionNamespace, kubeconfigPath, workflowRegSAName, output); err != nil {
		return fmt.Errorf("failed to create workflow registration ServiceAccount: %w", err)
	}

	// Get ServiceAccount token for authenticated workflow registration
	saToken, err := GetServiceAccountToken(ctx, WorkflowExecutionNamespace, workflowRegSAName, kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to get ServiceAccount token: %w", err)
	}
	_, _ = fmt.Fprintf(output, "✅ ServiceAccount token retrieved for authenticated workflow registration\n")

	dataStorageURL := "http://localhost:8092" // DD-TEST-001: WE → DataStorage dependency port
	if _, err = BuildAndRegisterTestWorkflows(clusterName, kubeconfigPath, dataStorageURL, saToken, output); err != nil {
		return fmt.Errorf("failed to build and register test workflows: %w", err)
	}
	_, _ = fmt.Fprintf(output, "✅ Test workflows ready\n")

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Namespace + pull secrets (Sequential - quick final setup)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(output, "\n📁 PHASE 4: Final setup (namespace + pull secrets)...\n")

	// Create execution namespace
	_, _ = fmt.Fprintf(output, "  📁 Creating execution namespace %s...\n", ExecutionNamespace)
	execNsCmd := exec.Command("kubectl", "create", "namespace", ExecutionNamespace,
		"--kubeconfig", kubeconfigPath)
	execNsCmd.Stdout = output
	execNsCmd.Stderr = output
	if err := execNsCmd.Run(); err != nil {
		// Namespace may already exist
		_, _ = fmt.Fprintf(output, "  Note: namespace creation returned error (may already exist): %v\n", err)
	}

	// Create image pull secret
	if err := createQuayPullSecret(kubeconfigPath, ExecutionNamespace, output); err != nil {
		_, _ = fmt.Fprintf(output, "  Warning: Could not create quay.io pull secret: %v\n", err)
		// Non-fatal - repos may be public
	}

	_, _ = fmt.Fprintf(output, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "✅ WorkflowExecution E2E cluster ready (PARALLEL MODE)!\n")
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}
func deployDataStorageWithConfig(clusterName, kubeconfigPath string, output io.Writer) error {
	projectRoot := getProjectRoot()

	// Build Data Storage image
	_, _ = fmt.Fprintln(output, "    Building Data Storage image...")
	buildCmd := exec.Command("podman", "build", "-t", "kubernaut-datastorage:latest",
		"-f", "docker/data-storage.Dockerfile", ".")
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = output
	buildCmd.Stderr = output
	if err := buildCmd.Run(); err != nil {
		// Try docker as fallback
		buildCmd = exec.Command("docker", "build", "-t", "kubernaut-datastorage:latest",
			"-f", "docker/data-storage.Dockerfile", ".")
		buildCmd.Dir = projectRoot
		buildCmd.Stdout = output
		buildCmd.Stderr = output
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build Data Storage: %w", err)
		}
	}

	// Load into Kind
	_, _ = fmt.Fprintln(output, "    Loading Data Storage image into Kind...")
	if err := loadImageToKind(clusterName, "kubernaut-datastorage:latest", output); err != nil {
		return fmt.Errorf("failed to load image: %w", err)
	}

	// Deploy ConfigMap with ADR-030 configuration
	configMapManifest := `
apiVersion: v1
kind: ConfigMap
metadata:
  name: datastorage-config
  namespace: kubernaut-system
data:
  config.yaml: |
    server:
      port: 8080
      metricsPort: 9090
      readTimeout: 30s
      writeTimeout: 30s
      idleTimeout: 120s
      gracefulShutdownTimeout: 30s
    database:
      host: postgresql
      port: 5432
      name: action_history
      ssl_mode: disable
      max_open_conns: 25
      max_idle_conns: 5
      conn_max_lifetime: 5m
      # ADR-030 Section 6: Secrets from file
      secretsFile: /etc/datastorage/secrets/db-credentials.yaml
      usernameKey: username
      passwordKey: password
    redis:
      addr: redis:6379
      db: 0
      dlq_stream_name: audit_dlq
      dlq_max_len: 10000
      dlq_consumer_group: audit_processors
      # ADR-030 Section 6: Secrets from file
      secretsFile: /etc/datastorage/secrets/redis-credentials.yaml
      passwordKey: password
    logging:
      level: info
      format: json
`
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(configMapManifest)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	// Deploy Secret with credentials in YAML format (ADR-030 Section 6)
	secretManifest := `
apiVersion: v1
kind: Secret
metadata:
  name: datastorage-secrets
  namespace: kubernaut-system
stringData:
  db-credentials.yaml: |
    username: slm_user
    password: test_password
  redis-credentials.yaml: |
    password: ""
`
	cmd = exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(secretManifest)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Secret: %w", err)
	}

	// Deploy Data Storage with proper volumes and CONFIG_PATH
	// Note: Image name is localhost/kubernaut-datastorage:latest when loaded via kind load
	deploymentManifest := `
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
          name: http
        - containerPort: 9090
          name: metrics
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
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: secrets
        secret:
          secretName: datastorage-secrets
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
    name: http
  - port: 9090
    targetPort: 9090
    name: metrics
---
apiVersion: v1
kind: Service
metadata:
  name: datastorage-service
  namespace: kubernaut-system
spec:
  type: ClusterIP
  selector:
    app: datastorage
  ports:
  - port: 8080
    targetPort: 8080
    name: http
`
	cmd = exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(deploymentManifest)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

func waitForWEDataStorageReady(kubeconfigPath string, output io.Writer) error {
	if err := waitForDeploymentReady(kubeconfigPath, "datastorage", output); err != nil {
		return err
	}
	// Brief wait for DS to initialize connections to PostgreSQL/Redis
	time.Sleep(5 * time.Second)
	return nil
}
