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

// WorkflowExecution E2E Test Infrastructure
//
// Sets up a complete environment for testing WorkflowExecution controller:
// - Kind cluster with Tekton Pipelines installed
// - Data Storage Service (PostgreSQL + DS) for audit events (BR-WE-005)
// - WorkflowExecution CRD deployed
// - WorkflowExecution controller running (with --datastorage-url configured)
// - Simple test pipeline bundle available

const (
	// WorkflowExecutionClusterName is the default Kind cluster name
	WorkflowExecutionClusterName = "workflowexecution-e2e"

	// TektonPipelinesVersion is the Tekton Pipelines version to install
	// NOTE: v1.7.0+ uses ghcr.io which doesn't require auth (gcr.io requires auth since 2025)
	TektonPipelinesVersion = "v1.7.0"

	// WorkflowExecutionNamespace is where the controller runs
	WorkflowExecutionNamespace = "kubernaut-system"

	// ExecutionNamespace is where PipelineRuns are created
	ExecutionNamespace = "kubernaut-workflows"

	// WorkflowExecutionMetricsHostPort is the host port for metrics endpoint
	// Mapped via Kind NodePort extraPortMappings (container: 30185 -> host: 9185)
	WorkflowExecutionMetricsHostPort = 9185
)

// CreateWorkflowExecutionCluster creates a Kind cluster for WorkflowExecution E2E tests
// It installs Tekton Pipelines and prepares the cluster for testing
func CreateWorkflowExecutionCluster(clusterName, kubeconfigPath string, output io.Writer) error {
	fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(output, "Creating WorkflowExecution E2E Kind Cluster\n")
	fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(output, "  Cluster: %s\n", clusterName)
	fmt.Fprintf(output, "  Kubeconfig: %s\n", kubeconfigPath)
	fmt.Fprintf(output, "  Tekton Version: %s\n", TektonPipelinesVersion)
	fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Find config file
	configPath, err := findKindConfig("kind-workflowexecution-config.yaml")
	if err != nil {
		return fmt.Errorf("failed to find Kind config: %w", err)
	}
	fmt.Fprintf(output, "Using Kind config: %s\n", configPath)

	// Create Kind cluster
	fmt.Fprintf(output, "\n📦 Creating Kind cluster...\n")
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
	fmt.Fprintf(output, "✅ Kind cluster created\n")

	// Install Tekton Pipelines
	fmt.Fprintf(output, "\n🔧 Installing Tekton Pipelines %s...\n", TektonPipelinesVersion)
	if err := installTektonPipelines(kubeconfigPath, output); err != nil {
		return fmt.Errorf("failed to install Tekton: %w", err)
	}
	fmt.Fprintf(output, "✅ Tekton Pipelines installed\n")

	// Deploy Data Storage infrastructure for audit events (BR-WE-005)
	// Following AIAnalysis E2E pattern: build → load → deploy (runs once, shared by 4 parallel procs)
	fmt.Fprintf(output, "\n🗄️  Deploying Data Storage infrastructure (BR-WE-005 audit events)...\n")

	// 1. Deploy PostgreSQL
	fmt.Fprintf(output, "  🐘 Deploying PostgreSQL...\n")
	if err := deployPostgreSQL(kubeconfigPath, output); err != nil {
		return fmt.Errorf("failed to deploy PostgreSQL: %w", err)
	}

	// 2. Deploy Redis
	fmt.Fprintf(output, "  🔴 Deploying Redis...\n")
	if err := deployRedis(kubeconfigPath, output); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 3. Wait for PostgreSQL and Redis to be ready (DS requires them)
	fmt.Fprintf(output, "  ⏳ Waiting for PostgreSQL to be ready...\n")
	if err := waitForDeploymentReady(kubeconfigPath, "postgres", output); err != nil {
		return fmt.Errorf("PostgreSQL did not become ready: %w", err)
	}
	fmt.Fprintf(output, "  ⏳ Waiting for Redis to be ready...\n")
	if err := waitForDeploymentReady(kubeconfigPath, "redis", output); err != nil {
		return fmt.Errorf("Redis did not become ready: %w", err)
	}

	// 4. Build and deploy Data Storage with proper ADR-030 config
	// Migrations are applied AFTER DS is ready (step 6)
	fmt.Fprintf(output, "  💾 Building and deploying Data Storage...\n")
	if err := deployDataStorageWithConfig(clusterName, kubeconfigPath, output); err != nil {
		return fmt.Errorf("failed to deploy Data Storage: %w", err)
	}

	// 5. Wait for DS to be ready
	fmt.Fprintf(output, "  ⏳ Waiting for Data Storage to be ready...\n")
	if err := waitForDataStorageReady(kubeconfigPath, output); err != nil {
		return fmt.Errorf("Data Storage did not become ready: %w", err)
	}
	fmt.Fprintf(output, "✅ Data Storage infrastructure deployed\n")

	// 6. Apply audit migrations using DS team's shared library
	// This creates: audit_events table + partitions + indexes
	// Required for BR-WE-005 audit persistence
	// Note: WE uses AIAnalysis pattern with label "app=postgres" (not "app=postgresql")
	fmt.Fprintf(output, "\n📋 Applying audit migrations...\n")
	migrationConfig := DefaultMigrationConfig(WorkflowExecutionNamespace, kubeconfigPath)
	migrationConfig.PostgresService = "postgres" // Match AIAnalysis pattern (label: app=postgres)
	if err := ApplyMigrationsWithConfig(context.Background(), MigrationConfig{
		Namespace:       WorkflowExecutionNamespace,
		KubeconfigPath:  kubeconfigPath,
		PostgresService: "postgres", // Match AIAnalysis PostgreSQL deployment
		PostgresUser:    "slm_user",
		PostgresDB:      "action_history",
		Tables:          AuditTables,
	}, output); err != nil {
		return fmt.Errorf("failed to apply audit migrations: %w", err)
	}

	// 7. Verify migrations applied successfully
	verifyConfig := DefaultMigrationConfig(WorkflowExecutionNamespace, kubeconfigPath)
	verifyConfig.PostgresService = "postgres"
	verifyConfig.Tables = AuditTables
	if err := VerifyMigrations(context.Background(), verifyConfig, output); err != nil {
		return fmt.Errorf("audit migration verification failed: %w", err)
	}
	fmt.Fprintf(output, "✅ Audit migrations verified\n")

	// Create execution namespace
	fmt.Fprintf(output, "\n📁 Creating execution namespace %s...\n", ExecutionNamespace)
	nsCmd := exec.Command("kubectl", "create", "namespace", ExecutionNamespace,
		"--kubeconfig", kubeconfigPath)
	nsCmd.Stdout = output
	nsCmd.Stderr = output
	if err := nsCmd.Run(); err != nil {
		// Namespace may already exist
		fmt.Fprintf(output, "Note: namespace creation returned error (may already exist): %v\n", err)
	}

	// Create image pull secret for quay.io (from podman auth)
	if err := createQuayPullSecret(kubeconfigPath, ExecutionNamespace, output); err != nil {
		fmt.Fprintf(output, "Warning: Could not create quay.io pull secret: %v\n", err)
		// Non-fatal - repos may be public
	}

	fmt.Fprintf(output, "\n✅ WorkflowExecution E2E cluster ready!\n")
	return nil
}

