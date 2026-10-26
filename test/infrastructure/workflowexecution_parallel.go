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

// CreateWorkflowExecutionClusterParallel creates a Kind cluster for WorkflowExecution E2E tests
// with parallel infrastructure setup to reduce total setup time.
//
// Phase 2 E2E Stabilization: Parallel Infrastructure Optimization
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
	if os.Getenv("E2E_COVERAGE") == trueFixture {
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

	// Issue #1769: retry on transient container-runtime errors.
	err = retryTransientKindRuntimeError(output, func() (string, error) {
		var captured strings.Builder
		createCmd := exec.CommandContext(context.Background(), "kind", "create", "cluster",
			"--name", clusterName,
			"--config", configPath,
			"--kubeconfig", kubeconfigPath,
		)
		createCmd.Stdout = io.MultiWriter(output, &captured)
		createCmd.Stderr = io.MultiWriter(output, &captured)
		runErr := createCmd.Run()
		return captured.String(), runErr
	})
	if err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}
	_, _ = fmt.Fprintf(output, "✅ Kind cluster created\n")

	// Create kubernaut-system namespace (required by PostgreSQL deployment in Phase 2)
	_, _ = fmt.Fprintf(output, "\n📁 Creating controller namespace %s...\n", WorkflowExecutionNamespace)
	nsCmd := exec.CommandContext(context.Background(), "kubectl", "--kubeconfig", kubeconfigPath,
		"create", "namespace", WorkflowExecutionNamespace)
	if err := nsCmd.Run(); err != nil {
		// Ignore if already exists
		_, _ = fmt.Fprintf(output, "  ⚠️  Namespace creation skipped (may already exist)\n")
	} else {
		_, _ = fmt.Fprintf(output, "✅ Namespace %s created\n", WorkflowExecutionNamespace)
	}

	_, _ = fmt.Fprintf(output, "\n⚡ PHASE 2: Parallel infrastructure setup...\n")
	if err := runWorkflowExecutionParallelSetup(kubeconfigPath, output); err != nil {
		return err
	}
	ctx := context.Background()

	if err := deployWorkflowExecutionDataStorage(ctx, clusterName, kubeconfigPath, output); err != nil {
		return err
	}

	// ═══════════════════════════════════════════════════════════════════════
	// PHASE 4: Namespace + pull secrets (Sequential - quick final setup)
	// ═══════════════════════════════════════════════════════════════════════
	_, _ = fmt.Fprintf(output, "\n📁 PHASE 4: Final setup (namespace + pull secrets)...\n")

	// Create execution namespace
	_, _ = fmt.Fprintf(output, "  📁 Creating execution namespace %s...\n", ExecutionNamespace)
	execNsCmd := exec.CommandContext(context.Background(), "kubectl", "create", "namespace", ExecutionNamespace,
		"--kubeconfig", kubeconfigPath)
	execNsCmd.Stdout = output
	execNsCmd.Stderr = output
	if err := execNsCmd.Run(); err != nil {
		// Namespace may already exist
		_, _ = fmt.Fprintf(output, "  Note: namespace creation returned error (may already exist): %v\n", err)
	}

	// Create image pull secret
	if err := createQuayPullSecret(ctx, kubeconfigPath, ExecutionNamespace, output); err != nil {
		_, _ = fmt.Fprintf(output, "  Warning: Could not create quay.io pull secret: %v\n", err)
		// Non-fatal - repos may be public
	}

	_, _ = fmt.Fprintf(output, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "✅ WorkflowExecution E2E cluster ready (PARALLEL MODE)!\n")
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

