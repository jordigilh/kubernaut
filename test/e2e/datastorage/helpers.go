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

package datastorage

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// generateUniqueNamespace creates a unique namespace for parallel test execution
// Format: datastorage-e2e-p{process}-{timestamp}
// This enables parallel E2E tests by providing complete namespace isolation
func generateUniqueNamespace() string {
	return fmt.Sprintf("datastorage-e2e-p%d-%d",
		GinkgoParallelProcess(),
		time.Now().Unix())
}

// createNamespace creates a Kubernetes namespace
func createNamespace(namespace string) error {
	cmd := exec.Command("kubectl", "create", "namespace", namespace)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to create namespace %s: %w, output: %s", namespace, err, output)
	}
	GinkgoWriter.Printf("✅ Created namespace: %s\n", namespace)
	return nil
}

// deleteNamespace deletes a Kubernetes namespace and waits for cleanup
func deleteNamespace(namespace string) error {
	GinkgoWriter.Printf("🧹 Deleting namespace: %s\n", namespace)

	cmd := exec.Command("kubectl", "delete", "namespace", namespace, "--wait=true", "--timeout=60s")
	output, err := cmd.CombinedOutput()
	if err != nil {
		GinkgoWriter.Printf("⚠️  Failed to delete namespace %s: %v, output: %s\n", namespace, err, output)
		return fmt.Errorf("failed to delete namespace %s: %w", namespace, err)
	}

	GinkgoWriter.Printf("✅ Deleted namespace: %s\n", namespace)
	return nil
}

// waitForPodReady waits for a pod to be ready in the specified namespace
func waitForPodReady(namespace, labelSelector string, timeout time.Duration) error {
	GinkgoWriter.Printf("⏳ Waiting for pod with label %s in namespace %s to be ready...\n", labelSelector, namespace)

	Eventually(func() bool {
		cmd := exec.Command("kubectl", "get", "pods",
			"-n", namespace,
			"-l", labelSelector,
			"-o", "jsonpath={.items[0].status.phase}")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return false
		}
		phase := string(output)
		return phase == "Running"
	}, timeout, 2*time.Second).Should(BeTrue(),
		fmt.Sprintf("Pod with label %s should be ready in namespace %s", labelSelector, namespace))

	GinkgoWriter.Printf("✅ Pod ready: %s in namespace %s\n", labelSelector, namespace)
	return nil
}

// applyManifest applies a Kubernetes manifest to the specified namespace
func applyManifest(namespace, manifestPath string) error {
	cmd := exec.Command("kubectl", "apply", "-n", namespace, "-f", manifestPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to apply manifest %s to namespace %s: %w, output: %s",
			manifestPath, namespace, err, output)
	}
	GinkgoWriter.Printf("✅ Applied manifest: %s to namespace %s\n", manifestPath, namespace)
	return nil
}

// getServiceURL returns the service URL for accessing a service in the namespace
// For Kind clusters, this uses port-forwarding
func getServiceURL(namespace, serviceName string, port int) string {
	// For Kind clusters, we'll use kubectl port-forward
	// The actual port-forwarding will be set up in the test
	return fmt.Sprintf("http://localhost:%d", port)
}

// portForwardService starts port-forwarding for a service in the background
// Returns a context cancel function to stop port-forwarding
func portForwardService(ctx context.Context, namespace, serviceName string, localPort, remotePort int) (context.CancelFunc, error) {
	portForwardCtx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(portForwardCtx, "kubectl", "port-forward",
		"-n", namespace,
		fmt.Sprintf("service/%s", serviceName),
		fmt.Sprintf("%d:%d", localPort, remotePort))

	// Start port-forward in background
	err := cmd.Start()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start port-forward for %s/%s: %w", namespace, serviceName, err)
	}

	// Wait a bit for port-forward to establish
	time.Sleep(2 * time.Second)

	GinkgoWriter.Printf("✅ Port-forward started: %s/%s %d:%d\n", namespace, serviceName, localPort, remotePort)

	// Return cancel function to stop port-forwarding
	return cancel, nil
}

// execInPod executes a command in a pod
func execInPod(namespace, podName string, command []string) (string, error) {
	args := []string{"exec", "-n", namespace, podName, "--"}
	args = append(args, command...)

	cmd := exec.Command("kubectl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("failed to exec in pod %s/%s: %w, output: %s",
			namespace, podName, err, output)
	}

	return string(output), nil
}

// getPodName gets the name of the first pod matching the label selector
func getPodName(namespace, labelSelector string) (string, error) {
	cmd := exec.Command("kubectl", "get", "pods",
		"-n", namespace,
		"-l", labelSelector,
		"-o", "jsonpath={.items[0].metadata.name}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get pod name for label %s in namespace %s: %w",
			labelSelector, namespace, err)
	}

	podName := string(output)
	if podName == "" {
		return "", fmt.Errorf("no pod found with label %s in namespace %s", labelSelector, namespace)
	}

	return podName, nil
}

// scalePod scales a deployment to the specified number of replicas
func scalePod(namespace, deploymentName string, replicas int) error {
	cmd := exec.Command("kubectl", "scale", "deployment",
		"-n", namespace,
		deploymentName,
		fmt.Sprintf("--replicas=%d", replicas))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to scale deployment %s/%s to %d replicas: %w, output: %s",
			namespace, deploymentName, replicas, err, output)
	}

	GinkgoWriter.Printf("✅ Scaled deployment %s/%s to %d replicas\n", namespace, deploymentName, replicas)
	return nil
}

// waitForPodCount waits for the specified number of pods to be running
func waitForPodCount(namespace, labelSelector string, expectedCount int, timeout time.Duration) error {
	GinkgoWriter.Printf("⏳ Waiting for %d pods with label %s in namespace %s...\n",
		expectedCount, labelSelector, namespace)

	Eventually(func() int {
		cmd := exec.Command("kubectl", "get", "pods",
			"-n", namespace,
			"-l", labelSelector,
			"-o", "jsonpath={.items[*].status.phase}")
		output, err := cmd.CombinedOutput()
		if err != nil {
			return 0
		}

		// Count "Running" phases
		phases := string(output)
		if phases == "" {
			return 0
		}

		// Simple count of "Running" occurrences
		count := 0
		for i := 0; i < len(phases); i++ {
			if i+7 <= len(phases) && phases[i:i+7] == "Running" {
				count++
			}
		}
		return count
	}, timeout, 2*time.Second).Should(Equal(expectedCount),
		fmt.Sprintf("Should have %d running pods with label %s in namespace %s",
			expectedCount, labelSelector, namespace))

	GinkgoWriter.Printf("✅ %d pods ready: %s in namespace %s\n", expectedCount, labelSelector, namespace)
	return nil
}