// createQuayPullSecret creates an image pull secret from the podman auth config
func createQuayPullSecret(kubeconfigPath, namespace string, output io.Writer) error {
	fmt.Fprintf(output, "🔐 Creating quay.io pull secret...\n")

	// Get the auth config from podman
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	authFile := filepath.Join(homeDir, ".config/containers/auth.json")
	if _, err := os.Stat(authFile); os.IsNotExist(err) {
		return fmt.Errorf("podman auth file not found at %s", authFile)
	}

	// Create the secret in the execution namespace
	secretCmd := exec.Command("kubectl", "create", "secret", "docker-registry", "quay-pull-secret",
		"--from-file=.dockerconfigjson="+authFile,
		"--namespace", namespace,
		"--kubeconfig", kubeconfigPath,
	)
	secretCmd.Stdout = output
	secretCmd.Stderr = output
	if err := secretCmd.Run(); err != nil {
		return fmt.Errorf("failed to create pull secret: %w", err)
	}

	// Create the secret in tekton-pipelines-resolvers namespace for bundle resolver
	secretResolverCmd := exec.Command("kubectl", "create", "secret", "docker-registry", "quay-pull-secret",
		"--from-file=.dockerconfigjson="+authFile,
		"--namespace", "tekton-pipelines-resolvers",
		"--kubeconfig", kubeconfigPath,
	)
	secretResolverCmd.Stdout = output
	secretResolverCmd.Stderr = output
	_ = secretResolverCmd.Run() // Ignore error if namespace doesn't exist

	// Patch the service account to use the pull secret
	patchCmd := exec.Command("kubectl", "patch", "serviceaccount", "kubernaut-workflow-runner",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"-p", `{"imagePullSecrets": [{"name": "quay-pull-secret"}]}`,
	)
	patchCmd.Stdout = output
	patchCmd.Stderr = output
	// Ignore error if service account doesn't exist yet
	_ = patchCmd.Run()

	// Also patch the default service account in execution namespace
	patchDefaultCmd := exec.Command("kubectl", "patch", "serviceaccount", "default",
		"-n", namespace,
		"--kubeconfig", kubeconfigPath,
		"-p", `{"imagePullSecrets": [{"name": "quay-pull-secret"}]}`,
	)
	patchDefaultCmd.Stdout = output
	patchDefaultCmd.Stderr = output
	_ = patchDefaultCmd.Run()

	// Patch the tekton-pipelines-resolvers service account
	patchResolverCmd := exec.Command("kubectl", "patch", "serviceaccount", "tekton-pipelines-resolvers",
		"-n", "tekton-pipelines-resolvers",
		"--kubeconfig", kubeconfigPath,
		"-p", `{"imagePullSecrets": [{"name": "quay-pull-secret"}]}`,
	)
	patchResolverCmd.Stdout = output
	patchResolverCmd.Stderr = output
	_ = patchResolverCmd.Run()

	fmt.Fprintf(output, "✅ Pull secret created and linked to service accounts\n")
	return nil
}

