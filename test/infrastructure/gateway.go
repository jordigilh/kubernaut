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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// REFACTORED INFRASTRUCTURE: Cluster Setup (ONCE) + Per-Test Service Deployment
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CreateGatewayCluster creates a Kind cluster for Gateway E2E testing
// This is called ONCE in BeforeSuite
//
// Steps:
// 1. Create Kind cluster with production-like configuration
// 2. Export kubeconfig to ~/.kube/kind-config
// 3. Install RemediationRequest CRD (cluster-wide resource)
// 4. Build and load Gateway Docker image
//
// Time: ~40 seconds
func CreateGatewayCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Gateway E2E Cluster Setup (ONCE)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Create Kind cluster
	fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createKindClusterOnly(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Install RemediationRequest CRD (cluster-wide)
	fmt.Fprintln(writer, "📋 Installing RemediationRequest CRD...")
	if err := installCRD(kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to install CRD: %w", err)
	}

	// 3. Build Gateway Docker image
	fmt.Fprintln(writer, "🔨 Building Gateway Docker image...")
	if err := buildGatewayImageOnly(writer); err != nil {
		return fmt.Errorf("failed to build Gateway image: %w", err)
	}

	// 4. Load Gateway image into Kind
	fmt.Fprintln(writer, "📦 Loading Gateway image into Kind cluster...")
	if err := loadGatewayImageOnly(clusterName, writer); err != nil {
		return fmt.Errorf("failed to load Gateway image: %w", err)
	}

	fmt.Fprintln(writer, "✅ Cluster ready - tests can now deploy services per-namespace")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// DeployTestServices deploys Redis, AlertManager, and Gateway in a test namespace
// This is called in BeforeAll for each test
//
// Steps:
// 1. Create namespace
// 2. Deploy Redis Master-Replica (2 pods)
// 3. Deploy Prometheus AlertManager (1 pod)
// 4. Deploy Gateway (1 pod)
// 5. Wait for all services ready
//
// Time: ~20 seconds
func DeployTestServices(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Deploying Test Services in Namespace: %s\n", namespace)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 1. Create test namespace
	fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createNamespaceOnly(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 2. Create kubernaut-system namespace (fallback for CRD creation)
	fmt.Fprintf(writer, "📁 Creating kubernaut-system namespace (fallback for CRDs)...\n")
	if err := createNamespaceOnly("kubernaut-system", kubeconfigPath, writer); err != nil {
		// Ignore error if namespace already exists
		if !strings.Contains(err.Error(), "AlreadyExists") {
			return fmt.Errorf("failed to create kubernaut-system namespace: %w", err)
		}
		fmt.Fprintf(writer, "   kubernaut-system namespace already exists\n")
	}

	// 3. Deploy Redis Master-Replica
	fmt.Fprintf(writer, "🚀 Deploying Redis Master-Replica...\n")
	if err := deployRedisInNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	// 3. Deploy AlertManager
	fmt.Fprintf(writer, "🚀 Deploying AlertManager...\n")
	if err := deployAlertManagerInNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy AlertManager: %w", err)
	}

	// 4. Deploy Gateway
	fmt.Fprintf(writer, "🚀 Deploying Gateway...\n")
	if err := deployGatewayInNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Gateway: %w", err)
	}

	// 5. Wait for all services ready
	fmt.Fprintf(writer, "⏳ Waiting for services to be ready...\n")
	if err := waitForServicesReady(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("services not ready: %w", err)
	}

	fmt.Fprintf(writer, "✅ Test services ready in namespace %s\n", namespace)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

// CleanupTestNamespace deletes a test namespace and all resources
// This is called in AfterAll for each test (if test passed)
//
// Time: ~5 seconds
func CleanupTestNamespace(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintf(writer, "🧹 Cleaning up namespace %s...\n", namespace)

	cmd := exec.Command("kubectl", "delete", "namespace", namespace, "--wait=true", "--timeout=30s")
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to delete namespace: %w", err)
	}

	fmt.Fprintf(writer, "✅ Namespace %s deleted\n", namespace)
	return nil
}

