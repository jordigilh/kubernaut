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
	"net"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck // Ginkgo/Gomega convention
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck // Ginkgo/Gomega convention
)

// generateUniqueNamespace creates a unique namespace for parallel test execution
// Format: datastorage-e2e-p{process}-{timestamp}
// This enables parallel E2E tests by providing complete namespace isolation
func generateUniqueNamespace() string {
	return fmt.Sprintf("datastorage-e2e-p%d-%d",
		GinkgoParallelProcess(),
		time.Now().Unix())
}

// waitForPodReady waits for a pod to be ready in the specified namespace
func waitForPodReady(namespace, labelSelector, kubeconfigPath string, timeout time.Duration) error {
	GinkgoWriter.Printf("⏳ Waiting for pod with label %s in namespace %s to be ready...\n", labelSelector, namespace)

	Eventually(func() bool {
		cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "get", "pods",
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

// portForwardService starts port-forwarding for a service in the background
// Returns a context cancel function to stop port-forwarding
func portForwardService(ctx context.Context, namespace, serviceName, kubeconfigPath string, localPort, remotePort int) (context.CancelFunc, error) {
	portForwardCtx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(portForwardCtx, "kubectl", "--kubeconfig", kubeconfigPath, "port-forward",
		"-n", namespace,
		fmt.Sprintf("service/%s", serviceName),
		fmt.Sprintf("%d:%d", localPort, remotePort))

	// Start port-forward in background
	err := cmd.Start()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start port-forward for %s/%s: %w", namespace, serviceName, err)
	}

	// Per TESTING_GUIDELINES.md: Use Eventually() to verify port-forward is ready
	Eventually(func() bool {
		// Test port is accessible
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 500*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 30*time.Second, 1*time.Second).Should(BeTrue(), "Port-forward should be established")

	GinkgoWriter.Printf("✅ Port-forward started: %s/%s %d:%d\n", namespace, serviceName, localPort, remotePort)

	// Return cancel function to stop port-forwarding
	return cancel, nil
}

// scalePod scales a deployment to the specified number of replicas
func scalePod(namespace, deploymentName, kubeconfigPath string, replicas int) error {
	cmd := exec.Command("kubectl", "--kubeconfig", kubeconfigPath, "scale", "deployment",
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

// deleteNamespace deletes a namespace
func deleteNamespace(ctx context.Context, namespace, kubeconfigPath string) error {
	cmd := exec.CommandContext(ctx, "kubectl", "--kubeconfig", kubeconfigPath, "delete", "namespace", namespace, "--wait=false")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to delete namespace %s: %w, output: %s", namespace, err, output)
	}

	GinkgoWriter.Printf("✅ Namespace deletion initiated: %s\n", namespace)
	return nil
}
