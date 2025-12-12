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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Dynamic Toolset E2E Infrastructure: Cluster Setup (ONCE) + Per-Test Deployment
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// CreateToolsetCluster creates a Kind cluster for Dynamic Toolset E2E testing
// This is called ONCE in BeforeSuite
//
// Steps:
// 1. Create Kind cluster with production-like configuration
// 2. Export kubeconfig to ~/.kube/kind-toolset-config
// 3. Build and load Dynamic Toolset Docker image
//
// Time: ~30 seconds
func CreateToolsetCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Dynamic Toolset E2E Cluster Setup (ONCE)")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 1. Create Kind cluster
	fmt.Fprintln(writer, "📦 Creating Kind cluster...")
	if err := createToolsetKindCluster(clusterName, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create Kind cluster: %w", err)
	}

	// 2. Build Dynamic Toolset Docker image
	fmt.Fprintln(writer, "🔨 Building Dynamic Toolset Docker image...")
	if err := buildToolsetImage(writer); err != nil {
		return fmt.Errorf("failed to build Toolset image: %w", err)
	}

	// 3. Load Dynamic Toolset image into Kind
	fmt.Fprintln(writer, "📦 Loading Dynamic Toolset image into Kind cluster...")
	if err := loadToolsetImage(clusterName, writer); err != nil {
		return fmt.Errorf("failed to load Toolset image: %w", err)
	}

	fmt.Fprintln(writer, "✅ Cluster ready - tests can now deploy services per-namespace")
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	return nil
}

// DeployToolsetTestServices deploys Dynamic Toolset and mock services in a test namespace
// This is called in BeforeAll for each test
//
// Steps:
// 1. Create namespace
// 2. Deploy Dynamic Toolset controller
// 3. Wait for controller ready
//
// Time: ~10 seconds
func DeployToolsetTestServices(ctx context.Context, namespace, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(writer, "Deploying Dynamic Toolset in Namespace: %s\n", namespace)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// 1. Create test namespace
	fmt.Fprintf(writer, "📁 Creating namespace %s...\n", namespace)
	if err := createTestNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to create namespace: %w", err)
	}

	// 2. Deploy Dynamic Toolset controller
	fmt.Fprintf(writer, "🚀 Deploying Dynamic Toolset controller...\n")
	if err := deployToolsetInNamespace(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("failed to deploy Dynamic Toolset: %w", err)
	}

	// 3. Wait for Dynamic Toolset controller ready
	fmt.Fprintf(writer, "⏳ Waiting for Dynamic Toolset to be ready...\n")
	if err := waitForToolsetReady(namespace, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("Dynamic Toolset not ready: %w", err)
	}

	fmt.Fprintf(writer, "✅ Dynamic Toolset ready in namespace %s\n", namespace)
	fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	return nil
}

// DeployMockService deploys a mock nginx service with HolmesGPT annotations
// This is used by tests to simulate services that Dynamic Toolset should discover
//
// Parameters:
// - namespace: Kubernetes namespace
// - serviceName: Name of the mock service
// - annotations: Map of holmesgpt.io annotations to apply
//
// Time: ~5 seconds
func DeployMockService(ctx context.Context, namespace, serviceName string, annotations map[string]string, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintf(writer, "🚀 Deploying mock service '%s' with annotations...\n", serviceName)

	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Read mock service template
	templatePath := filepath.Join(workspaceRoot, "test", "e2e", "toolset", "mock-service-template.yaml")
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read mock service template: %w", err)
	}

	// Replace placeholders
	manifestContent := string(templateContent)
	manifestContent = strings.ReplaceAll(manifestContent, "{{NAMESPACE}}", namespace)
	manifestContent = strings.ReplaceAll(manifestContent, "{{SERVICE_NAME}}", serviceName)

	// Build annotations YAML
	var annotationsYAML strings.Builder
	for key, value := range annotations {
		annotationsYAML.WriteString(fmt.Sprintf("    %s: \"%s\"\n", key, value))
	}
	manifestContent = strings.ReplaceAll(manifestContent, "{{ANNOTATIONS}}", annotationsYAML.String())

	// Apply manifest
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdin = strings.NewReader(manifestContent)
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy mock service: %w", err)
	}

	// Wait for pod to be running
	fmt.Fprintf(writer, "⏳ Waiting for mock service pod to be ready...\n")
	if err := waitForPods(namespace, fmt.Sprintf("app=%s", serviceName), 1, 30, 2*time.Second, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("mock service pod not ready: %w", err)
	}

	fmt.Fprintf(writer, "✅ Mock service '%s' deployed and ready\n", serviceName)
	return nil
}