// DeleteWorkflowExecutionCluster deletes the Kind cluster
func DeleteWorkflowExecutionCluster(clusterName string, output io.Writer) error {
	fmt.Fprintf(output, "🗑️  Deleting Kind cluster %s...\n", clusterName)

	cmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete Kind cluster: %w", err)
	}

	fmt.Fprintf(output, "✅ Kind cluster deleted\n")
	return nil
}

// installTektonPipelines installs Tekton Pipelines using the official release manifests
func installTektonPipelines(kubeconfigPath string, output io.Writer) error {
	// Install Tekton Pipelines from GitHub releases (v1.0+ use GitHub releases)
	// NOTE: storage.googleapis.com/tekton-releases requires auth since 2025
	releaseURL := fmt.Sprintf("https://github.com/tektoncd/pipeline/releases/download/%s/release.yaml", TektonPipelinesVersion)

	fmt.Fprintf(output, "  Applying Tekton release from: %s\n", releaseURL)

	applyCmd := exec.Command("kubectl", "apply",
		"-f", releaseURL,
		"--kubeconfig", kubeconfigPath,
	)
	applyCmd.Stdout = output
	applyCmd.Stderr = output
	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply Tekton release: %w", err)
	}

	// Wait for Tekton controller to be ready (increased timeout for slow image pulls)
	fmt.Fprintf(output, "  ⏳ Waiting for Tekton Pipelines controller (up to 5 min)...\n")
	waitCmd := exec.Command("kubectl", "wait",
		"-n", "tekton-pipelines",
		"--for=condition=available",
		"deployment/tekton-pipelines-controller",
		"--timeout=300s",
		"--kubeconfig", kubeconfigPath,
	)
	waitCmd.Stdout = output
	waitCmd.Stderr = output
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("Tekton controller did not become ready: %w", err)
	}

	// Wait for Tekton webhook to be ready
	fmt.Fprintf(output, "  ⏳ Waiting for Tekton webhook (up to 5 min)...\n")
	webhookWaitCmd := exec.Command("kubectl", "wait",
		"-n", "tekton-pipelines",
		"--for=condition=available",
		"deployment/tekton-pipelines-webhook",
		"--timeout=300s",
		"--kubeconfig", kubeconfigPath,
	)
	webhookWaitCmd.Stdout = output
	webhookWaitCmd.Stderr = output
	if err := webhookWaitCmd.Run(); err != nil {
		return fmt.Errorf("Tekton webhook did not become ready: %w", err)
	}

	return nil
}

