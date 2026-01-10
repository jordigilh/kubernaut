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
		cmd := exec.CommandContext(ctx, "kubectl",
			"--kubeconfig", kubeconfigPath,
			"-n", controllerNamespace,
			"exec", podName,
			"--", "sh", "-c",
			fmt.Sprintf("ls /tmp/notifications/%s 2>/dev/null || true", pattern))

		output, err := cmd.CombinedOutput()
		if err == nil && len(output) > 0 {
			// Found at least one file
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

	// Copy file from pod to host
	podPath := fmt.Sprintf("%s/%s:/tmp/notifications/%s",
		controllerNamespace, podName, foundFile)
	hostPath := filepath.Join(tmpDir, filepath.Base(foundFile))

	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"cp", podPath, hostPath)

	if output, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to copy file from pod: %w (output: %s)", err, string(output))
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

	cmd := exec.CommandContext(ctx, "kubectl",
		"--kubeconfig", kubeconfigPath,
		"-n", controllerNamespace,
		"exec", podName,
		"--", "sh", "-c",
		fmt.Sprintf("ls /tmp/notifications/%s 2>/dev/null || true", pattern))

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
func EventuallyFindFileInPod(pattern string) func() (string, error) {
	return func() (string, error) {
		return WaitForFileInPod(context.Background(), pattern, 500*time.Millisecond)
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