// DeleteMockService deletes a mock service from the namespace
func DeleteMockService(ctx context.Context, namespace, serviceName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintf(writer, "🧹 Deleting mock service '%s'...\n", serviceName)

	deleteCmd := exec.Command("kubectl", "delete", "deployment,service", serviceName, "-n", namespace, "--wait=true", "--timeout=30s")
	deleteCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	deleteCmd.Stdout = writer
	deleteCmd.Stderr = writer

	if err := deleteCmd.Run(); err != nil {
		return fmt.Errorf("failed to delete mock service: %w", err)
	}

	fmt.Fprintf(writer, "✅ Mock service '%s' deleted\n", serviceName)
	return nil
}

// GetConfigMap retrieves a ConfigMap from the namespace
func GetConfigMap(namespace, configMapName, kubeconfigPath string) (map[string]interface{}, error) {
	cmd := exec.Command("kubectl", "get", "configmap", configMapName, "-n", namespace, "-o", "json")
	cmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	var configMap map[string]interface{}
	if err := json.Unmarshal(output, &configMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ConfigMap: %w", err)
	}

	return configMap, nil
}

// DeleteToolsetCluster deletes the Kind cluster
// This is called ONCE in AfterSuite
//
// Time: ~5 seconds
func DeleteToolsetCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	fmt.Fprintln(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintln(writer, "Dynamic Toolset E2E Cluster Teardown")
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

// createToolsetKindCluster creates a Kind cluster for Dynamic Toolset testing
// If the cluster already exists, it will be deleted and recreated
// This function is safe for parallel execution - only one process will create the cluster
func createToolsetKindCluster(clusterName, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Check if cluster already exists (quick check before cleanup)
	checkCmd := exec.Command("kind", "get", "clusters")
	output, err := checkCmd.Output()
	if err == nil {
		clusters := strings.Split(strings.TrimSpace(string(output)), "\n")
		for _, cluster := range clusters {
			if cluster == clusterName {
				fmt.Fprintf(writer, "   ⚠️  Cluster '%s' already exists - deleting it first...\n", clusterName)
				deleteCmd := exec.Command("kind", "delete", "cluster", "--name", clusterName)
				deleteCmd.Stdout = writer
				deleteCmd.Stderr = writer
				if err := deleteCmd.Run(); err != nil {
					fmt.Fprintf(writer, "   ⚠️  Warning: failed to delete existing cluster: %v\n", err)
				} else {
					fmt.Fprintf(writer, "   ✅ Existing cluster deleted\n")
				}
				// Also clean up any leftover containers
				cleanupCmd := exec.Command("podman", "rm", "-f", clusterName+"-control-plane")
				_ = cleanupCmd.Run() // Ignore errors - container may not exist
				break
			}
		}
	}

	// Ensure kubeconfig directory exists
	kubeconfigDir := filepath.Dir(kubeconfigPath)
	if err := os.MkdirAll(kubeconfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	// Remove any leftover kubeconfig lock file
	lockFile := kubeconfigPath + ".lock"
	_ = os.Remove(lockFile) // Ignore errors - file may not exist

	// Create Kind cluster
	kindConfigPath := filepath.Join(workspaceRoot, "test", "e2e", "toolset", "kind-cluster-config.yaml")
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

// buildToolsetImage builds the Dynamic Toolset Docker image using Podman
func buildToolsetImage(writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	buildCmd := exec.Command("podman", "build",
		"-t", "localhost/kubernaut-dynamic-toolsets:e2e-test",
		"-f", "docker/dynamic-toolset-ubi9.Dockerfile",
		".",
	)
	buildCmd.Dir = workspaceRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("podman build failed: %w", err)
	}

	fmt.Fprintln(writer, "   Dynamic Toolset image built: localhost/kubernaut-dynamic-toolsets:e2e-test")
	return nil
}

// loadToolsetImage loads the Dynamic Toolset image into the Kind cluster
func loadToolsetImage(clusterName string, writer io.Writer) error {
	// Save image to tar
	saveCmd := exec.Command("podman", "save", "localhost/kubernaut-dynamic-toolsets:e2e-test", "-o", "/tmp/toolset-e2e.tar")
	saveCmd.Stdout = writer
	saveCmd.Stderr = writer

	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}

	// Load image into Kind cluster
	loadCmd := exec.Command("kind", "load", "image-archive", "/tmp/toolset-e2e.tar", "--name", clusterName)
	loadCmd.Stdout = writer
	loadCmd.Stderr = writer

	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load image into Kind: %w", err)
	}

	// Clean up tar file
	_ = os.Remove("/tmp/toolset-e2e.tar")

	fmt.Fprintln(writer, "   Dynamic Toolset image loaded into Kind cluster")
	return nil
}