// DeployWorkflowExecutionController deploys the WorkflowExecution controller to the cluster
func DeployWorkflowExecutionController(ctx context.Context, namespace, kubeconfigPath string, output io.Writer) error {
	fmt.Fprintf(output, "\n🚀 Deploying WorkflowExecution Controller to %s...\n", namespace)

	// Find project root for absolute paths
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	// Create controller namespace
	nsCmd := exec.Command("kubectl", "create", "namespace", namespace,
		"--kubeconfig", kubeconfigPath)
	nsCmd.Stdout = output
	nsCmd.Stderr = output
	_ = nsCmd.Run() // May already exist

	// Deploy CRDs (use absolute path)
	crdPath := filepath.Join(projectRoot, "config/crd/bases/workflowexecution.kubernaut.ai_workflowexecutions.yaml")
	fmt.Fprintf(output, "  Applying WorkflowExecution CRDs...\n")
	crdCmd := exec.Command("kubectl", "apply",
		"-f", crdPath,
		"--kubeconfig", kubeconfigPath,
	)
	crdCmd.Stdout = output
	crdCmd.Stderr = output
	if err := crdCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply CRDs: %w", err)
	}

	// Build and load controller image (using podman since Kind uses it)
	fmt.Fprintf(output, "  Building controller image...\n")
	dockerfilePath := filepath.Join(projectRoot, "cmd/workflowexecution/Dockerfile")
	// Use docker.io prefix to ensure consistent naming when loaded into Kind
	imageName := "docker.io/kubernaut/workflowexecution-controller:e2e"
	buildCmd := exec.Command("podman", "build",
		"-t", imageName,
		"-f", dockerfilePath,
		projectRoot,
	)
	buildCmd.Stdout = output
	buildCmd.Stderr = output
	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("failed to build controller image: %w", err)
	}

	// Save image to tarball for Kind loading (Podman images need explicit save/load)
	fmt.Fprintf(output, "  Saving image for Kind cluster...\n")
	tarPath := filepath.Join(projectRoot, "workflowexecution-controller.tar")
	saveCmd := exec.Command("podman", "save", "-o", tarPath, imageName)
	saveCmd.Stdout = output
	saveCmd.Stderr = output
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save controller image: %w", err)
	}
	defer os.Remove(tarPath)

	// Load image into Kind from tarball
	fmt.Fprintf(output, "  Loading image into Kind cluster...\n")
	loadCmd := exec.Command("kind", "load", "image-archive", tarPath,
		"--name", WorkflowExecutionClusterName,
	)
	loadCmd.Stdout = output
	loadCmd.Stderr = output
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image into Kind: %w", err)
	}

	// Apply controller deployment (use absolute path)
	manifestsPath := filepath.Join(projectRoot, "test/e2e/workflowexecution/manifests/")
	fmt.Fprintf(output, "  Applying controller deployment...\n")
	deployCmd := exec.Command("kubectl", "apply",
		"-f", manifestsPath,
		"--kubeconfig", kubeconfigPath,
	)
	deployCmd.Stdout = output
	deployCmd.Stderr = output
	if err := deployCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply controller deployment: %w", err)
	}

	fmt.Fprintf(output, "✅ WorkflowExecution Controller deployed\n")
	return nil
}

// CreateSimpleTestPipeline creates a simple "hello world" pipeline for testing
// Also creates a failing pipeline for BR-WE-004 failure details testing
func CreateSimpleTestPipeline(kubeconfigPath string, output io.Writer) error {
	fmt.Fprintf(output, "\n📝 Creating test pipelines (success + failure)...\n")

	pipelineYAML := `
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: test-hello-world
  namespace: kubernaut-workflows
spec:
  params:
    - name: TARGET_RESOURCE
      type: string
      description: Target resource being remediated
    - name: MESSAGE
      type: string
      default: "Hello from Kubernaut!"
  tasks:
    - name: echo-hello
      taskRef:
        name: test-echo-task
      params:
        - name: message
          value: $(params.MESSAGE)
---
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: test-echo-task
  namespace: kubernaut-workflows
spec:
  params:
    - name: message
      type: string
  steps:
    - name: echo
      image: registry.access.redhat.com/ubi9/ubi-minimal:latest
      script: |
        #!/bin/sh
        echo "$(params.message)"
        echo "Test task completed successfully"
        sleep 2
---
# Intentionally failing pipeline for BR-WE-004 failure details testing
apiVersion: tekton.dev/v1
kind: Pipeline
metadata:
  name: test-intentional-failure
  namespace: kubernaut-workflows
spec:
  params:
    - name: TARGET_RESOURCE
      type: string
      description: Target resource being remediated
    - name: FAILURE_REASON
      type: string
      default: "Simulated failure for E2E testing"
  tasks:
    - name: fail-task
      taskRef:
        name: test-fail-task
      params:
        - name: reason
          value: $(params.FAILURE_REASON)
---
apiVersion: tekton.dev/v1
kind: Task
metadata:
  name: test-fail-task
  namespace: kubernaut-workflows
spec:
  params:
    - name: reason
      type: string
  steps:
    - name: fail
      image: registry.access.redhat.com/ubi9/ubi-minimal:latest
      script: |
        #!/bin/sh
        echo "Task will fail with reason: $(params.reason)"
        echo "This is an intentional failure for BR-WE-004 E2E testing"
        exit 1
`

	// Write to temp file and apply
	tmpFile, err := os.CreateTemp("", "test-pipeline-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(pipelineYAML); err != nil {
		return fmt.Errorf("failed to write pipeline YAML: %w", err)
	}
	tmpFile.Close()

	applyCmd := exec.Command("kubectl", "apply",
		"-f", tmpFile.Name(),
		"--kubeconfig", kubeconfigPath,
	)
	applyCmd.Stdout = output
	applyCmd.Stderr = output
	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to create test pipeline: %w", err)
	}

	fmt.Fprintf(output, "✅ Test pipeline created\n")
	return nil
}