// DeleteGatewayCluster deletes the Kind cluster
// This is called ONCE in AfterSuite
//
// Time: ~5 seconds
func DeleteGatewayCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Gateway E2E Cluster Teardown")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Delete Kind cluster
	deleteCmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
	deleteCmd.Stdout = writer
	deleteCmd.Stderr = writer

	if err := deleteCmd.Run(); err != nil {
		return fmt.Errorf("failed to delete Kind cluster: %w", err)
	}

	// Remove kubeconfig file
	if err := os.Remove(kubeconfigPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(writer, "⚠️  Failed to remove kubeconfig file: %v\n", err)
	}

	fmt.Fprintf(writer, "✅ Cluster %s deleted\n", clusterName)
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// INTERNAL HELPER FUNCTIONS
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// createKindClusterOnly creates a Kind cluster with the specified configuration
func createKindClusterOnly(clusterName, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Ensure kubeconfig directory exists
	kubeconfigDir := filepath.Dir(kubeconfigPath)
	if err := os.MkdirAll(kubeconfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	// Create Kind cluster
	kindConfigPath := filepath.Join(workspaceRoot, "test", "e2e", "gateway", "kind-cluster-config.yaml")
	createCmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", kindConfigPath,
		"--kubeconfig", kubeconfigPath,
	)
	createCmd.Stdout = writer
	createCmd.Stderr = writer

	if err := createCmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	fmt.Fprintf(writer, "   Cluster: %s\n", clusterName)
	fmt.Fprintf(writer, "   Kubeconfig: %s\n", kubeconfigPath)
	return nil
}

// installCRD installs the RemediationRequest CRD (cluster-wide resource)
func installCRD(kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	crdPath := filepath.Join(workspaceRoot, "config", "crd", "bases", "remediation.kubernaut.io_remediationrequests.yaml")
	applyCmd := exec.Command("kubectl", "apply", "-f", crdPath)
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to apply CRD: %w", err)
	}

	fmt.Fprintln(writer, "   RemediationRequest CRD installed")
	return nil
}

// buildGatewayImageOnly builds the Gateway Docker image using Podman
func buildGatewayImageOnly(writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	buildCmd := exec.Command("podman", "build",
		"-t", "localhost/kubernaut-gateway:e2e-test",
		"-f", "docker/gateway-ubi9.Dockerfile",
		".",
	)
	buildCmd.Dir = workspaceRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("podman build failed: %w", err)
	}

	fmt.Fprintln(writer, "   Gateway image built: localhost/kubernaut-gateway:e2e-test")
	return nil
}

// loadGatewayImageOnly loads the Gateway image into the Kind cluster
func loadGatewayImageOnly(clusterName string, writer io.Writer) error {
	// Save image to tar
	saveCmd := exec.Command("podman", "save", "localhost/kubernaut-gateway:e2e-test", "-o", "/tmp/gateway-e2e.tar")
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load image into Kind cluster
	loadCmd := exec.Command("kind", "load", "image-archive", "/tmp/gateway-e2e.tar", "--name", clusterName)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer

	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image into Kind: %w", err)
	}

	// Clean up tar file
	_ = os.Remove("/tmp/gateway-e2e.tar")

	fmt.Fprintln(writer, "   Gateway image loaded into Kind cluster")
	return nil
}

// createNamespaceOnly creates a namespace in Kubernetes
func createNamespaceOnly(namespace, kubeconfigPath string, writer io.Writer) error {
	createCmd := exec.Command("kubectl", "create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
	createCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	yamlOutput, err := createCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to generate namespace YAML: %w", err)
	}

	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdin = bytes.NewReader(yamlOutput)
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	fmt.Fprintf(writer, "   Namespace: %s\n", namespace)
	return nil
}

// deployRedisInNamespace deploys Redis Master-Replica in the specified namespace
func deployRedisInNamespace(namespace, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Read template
	templatePath := filepath.Join(workspaceRoot, "test", "e2e", "gateway", "redis-simple-ha.yaml")
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read Redis template: %w", err)
	}

	// Replace namespace placeholder
	manifestContent := strings.ReplaceAll(string(templateContent), "namespace: kubernaut-system", fmt.Sprintf("namespace: %s", namespace))

	// Apply manifest
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdin = strings.NewReader(manifestContent)
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Redis: %w", err)
	}

	fmt.Fprintln(writer, "   Redis Master-Replica deployed (2 pods)")
	return nil
}