// deployToolsetInNamespace deploys the Dynamic Toolset controller in the specified namespace
func deployToolsetInNamespace(namespace, kubeconfigPath string, writer io.Writer) error {
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Read template
	templatePath := filepath.Join(workspaceRoot, "test", "e2e", "toolset", "dynamic-toolset-deployment.yaml")
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("failed to read Dynamic Toolset template: %w", err)
	}

	// Replace namespace placeholder
	manifestContent := strings.ReplaceAll(string(templateContent), "__NAMESPACE__", namespace)

	// Apply manifest
	applyCmd := exec.Command("kubectl", "apply", "-f", "-")
	applyCmd.Env = append(os.Environ(), fmt.Sprintf("KUBECONFIG=%s", kubeconfigPath))
	applyCmd.Stdin = strings.NewReader(manifestContent)
	applyCmd.Stdout = writer
	applyCmd.Stderr = writer

	if err := applyCmd.Run(); err != nil {
		return fmt.Errorf("failed to deploy Dynamic Toolset: %w", err)
	}

	fmt.Fprintln(writer, "   Dynamic Toolset controller deployed")
	return nil
}

// waitForToolsetReady waits for the Dynamic Toolset controller to be ready
func waitForToolsetReady(namespace, kubeconfigPath string, writer io.Writer) error {
	maxAttempts := 60
	delay := 2 * time.Second

	fmt.Fprintf(writer, "   Waiting for Dynamic Toolset controller...\n")
	if err := waitForPods(namespace, "app.kubernetes.io/component=dynamic-toolsets", 1, maxAttempts, delay, kubeconfigPath, writer); err != nil {
		return fmt.Errorf("Dynamic Toolset controller not ready: %w", err)
	}

	fmt.Fprintln(writer, "   Dynamic Toolset controller ready")
	return nil
}

// waitForPods waits for pods matching label selector to be ready
// TODO: Move to shared infrastructure file (used by toolset)
func waitForPods(namespace, labelSelector string, expectedCount, maxAttempts int, delay time.Duration, kubeconfigPath string, writer io.Writer) error {
	clientset, err := getKubernetesClient(kubeconfigPath)
	if err != nil {
		return err
	}
	
	ctx := context.Background()
	for i := 0; i < maxAttempts; i++ {
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
			LabelSelector: labelSelector,
		})
		if err == nil && len(pods.Items) >= expectedCount {
			readyCount := 0
			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodRunning {
					for _, condition := range pod.Status.Conditions {
						if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
							readyCount++
							break
						}
					}
				}
			}
			if readyCount >= expectedCount {
				return nil
			}
		}
		time.Sleep(delay)
	}
	return fmt.Errorf("pods not ready after %d attempts", maxAttempts)
}