// WaitForPipelineRunCompletion waits for a PipelineRun to complete
func WaitForPipelineRunCompletion(kubeconfigPath, prName, namespace string, timeout time.Duration, output io.Writer) error {
	fmt.Fprintf(output, "⏳ Waiting for PipelineRun %s/%s to complete (timeout: %v)...\n", namespace, prName, timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for PipelineRun completion")
		case <-ticker.C:
			// Check PipelineRun status
			statusCmd := exec.Command("kubectl", "get", "pipelinerun", prName,
				"-n", namespace,
				"-o", "jsonpath={.status.conditions[0].status}",
				"--kubeconfig", kubeconfigPath,
			)
			statusOutput, err := statusCmd.Output()
			if err != nil {
				continue // PipelineRun may not exist yet
			}

			status := string(statusOutput)
			if status == "True" {
				fmt.Fprintf(output, "✅ PipelineRun completed successfully\n")
				return nil
			} else if status == "False" {
				// Get failure reason
				reasonCmd := exec.Command("kubectl", "get", "pipelinerun", prName,
					"-n", namespace,
					"-o", "jsonpath={.status.conditions[0].reason}",
					"--kubeconfig", kubeconfigPath,
				)
				reasonOutput, _ := reasonCmd.Output()
				return fmt.Errorf("PipelineRun failed: %s", string(reasonOutput))
			}
			// status == "Unknown" means still running
			fmt.Fprintf(output, "  PipelineRun status: %s\n", status)
		}
	}
}

// deployDataStorageWithConfig deploys Data Storage with proper ADR-030 configuration
// This includes ConfigMap, Secrets, and CONFIG_PATH as required by DS
func deployDataStorageWithConfig(clusterName, kubeconfigPath string, output io.Writer) error {
	projectRoot := getProjectRoot()

	// Build Data Storage image
	fmt.Fprintln(output, "    Building Data Storage image...")
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
	fmt.Fprintln(output, "    Loading Data Storage image into Kind...")
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
      host: postgres
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

// waitForDeploymentReady waits for a deployment to be ready in kubernaut-system namespace
func waitForDeploymentReady(kubeconfigPath, deploymentName string, output io.Writer) error {
	waitCmd := exec.Command("kubectl", "wait",
		"-n", WorkflowExecutionNamespace,
		"--for=condition=available",
		"deployment/"+deploymentName,
		"--timeout=120s",
		"--kubeconfig", kubeconfigPath,
	)
	waitCmd.Stdout = output
	waitCmd.Stderr = output
	if err := waitCmd.Run(); err != nil {
		return fmt.Errorf("deployment %s did not become available: %w", deploymentName, err)
	}
	return nil
}

// waitForDataStorageReady waits for Data Storage deployment to be ready
// Uses kubectl wait with 120s timeout (DS may take time to connect to PostgreSQL)
func waitForDataStorageReady(kubeconfigPath string, output io.Writer) error {
	if err := waitForDeploymentReady(kubeconfigPath, "datastorage", output); err != nil {
		return err
	}
	// Brief wait for DS to initialize connections to PostgreSQL/Redis
	time.Sleep(5 * time.Second)
	return nil
}

// findProjectRoot finds the project root by looking for go.mod
func findProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	// Walk up to find project root (contains go.mod)
	projectRoot := cwd
	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err == nil {
			return projectRoot, nil
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			// Reached filesystem root, return cwd as fallback
			return cwd, nil
		}
		projectRoot = parent
	}
}

// findKindConfig finds the Kind config file relative to the project root
func findKindConfig(filename string) (string, error) {
	projectRoot, err := findProjectRoot()
	if err != nil {
		return "", err
	}
	cwd, _ := os.Getwd()

	// Try paths relative to project root
	paths := []string{
		filepath.Join(projectRoot, "test", "infrastructure", filename),
		filepath.Join(cwd, "test", "infrastructure", filename),
		filepath.Join(cwd, "..", "..", "test", "infrastructure", filename),
		filepath.Join(cwd, "..", "infrastructure", filename),
		filename,
	}

	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("Kind config file %s not found in any expected location (tried from %s)", filename, projectRoot)
}