func deployWorkflowExecutionDataStorage(ctx context.Context, clusterName, kubeconfigPath string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "\n💾 PHASE 3: Deploying Data Storage + migrations...\n  🔐 Generating inter-service TLS certificates (Issue #785)...\n")
	if _, err := GenerateInterServiceTLS(ctx, kubeconfigPath, WorkflowExecutionNamespace, output); err != nil {
		return fmt.Errorf("failed to generate inter-service TLS: %w", err)
	}
	if err := GenerateSigningCertSecret(ctx, kubeconfigPath, WorkflowExecutionNamespace, output); err != nil {
		return fmt.Errorf("failed to generate signing certificate: %w", err)
	}
	_, _ = fmt.Fprintf(output, "  💾 Deploying Data Storage service...\n")
	if err := deployDataStorageWithConfig(ctx, clusterName, kubeconfigPath, output); err != nil {
		return fmt.Errorf("failed to deploy Data Storage: %w", err)
	}
	_, _ = fmt.Fprintf(output, "  ⏳ Waiting for Data Storage to be ready...\n")
	if err := waitForWEDataStorageReady(ctx, kubeconfigPath, output); err != nil {
		return fmt.Errorf("data storage did not become ready: %w", err)
	}
	_, _ = fmt.Fprintf(output, "✅ Data Storage deployed and ready\n\n📋 Applying ALL migrations (auto-discovered)...\n")
	if err := ApplyAllMigrations(ctx, WorkflowExecutionNamespace, kubeconfigPath, output); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}
	verifyConfig := DefaultMigrationConfig(WorkflowExecutionNamespace, kubeconfigPath)
	verifyConfig.PostgresService = "postgresql"
	verifyConfig.Tables = []string{"audit_events"}
	if err := VerifyMigrations(ctx, verifyConfig, output); err != nil {
		return fmt.Errorf("migration verification failed: %w", err)
	}
	_, _ = fmt.Fprintf(output, "✅ All migrations applied and verified\n\n🎯 Building and registering test workflow bundles...\n")
	workflowRegSAName := "workflow-registration-sa"
	if err := CreateE2EServiceAccountWithDataStorageAccess(ctx, WorkflowExecutionNamespace, kubeconfigPath, workflowRegSAName, output); err != nil {
		return fmt.Errorf("failed to create workflow registration ServiceAccount: %w", err)
	}
	saToken, err := GetServiceAccountToken(ctx, WorkflowExecutionNamespace, workflowRegSAName, kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to get ServiceAccount token: %w", err)
	}
	_, _ = fmt.Fprintf(output, "✅ ServiceAccount token retrieved for authenticated workflow registration\n🔑 Creating DD-WE-006 dependency Secret in %s...\n", ExecutionNamespace)
	depSecretCmd := exec.CommandContext(ctx, "kubectl", "create", "secret", "generic", "e2e-dep-secret", "--from-literal=token=e2e-test-value", "--namespace", ExecutionNamespace, "--kubeconfig", kubeconfigPath)
	depSecretOut, depSecretErr := depSecretCmd.CombinedOutput()
	if depSecretErr != nil && !strings.Contains(string(depSecretOut), "AlreadyExists") {
		_, _ = fmt.Fprintf(output, "⚠️  Failed to create DD-WE-006 dep Secret (non-fatal): %s\n", string(depSecretOut))
	} else {
		_, _ = fmt.Fprintf(output, "   ✅ Secret e2e-dep-secret ready in %s\n", ExecutionNamespace)
	}
	if _, err := BuildAndRegisterTestWorkflows(ctx, clusterName, kubeconfigPath, "https://localhost:8092", saToken, output); err != nil {
		return fmt.Errorf("failed to build and register test workflows: %w", err)
	}
	_, _ = fmt.Fprintf(output, "✅ Test workflows ready\n")
	return nil
}

