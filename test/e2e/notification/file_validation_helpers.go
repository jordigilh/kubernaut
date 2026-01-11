/*
Copyright 2025 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS   OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package notification

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// WaitForFileInPod waits for a file matching the given pattern to appear in the pod,
// then copies it to the host for validation.
//
// This function bypasses the hostPath volume mount (which has reliability issues with
// macOS + Podman + Kind) and instead uses `kubectl cp` to directly extract files from
// the pod.
//
// Returns: Path to the copied file on the host (in a temp directory)
func WaitForFileInPod(ctx context.Context, pattern string, timeout time.Duration) (string, error) {
	// Create temp directory for copied files
	tmpDir, err := os.MkdirTemp("", "notification-e2e-files-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	// Find pod name
	podName, err := getNotificationPodName(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get pod name: %w", err)
	}

	var foundFile string
	pollInterval := 200 * time.Millisecond
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		// List files in pod matching pattern
		// Use -c to specify container and avoid "Defaulted container" messages
		cmd := exec.CommandContext(ctx, "kubectl",
			"--kubeconfig", kubeconfigPath,
			"-n", controllerNamespace,
			"exec", podName,
			"-c", "manager",  // Specify container to avoid "Defaulted container" messages
			"--", "sh", "-c",
			fmt.Sprintf("cd /tmp/notifications && ls %s 2>/dev/null || true", pattern))

		output, err := cmd.CombinedOutput()
		if err == nil && len(output) > 0 {
			// Found at least one file (just filename, not full path)
			files := strings.Split(strings.TrimSpace(string(output)), "\n")
			if len(files) > 0 && files[0] != "" {
				foundFile = files[0]
				break
			}
		}

		time.Sleep(pollInterval)
	}

	if foundFile == "" {
		// Cleanup temp dir if no file found
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("file matching pattern %s not found in pod within %v", pattern, timeout)
	}

	// Read file content from pod using kubectl exec (more reliable than kubectl cp)
	// foundFile is just the filename (from `cd && ls`), so append to directory
	filePath := fmt.Sprintf("/tmp/notifications/%s", foundFile)

	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"-n", controllerNamespace,
		"exec", podName,
		"-c", "manager",  // Specify container to avoid "Defaulted container" messages
		"--", "cat", filePath)

	fileContent, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to read file from pod: %w (output: %s)", err, string(fileContent))
	}

	// Write content to temp file on host
	hostPath := filepath.Join(tmpDir, foundFile)
	if err := os.WriteFile(hostPath, fileContent, 0644); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to write file to host: %w", err)
	}

	return hostPath, nil
}

// ListFilesInPod lists all files matching the given pattern in the pod.
// Returns a slice of filenames (not full paths).
func ListFilesInPod(ctx context.Context, pattern string) ([]string, error) {
	podName, err := getNotificationPodName(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod name: %w", err)
	}

	// Use cd to get just filenames, not full paths
	// Specify container to avoid "Defaulted container" messages
	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"-n", controllerNamespace,
		"exec", podName,
		"-c", "manager",
		"--", "sh", "-c",
		fmt.Sprintf("cd /tmp/notifications && ls %s 2>/dev/null || true", pattern))

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to list files in pod: %w", err)
	}

	if len(output) == 0 {
		return []string{}, nil
	}

	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	result := make([]string, 0, len(files))
	for _, f := range files {
		if f != "" {
			result = append(result, f)
		}
	}

	return result, nil
}

// getNotificationPodName returns the name of the notification controller pod
func getNotificationPodName(ctx context.Context) (string, error) {
	// Build clientset from kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return "", fmt.Errorf("failed to build config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create clientset: %w", err)
	}

	pods, err := clientset.CoreV1().Pods(controllerNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=notification-controller",
	})
	if err != nil {
		return "", fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return "", fmt.Errorf("no notification controller pods found")
	}

	return pods.Items[0].Name, nil
}

// EventuallyFindFileInPod is a Gomega-friendly wrapper around WaitForFileInPod
// that can be used with Eventually()
// Uses 2 second timeout per poll to allow for file creation lag in Kubernetes
func EventuallyFindFileInPod(pattern string) func() (string, error) {
	return func() (string, error) {
		return WaitForFileInPod(context.Background(), pattern, 2*time.Second)
	}
}

// CountFilesInPod returns the number of files matching the pattern in the pod
func CountFilesInPod(ctx context.Context, pattern string) (int, error) {
	files, err := ListFilesInPod(ctx, pattern)
	if err != nil {
		return 0, err
	}
	return len(files), nil
}

// EventuallyCountFilesInPod is a Gomega-friendly wrapper for counting files
// DD-NOT-006 v5: No artificial delay - relies on Eventually() timeout and polling
// Rationale: Controller writes file → overlay FS → Podman VM → Kind node FS
// This multi-layer sync can take 1-2s on macOS under light load, 5-10s under
// high concurrent load (12 parallel test processes) due to virtiofs contention.
// Tests wait for Phase==Sent before calling this, ensuring the write is
// complete, but the file may not be visible yet due to filesystem sync latency.
// Solution: Use longer Eventually timeout (15s) with frequent polling (1s) to
// adapt to varying sync latencies instead of betting on a fixed delay.
func EventuallyCountFilesInPod(pattern string) func() (int, error) {
	return func() (int, error) {
		return CountFilesInPod(context.Background(), pattern)
	}
}

// CleanupCopiedFile removes a file that was copied from the pod
func CleanupCopiedFile(filePath string) error {
	if filePath == "" {
		return nil
	}

	// Remove the temp directory containing the file
	tmpDir := filepath.Dir(filePath)
	if strings.HasPrefix(filepath.Base(tmpDir), "notification-e2e-files-") {
		return os.RemoveAll(tmpDir)
	}

	// Just remove the file if not in a temp directory
	return os.Remove(filePath)
}
