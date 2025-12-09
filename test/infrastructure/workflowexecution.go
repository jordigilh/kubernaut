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
	"time"
)

// WorkflowExecution E2E Test Infrastructure
//
// Sets up a complete environment for testing WorkflowExecution controller:
// - Kind cluster with Tekton Pipelines installed
// - WorkflowExecution CRD deployed
// - WorkflowExecution controller running
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