// deployAlertManagerInNamespace deploys Prometheus AlertManager in the specified namespace
func deployAlertManagerInNamespace(namespace, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Read template
	templatePath := filepath.Join(workspaceRoot, "test", "e2e", "gateway", "alertmanager.yaml")
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read AlertManager template: %w", err)
	}

	// Replace namespace placeholder and webhook URL
	manifestContent := strings.ReplaceAll(string(templateContent), "namespace: kubernaut-system", fmt.Sprintf("namespace: %s", namespace))
	manifestContent = strings.ReplaceAll(manifestContent,
		"url: 'http://gateway-service.kubernaut-system.svc.cluster.local:8080/api/v1/webhook/prometheus'",
		fmt.Sprintf("url: 'http://gateway-service.%s.svc.cluster.local:8080/api/v1/signals/prometheus'", namespace))

	// Apply manifest
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdin = strings.NewReader(manifestContent)
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy AlertManager: %w", err)
	}

	fmt.Fprintln(writer, "   Prometheus AlertManager deployed")
	return nil
}

// deployGatewayInNamespace deploys the Gateway service in the specified namespace
func deployGatewayInNamespace(namespace, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Read template
	templatePath := filepath.Join(workspaceRoot, "test", "e2e", "gateway", "gateway-deployment.yaml")
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read Gateway template: %w", err)
	}

	// Replace namespace placeholder and Redis address (in both ConfigMap and args)
	manifestContent := strings.ReplaceAll(string(templateContent), "namespace: kubernaut-system", fmt.Sprintf("namespace: %s", namespace))
	manifestContent = strings.ReplaceAll(manifestContent,
		"redis_addr: \"redis-master.kubernaut-system.svc.cluster.local:6379\"",
		fmt.Sprintf("redis_addr: \"redis-master.%s.svc.cluster.local:6379\"", namespace))
	manifestContent = strings.ReplaceAll(manifestContent,
		"--redis=redis-master.kubernaut-system.svc.cluster.local:6379",
		fmt.Sprintf("--redis=redis-master.%s.svc.cluster.local:6379", namespace))

	// Apply manifest
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdin = strings.NewReader(manifestContent)
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Gateway: %w", err)
	}

	fmt.Fprintln(writer, "   Gateway service deployed")
	return nil
}

// waitForServicesReady waits for all services to be ready in the namespace
func waitForServicesReady(namespace, kubeconfigPath string, writer io.Writer) error {
	maxAttempts := 60
	delay := 2 * time.Second

	// Wait for Redis master
	fmt.Fprintf(writer, "   Waiting for Redis master...\n")
	if err := waitForPods(namespace, "app=redis,role=master", 1, maxAttempts, delay, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("Redis master not ready: %w", err)
	}

	// Wait for Redis replica
	fmt.Fprintf(writer, "   Waiting for Redis replica...\n")
	if err := waitForPods(namespace, "app=redis,role=replica", 1, maxAttempts, delay, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("Redis replica not ready: %w", err)
	}

	// Wait for AlertManager
	fmt.Fprintf(writer, "   Waiting for AlertManager...\n")
	if err := waitForPods(namespace, "app=alertmanager", 1, maxAttempts, delay, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("AlertManager not ready: %w", err)
	}

	// Wait for Gateway
	fmt.Fprintf(writer, "   Waiting for Gateway...\n")
	if err := waitForPods(namespace, "app=gateway", 1, maxAttempts, delay, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("Gateway not ready: %w", err)
	}

	fmt.Fprintln(writer, "   All services ready")
	return nil
}