func runWorkflowExecutionParallelSetup(kubeconfigPath string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "  ├── Installing Tekton Pipelines\n  ├── Deploying PostgreSQL + Redis\n  └── Building Data Storage image\n")
	ctx := context.Background()
	results := make(chan struct {
		name string
		err  error
	}, 3)
	go func() {
		_, _ = fmt.Fprintf(output, "\n🔧 [Goroutine 1] Installing Tekton Pipelines %s...\n", TektonPipelinesVersion)
		err := installTektonPipelines(ctx, kubeconfigPath, output)
		if err != nil {
			err = fmt.Errorf("tekton installation failed: %w", err)
		} else {
			_, _ = fmt.Fprintf(output, "✅ [Goroutine 1] Tekton Pipelines installed\n")
		}
		results <- struct {
			name string
			err  error
		}{"Tekton Pipelines", err}
	}()
	go func() {
		_, _ = fmt.Fprintf(output, "\n🗄️  [Goroutine 2] Deploying PostgreSQL + Redis...\n")
		steps := []struct {
			name string
			fn   func() error
		}{{"postgresql deployment", func() error {
			return deployPostgreSQLInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, output)
		}}, {"redis deployment", func() error { return deployRedisInNamespace(ctx, WorkflowExecutionNamespace, kubeconfigPath, output) }}, {"postgresql readiness", func() error { return waitForDeploymentReady(ctx, kubeconfigPath, "postgresql", output) }}, {"redis readiness", func() error { return waitForDeploymentReady(ctx, kubeconfigPath, "redis", output) }}}
		for _, step := range steps {
			if err := step.fn(); err != nil {
				results <- struct {
					name string
					err  error
				}{"PostgreSQL+Redis", fmt.Errorf("%s failed: %w", step.name, err)}
				return
			}
		}
		_, _ = fmt.Fprintf(output, "✅ [Goroutine 2] PostgreSQL + Redis ready\n")
		results <- struct {
			name string
			err  error
		}{"PostgreSQL+Redis", nil}
	}()
	go func() {
		_, _ = fmt.Fprintf(output, "\n💾 [Goroutine 3] Building Data Storage image...\n")
		err := buildDataStorageImage(ctx, output)
		if err != nil {
			err = fmt.Errorf("data storage image build failed: %w", err)
		} else {
			_, _ = fmt.Fprintf(output, "✅ [Goroutine 3] Data Storage image built\n")
		}
		results <- struct {
			name string
			err  error
		}{"DS image build", err}
	}()
	_, _ = fmt.Fprintf(output, "\n⏳ Waiting for parallel tasks to complete...\n")
	var errs []error
	for range 3 {
		result := <-results
		if result.err != nil {
			_, _ = fmt.Fprintf(output, "❌ %s: %v\n", result.name, result.err)
			errs = append(errs, result.err)
		} else {
			_, _ = fmt.Fprintf(output, "✅ %s completed\n", result.name)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("parallel setup failed with %d errors: %v", len(errs), errs)
	}
	_, _ = fmt.Fprintf(output, "✅ All parallel tasks completed successfully\n")
	return nil
}
func deployDataStorageWithConfig(ctx context.Context, clusterName, kubeconfigPath string, output io.Writer) error {
	projectRoot := getProjectRoot()

	// Build Data Storage image
	_, _ = fmt.Fprintln(output, "    Building Data Storage image...")
	buildCmd := exec.CommandContext(ctx, "podman", "build",
		"--build-arg", fmt.Sprintf("GOARCH=%s", runtime.GOARCH),
		"-t", "kubernaut-datastorage:latest",
		"-f", "docker/data-storage.Dockerfile", ".")
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = output
	buildCmd.Stderr = output
	if err := buildCmd.Run(); err != nil {
		// Try docker as fallback
		buildCmd = exec.CommandContext(ctx, "docker", "build",
			"--build-arg", fmt.Sprintf("GOARCH=%s", runtime.GOARCH),
			"-t", "kubernaut-datastorage:latest",
			"-f", "docker/data-storage.Dockerfile", ".")
		buildCmd.Dir = projectRoot
		buildCmd.Stdout = output
		buildCmd.Stderr = output
		if err := buildCmd.Run(); err != nil {
			return fmt.Errorf("failed to build Data Storage: %w", err)
		}
	}

	// Load into Kind (skip when using registry images - IMAGE_REGISTRY + IMAGE_TAG set)
	// Pattern matches buildImageWithArgs() in shared_integration_utils.go
	if os.Getenv("IMAGE_REGISTRY") != "" && os.Getenv("IMAGE_TAG") != "" {
		_, _ = fmt.Fprintln(output, "    Skipping image load (registry mode - Kubernetes pulls from registry)")
		_, _ = fmt.Fprintf(output, "    ℹ️  IMAGE_REGISTRY=%s, IMAGE_TAG=%s\n", os.Getenv("IMAGE_REGISTRY"), os.Getenv("IMAGE_TAG"))
		_, _ = fmt.Fprintf(output, "    📦 Image will be pulled directly by Kubernetes: kubernaut-datastorage:latest\n")
	} else {
		_, _ = fmt.Fprintln(output, "    Loading Data Storage image into Kind...")
		if err := loadImageToKind(ctx, clusterName, "kubernaut-datastorage:latest", output); err != nil {
			return fmt.Errorf("failed to load image: %w", err)
		}
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
      healthPort: 8081
      readTimeout: 30s
      writeTimeout: 30s
      idleTimeout: 120s
      gracefulShutdownTimeout: 30s
      tls:
        certDir: /etc/tls
    database:
      host: postgresql
      port: 5432
      name: action_history
      sslMode: disable
      maxOpenConns: 25
      maxIdleConns: 5
      connMaxLifetime: 5m
      # ADR-030 Section 6: Secrets from file
      secretsFile: /etc/datastorage/secrets/db-secrets.yaml
      usernameKey: username
      passwordKey: password
    redis:
      addr: redis:6379
      db: 0
      dlqStreamName: audit_dlq
      dlqMaxLen: 10000
      dlqConsumerGroup: audit_processors
      # ADR-030 Section 6: Secrets from file
      secretsFile: /etc/datastorage/secrets/redis-secrets.yaml
      passwordKey: password
    logging:
      level: info
      format: json
`
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(configMapManifest)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	// Deploy Secrets with credentials in YAML format (ADR-030 Section 6)
	secretManifest := `
apiVersion: v1
kind: Secret
metadata:
  name: postgresql-secret
  namespace: kubernaut-system
stringData:
  db-secrets.yaml: |
    username: slm_user
    password: test_password
---
apiVersion: v1
kind: Secret
metadata:
  name: redis-secret
  namespace: kubernaut-system
stringData:
  redis-secrets.yaml: |
    password: ""
`
	cmd = exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(secretManifest)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create Secrets: %w", err)
	}

	// Deploy Data Storage with proper volumes and CONFIG_PATH
	// Note: Image name is localhost/kubernaut-datastorage:latest when loaded via kind load
	deploymentManifest := fmt.Sprintf(`
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
        imagePullPolicy: %s
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
        - name: tls-certs
          mountPath: /etc/tls
          readOnly: true
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
          initialDelaySeconds: 30
          periodSeconds: 10
      volumes:
      - name: config
        configMap:
          name: datastorage-config
      - name: secrets
        projected:
          sources:
          - secret:
              name: postgresql-secret
              items:
              - key: db-secrets.yaml
                path: db-secrets.yaml
          - secret:
              name: redis-secret
              items:
              - key: redis-secrets.yaml
                path: redis-secrets.yaml
       - name: tls-certs
         secret:
           secretName: datastorage-tls
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
`, GetImagePullPolicy())
	cmd = exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "apply", "-f", "-")
	cmd.Stdin = strings.NewReader(deploymentManifest)
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

func waitForWEDataStorageReady(ctx context.Context, kubeconfigPath string, output io.Writer) error {
	if err := waitForDeploymentReady(ctx, kubeconfigPath, "datastorage", output); err != nil {
		return err
	}
	// Brief wait for DS to initialize connections to PostgreSQL/Redis
	time.Sleep(5 * time.Second)
	return nil
}