// waitForPods waits for a specific number of pods matching a label selector to be ready
func waitForPods(namespace, labelSelector string, expectedCount int, maxAttempts int, delay time.Duration, kubeconfigPath string, writer io.Writer) error {
	for i := 0; i < maxAttempts; i++ {
		cmd := exec.Command("kubectl", "get", "pods", "-n", namespace, "-l", labelSelector, "--field-selector=status.phase=Running", "-o", "json")
		cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
		output, err := cmd.Output()
		if err != nil {
			fmt.Fprintf(writer, "      Warning: Failed to get pods: %v\n", err)
			time.Sleep(delay)
			continue
		}

		var podList struct {
			Items []interface{} `json:"items"`
		}
		if err := json.Unmarshal(output, &podList); err != nil {
			return fmt.Errorf("failed to unmarshal pod list: %w", err)
		}

		if len(podList.Items) == expectedCount {
			return nil
		}
		fmt.Fprintf(writer, "      Waiting for %d pods with selector '%s' to be ready, found %d. Attempt %d/%d\n", expectedCount, labelSelector, len(podList.Items), i+1, maxAttempts)
		time.Sleep(delay)
	}
	return fmt.Errorf("pods with label %s did not become ready after %d attempts", labelSelector, maxAttempts)
}

// RunCommand executes a shell command with KUBECONFIG set
// This is a helper function for E2E tests to query Kubernetes resources
func RunCommand(command, kubeconfigPath string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w, output: %s", err, string(output))
	}

	return strings.TrimSpace(string(output)), nil
}

// Note: findWorkspaceRoot() is defined in datastorage.go and shared across infrastructure files

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Redis Container Management for Integration Tests
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// StartRedisContainer starts a Redis container for integration tests
func StartRedisContainer(containerName string, port int, writer io.Writer) error {
	fmt.Fprintf(writer, "Starting Redis container '%s' on port %d...\n", containerName, port)

	// Check if container already exists
	checkCmd := exec.Command("podman", "ps", "-a", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, _ := checkCmd.CombinedOutput()
	if strings.TrimSpace(string(output)) == containerName {
		// Container exists, check if it's running
		statusCmd := exec.Command("podman", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
		statusOutput, _ := statusCmd.CombinedOutput()
		if strings.TrimSpace(string(statusOutput)) == containerName {
			fmt.Fprintf(writer, "✅ Redis container '%s' already running\n", containerName)
			return nil
		}

		// Container exists but not running, start it
		fmt.Fprintf(writer, "Starting existing Redis container '%s'...\n", containerName)
		startCmd := exec.Command("podman", "start", containerName)
		if err := startCmd.Run(); err != nil {
			return fmt.Errorf("failed to start existing Redis container: %w", err)
		}
		fmt.Fprintf(writer, "✅ Redis container '%s' started\n", containerName)
		return nil
	}

	// Create new container
	cmd := exec.Command("podman", "run", "-d",
		"--name", containerName,
		"-p", fmt.Sprintf("%d:6379", port),
		"redis:7-alpine")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start Redis container: %w, output: %s", err, string(output))
	}

	fmt.Fprintf(writer, "✅ Redis container '%s' created and started\n", containerName)
	return nil
}

// StopRedisContainer stops and removes a Redis container
func StopRedisContainer(containerName string, writer io.Writer) error {
	fmt.Fprintf(writer, "Stopping Redis container '%s'...\n", containerName)

	// Check if container exists
	checkCmd := exec.Command("podman", "ps", "-a", "--filter", fmt.Sprintf("name=%s", containerName), "--format", "{{.Names}}")
	output, _ := checkCmd.CombinedOutput()
	if strings.TrimSpace(string(output)) != containerName {
		fmt.Fprintf(writer, "✅ Redis container '%s' does not exist (already cleaned up)\n", containerName)
		return nil
	}

	// Stop container
	stopCmd := exec.Command("podman", "stop", containerName)
	if err := stopCmd.Run(); err != nil {
		fmt.Fprintf(writer, "⚠️  Warning: Failed to stop Redis container '%s': %v\n", containerName, err)
	}

	// Remove container
	rmCmd := exec.Command("podman", "rm", containerName)
	if err := rmCmd.Run(); err != nil {
		return fmt.Errorf("failed to remove Redis container: %w", err)
	}

	fmt.Fprintf(writer, "✅ Redis container '%s' stopped and removed\n", containerName)
	return nil
}
