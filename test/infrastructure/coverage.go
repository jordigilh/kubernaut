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
License.
*/

package infrastructure

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// E2E Binary Coverage Collection - GOCOVERDIR Pipeline
// Authority: DD-TEST-007 E2E Coverage Collection Standard
//
// Deployed Go service binaries built with GOFLAGS=-cover write coverage
// counters to GOCOVERDIR=/coverdata inside Kind pods.  This helper
// orchestrates:
//   1. Scale down the deployment (SIGTERM flushes coverage)
//   2. Wait for pod termination
//   3. Extract /coverdata from the Kind node via podman/docker cp
//   4. Convert to Go coverage profile format (textfmt)
//   5. Produce coverage_e2e_{service}_binary.out (project root)
//
// The Makefile post-step replaces Ginkgo's --coverprofile output with
// the _binary.out file so CI picks up the real deployed-service coverage.
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// E2ECoverageOptions configures coverage collection for a single service.
type E2ECoverageOptions struct {
	// ServiceName is the short service name used in file naming (e.g., "datastorage").
	ServiceName string
	// ClusterName is the Kind cluster name (e.g., "datastorage-e2e").
	ClusterName string
	// DeploymentName is the Kubernetes Deployment to scale down (e.g., "datastorage").
	DeploymentName string
	// Namespace is the Kubernetes namespace containing the deployment.
	Namespace string
	// KubeconfigPath is the path to the kubeconfig for the Kind cluster.
	KubeconfigPath string
}

// CollectE2EBinaryCoverage orchestrates the full GOCOVERDIR extraction pipeline
// for a single Go service running in a Kind cluster.
//
// It is designed to be called from SynchronizedAfterSuite (Process 1) BEFORE
// the Kind cluster is deleted.  Errors are returned but callers should treat
// them as non-fatal (coverage collection must never fail the test suite).
func CollectE2EBinaryCoverage(opts E2ECoverageOptions, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "📊 Collecting E2E binary coverage for %s\n", opts.ServiceName)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Step 1: Scale down the deployment to flush coverage (SIGTERM triggers write)
	if err := scaleDownDeploymentForCoverage(opts, writer); err != nil {
		return fmt.Errorf("scale-down failed: %w", err)
	}

	// Step 2: Determine local coverage directory
	projectRoot := getProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("cannot determine project root (go.mod not found)")
	}
	coverDir := filepath.Join(projectRoot, "coverdata", opts.ServiceName)

	// Step 3: Extract coverage data from Kind node
	if err := extractCoverageFromKindNode(opts.ClusterName, coverDir, writer); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Step 4: Convert to textfmt and produce the _binary.out file
	binaryOutFile := filepath.Join(projectRoot, fmt.Sprintf("coverage_e2e_%s_binary.out", opts.ServiceName))
	if err := convertCoverageToProfile(coverDir, binaryOutFile, writer); err != nil {
		return fmt.Errorf("conversion failed: %w", err)
	}

	// Step 4b: Remap container build paths to host paths
	// Docker images use WORKDIR /opt/app-root/src; Go embeds these paths at compile time.
	// go tool cover needs host paths to open source files.
	if err := remapCoveragePaths(binaryOutFile, projectRoot, writer); err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  Coverage path remapping failed (non-fatal): %v\n", err)
	}

	// Step 5: Log summary percentage
	logCoveragePercent(coverDir, writer)

	// Step 6: Generate HTML report for local debugging (best-effort)
	generateHTMLReport(coverDir, binaryOutFile, writer)

	_, _ = fmt.Fprintf(writer, "✅ Binary coverage written to %s\n", binaryOutFile)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	return nil
}

// scaleDownDeploymentForCoverage scales the deployment to 0 replicas and waits
// for pod termination.  The Go runtime flushes GOCOVERDIR data on SIGTERM.
func scaleDownDeploymentForCoverage(opts E2ECoverageOptions, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🔽 Scaling down %s/%s to 0 replicas...\n", opts.Namespace, opts.DeploymentName)

	cmd := exec.Command("kubectl", "--kubeconfig", opts.KubeconfigPath,
		"scale", "deployment", opts.DeploymentName,
		"-n", opts.Namespace, "--replicas=0")
	cmd.Stdout = writer
	cmd.Stderr = writer
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kubectl scale failed for %s: %w", opts.DeploymentName, err)
	}

	// Wait for pod deletion (blocks until pod is fully terminated)
	_, _ = fmt.Fprintf(writer, "⏳ Waiting for %s pod to terminate...\n", opts.DeploymentName)
	waitCmd := exec.Command("kubectl", "--kubeconfig", opts.KubeconfigPath,
		"wait", "--for=delete", "pod",
		"-l", fmt.Sprintf("app=%s", opts.DeploymentName),
		"-n", opts.Namespace,
		"--timeout=60s")
	waitCmd.Stdout = writer
	waitCmd.Stderr = writer
	_ = waitCmd.Run() // Ignore error if no matching pods exist

	// Brief additional wait to ensure coverage data is fully written to disk
	time.Sleep(2 * time.Second)

	_, _ = fmt.Fprintf(writer, "✅ %s scaled down, coverage data flushed\n", opts.DeploymentName)
	return nil
}

// extractCoverageFromKindNode copies /coverdata from the Kind node container
// to a local directory.  Tries the worker node first, then falls back to
// the control-plane node.
func extractCoverageFromKindNode(clusterName, coverDir string, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "📦 Extracting coverage data from Kind node...")

	// Create local coverage directory
	if err := os.MkdirAll(coverDir, 0755); err != nil {
		return fmt.Errorf("failed to create coverage directory %s: %w", coverDir, err)
	}

	// Try worker node first (most common), then control-plane
	nodeNames := []string{
		clusterName + "-worker",
		clusterName + "-control-plane",
	}

	var lastErr error
	for _, nodeName := range nodeNames {
		// Try podman first (CI uses podman), then docker
		for _, runtime := range []string{"podman", "docker"} {
			// DD-TEST-007: List /coverdata contents before copy for diagnostics
			listCmd := exec.Command(runtime, "exec", nodeName,
				"sh", "-c", "echo '=== /coverdata contents ===' && ls -la /coverdata/ 2>&1 && echo '=== /coverdata permissions ===' && stat /coverdata/ 2>&1 || true")
			if listOutput, listErr := listCmd.CombinedOutput(); listErr == nil {
				_, _ = fmt.Fprintf(writer, "📋 Kind node %s (%s):\n%s\n", nodeName, runtime, listOutput)
			}

			cmd := exec.Command(runtime, "cp",
				nodeName+":/coverdata/.",
				coverDir)
			output, err := cmd.CombinedOutput()
			if err == nil {
				_, _ = fmt.Fprintf(writer, "✅ Extracted from %s via %s\n", nodeName, runtime)
				return validateExtractedFiles(coverDir, writer)
			}
			lastErr = fmt.Errorf("%s cp from %s failed: %w (output: %s)", runtime, nodeName, err, output)
		}
	}

	return fmt.Errorf("all extraction attempts failed, last error: %w", lastErr)
}

// validateExtractedFiles checks that the coverage directory contains data files.
func validateExtractedFiles(coverDir string, writer io.Writer) error {
	files, err := os.ReadDir(coverDir)
	if err != nil {
		return fmt.Errorf("failed to read coverage directory: %w", err)
	}

	covCount := 0
	for _, f := range files {
		if !f.IsDir() {
			covCount++
		}
	}

	if covCount == 0 {
		_, _ = fmt.Fprintln(writer, "⚠️  No coverage files found in extracted data (service may not have processed requests)")
		return fmt.Errorf("no coverage data files found in %s", coverDir)
	}

	_, _ = fmt.Fprintf(writer, "📁 Found %d coverage data files\n", covCount)
	return nil
}

// convertCoverageToProfile converts raw GOCOVERDIR data to the standard Go
// coverage profile format using `go tool covdata textfmt`.  The output file
// is compatible with `go tool cover -func` and `go tool cover -html`.
func convertCoverageToProfile(coverDir, outputFile string, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🔄 Converting coverage data to profile format...\n")

	cmd := exec.Command("go", "tool", "covdata", "textfmt",
		"-i="+coverDir,
		"-o="+outputFile)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go tool covdata textfmt failed: %w (output: %s)", err, output)
	}

	// Verify the output file was created and is non-empty
	info, err := os.Stat(outputFile)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("textfmt produced empty or missing output file: %s", outputFile)
	}

	_, _ = fmt.Fprintf(writer, "✅ Profile written: %s (%d bytes)\n", outputFile, info.Size())
	return nil
}

// remapCoveragePaths rewrites container build paths to host paths in the coverage profile.
// Docker images use WORKDIR /opt/app-root/src; Go embeds these absolute paths at compile time
// via -cover. When `go tool cover -func` runs on the host, it cannot find files at the
// container path. This function rewrites /opt/app-root/src → projectRoot so coverage tools work.
func remapCoveragePaths(profileFile, projectRoot string, writer io.Writer) error {
	const containerBuildPath = "/opt/app-root/src"

	content, err := os.ReadFile(profileFile)
	if err != nil {
		return fmt.Errorf("read profile: %w", err)
	}

	if !strings.Contains(string(content), containerBuildPath) {
		_, _ = fmt.Fprintf(writer, "  ℹ️  No container paths found in profile (no remapping needed)\n")
		return nil
	}

	remapped := strings.ReplaceAll(string(content), containerBuildPath, projectRoot)
	if err := os.WriteFile(profileFile, []byte(remapped), 0644); err != nil {
		return fmt.Errorf("write remapped profile: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Remapped container paths: %s → %s\n", containerBuildPath, projectRoot)
	return nil
}

// logCoveragePercent logs the coverage percentage summary (best-effort).
func logCoveragePercent(coverDir string, writer io.Writer) {
	cmd := exec.Command("go", "tool", "covdata", "percent", "-i="+coverDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  Could not compute coverage percent: %v\n", err)
		return
	}
	_, _ = fmt.Fprintf(writer, "\n📊 Coverage summary:\n%s\n", output)
}

// generateHTMLReport produces an HTML coverage report for local debugging (best-effort).
func generateHTMLReport(coverDir, profileFile string, writer io.Writer) {
	htmlFile := filepath.Join(coverDir, "e2e-coverage.html")
	cmd := exec.Command("go", "tool", "cover",
		"-html="+profileFile,
		"-o="+htmlFile)
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  HTML report generation failed: %v\n", err)
		return
	}
	_, _ = fmt.Fprintf(writer, "📄 HTML report: %s\n", htmlFile)
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Python E2E Coverage Collection
//
// Python services (HAPI) use coverage.py instead of GOCOVERDIR.
// The entrypoint wraps uvicorn with `coverage run` when E2E_COVERAGE=true,
// writing a .coverage SQLite database to /coverdata/.coverage.
//
// This helper orchestrates:
//  1. Scale down the deployment (SIGTERM causes coverage flush)
//  2. Wait for pod termination
//  3. Extract /coverdata/.coverage from the Kind node
//  4. Generate text report via `python3 -m coverage report` with path remapping
//  5. Produce coverage_e2e_{service}_python.txt (project root)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// E2EPythonCoverageOptions configures Python coverage collection for a service.
type E2EPythonCoverageOptions struct {
	// ServiceName is the short service name used in file naming (e.g., "holmesgpt-api").
	ServiceName string
	// ClusterName is the Kind cluster name.
	ClusterName string
	// DeploymentName is the Kubernetes Deployment to scale down.
	DeploymentName string
	// Namespace is the Kubernetes namespace containing the deployment.
	Namespace string
	// KubeconfigPath is the path to the kubeconfig for the Kind cluster.
	KubeconfigPath string
	// SourceDir is the path to Python source (relative to project root) for path remapping.
	// e.g., "holmesgpt-api/src"
	SourceDir string
	// ContainerSourceDir is the source path inside the container.
	// e.g., "/opt/app-root/src/src"
	ContainerSourceDir string
}

// CollectE2EPythonCoverage orchestrates Python coverage extraction from a Kind cluster.
// Designed for SynchronizedAfterSuite, BEFORE the Kind cluster is deleted.
// Errors are non-fatal (coverage must never fail the test suite).
//
// Strategy: Extract .coverage directly from the running pod via kubectl cp,
// bypassing the Kind node hostPath mount chain (which is unreliable with Podman).
// Steps:
//  1. Find the running pod
//  2. Send SIGTERM to Python inside the pod (triggers coverage.py atexit flush)
//  3. Wait briefly for .coverage to be written
//  4. kubectl cp the .coverage file from the pod
//  5. Scale down the deployment
//  6. Generate coverage report
func CollectE2EPythonCoverage(opts E2EPythonCoverageOptions, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "📊 Collecting Python E2E coverage for %s\n", opts.ServiceName)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	projectRoot := getProjectRoot()
	if projectRoot == "" {
		return fmt.Errorf("cannot determine project root (go.mod not found)")
	}
	coverDir := filepath.Join(projectRoot, "coverdata", opts.ServiceName)
	if err := os.MkdirAll(coverDir, 0755); err != nil {
		return fmt.Errorf("failed to create coverage directory: %w", err)
	}

	covFile := filepath.Join(coverDir, ".coverage")

	// Step 1: Find the running pod name
	_, _ = fmt.Fprintln(writer, "🔍 Finding running pod...")
	getPodCmd := exec.Command("kubectl", "--kubeconfig", opts.KubeconfigPath,
		"get", "pods", "-n", opts.Namespace,
		"-l", fmt.Sprintf("app=%s", opts.DeploymentName),
		"-o", "jsonpath={.items[0].metadata.name}",
		"--field-selector=status.phase=Running")
	podNameBytes, err := getPodCmd.Output()
	if err != nil || len(podNameBytes) == 0 {
		_, _ = fmt.Fprintf(writer, "⚠️  No running pod found for app=%s, trying scale-down extraction fallback\n", opts.DeploymentName)
		return collectPythonCoverageFallback(opts, coverDir, covFile, projectRoot, writer)
	}
	podName := string(podNameBytes)
	_, _ = fmt.Fprintf(writer, "✅ Found pod: %s\n", podName)

	// Step 2: Check if .coverage file exists before SIGTERM (diagnostic)
	_, _ = fmt.Fprintln(writer, "📋 Checking /coverdata/ inside pod before SIGTERM...")
	lsCmd := exec.Command("kubectl", "--kubeconfig", opts.KubeconfigPath,
		"exec", "-n", opts.Namespace, podName, "--",
		"sh", "-c", "ls -la /coverdata/ 2>&1; echo '---'; id; echo '---'; cat /tmp/.coveragerc 2>/dev/null || echo 'no .coveragerc'")
	if lsOutput, lsErr := lsCmd.CombinedOutput(); lsErr == nil {
		_, _ = fmt.Fprintf(writer, "%s\n", lsOutput)
	}

	// Step 3: Send SIGUSR1 to the Python/coverage launcher process.
	// The run_with_coverage.py launcher registers a SIGUSR1 handler that calls
	// cov.stop() + cov.save() and keeps the process running (so kubectl cp works).
	// SIGUSR1 is used because uvicorn only intercepts SIGTERM and SIGINT —
	// our handler survives uvicorn.run() startup.
	_, _ = fmt.Fprintln(writer, "📤 Sending SIGUSR1 to Python process inside pod...")
	killCmd := exec.Command("kubectl", "--kubeconfig", opts.KubeconfigPath,
		"exec", "-n", opts.Namespace, podName, "--",
		"sh", "-c", `
PYTHON_PID=$(ps -eo pid,comm,args | grep '[p]ython3.*run_with_coverage' | awk '{print $1}' | head -1)
if [ -n "$PYTHON_PID" ]; then
    echo "Found Python/coverage launcher: PID $PYTHON_PID"
    kill -USR1 "$PYTHON_PID"
    echo "SIGUSR1 sent to PID $PYTHON_PID (exit=$?)"
else
    echo "No Python/coverage launcher found, trying fallback pattern..."
    PYTHON_PID=$(ps -eo pid,comm,args | grep '[p]ython3.*uvicorn' | awk '{print $1}' | head -1)
    if [ -n "$PYTHON_PID" ]; then
        echo "Found Python/uvicorn process: PID $PYTHON_PID"
        kill -USR1 "$PYTHON_PID"
        echo "SIGUSR1 sent to PID $PYTHON_PID (exit=$?)"
    else
        echo "ERROR: No Python process found"
        ps -eo pid,comm,args
    fi
fi
`)
	if killOutput, killErr := killCmd.CombinedOutput(); killErr == nil {
		_, _ = fmt.Fprintf(writer, "   %s", killOutput)
	} else {
		_, _ = fmt.Fprintf(writer, "   Signal command failed: %s (err: %v)\n", killOutput, killErr)
	}

	// Step 4: Wait for the launcher's SIGUSR1 handler to flush .coverage file.
	// The handler calls cov.stop() + cov.save() and keeps the process running.
	_, _ = fmt.Fprintln(writer, "⏳ Waiting for coverage data flush (up to 20s)...")
	covPath := "/coverdata/.coverage"
	const maxPollIterations = 20
	for i := 0; i < maxPollIterations; i++ {
		time.Sleep(1 * time.Second)
		checkCmd := exec.Command("kubectl", "--kubeconfig", opts.KubeconfigPath,
			"exec", "-n", opts.Namespace, podName, "--",
			"sh", "-c", fmt.Sprintf("test -f %s && stat -c '%%s' %s 2>/dev/null || echo 'not_found'", covPath, covPath))
		checkOutput, checkErr := checkCmd.CombinedOutput()
		result := string(checkOutput)
		if checkErr != nil {
			// Pod may have terminated (container restarted or deleted)
			// This is expected: after double-wait, bash exits → container terminates.
			// The .coverage file should be on the hostPath volume - try kubectl cp or fallback.
			_, _ = fmt.Fprintf(writer, "   Pod no longer responding (iteration %d), proceeding to extract\n", i+1)
			break
		}
		if result != "" && result != "not_found\n" && result != "not_found" {
			_, _ = fmt.Fprintf(writer, "   ✅ .coverage file detected (%s bytes) after %ds\n", strings.TrimSpace(result), i+1)
			break
		}
		if i == maxPollIterations-1 {
			_, _ = fmt.Fprintf(writer, "   ⚠️  .coverage not detected after %ds\n", maxPollIterations)
			// Final diagnostic: check if Python process is still running
			psCmd := exec.Command("kubectl", "--kubeconfig", opts.KubeconfigPath,
				"exec", "-n", opts.Namespace, podName, "--",
				"sh", "-c", "ps aux 2>/dev/null || echo 'ps not available'")
			if psOutput, psErr := psCmd.CombinedOutput(); psErr == nil {
				_, _ = fmt.Fprintf(writer, "   Process state:\n%s\n", psOutput)
			}
		}
	}

	// Step 5: kubectl cp the .coverage file from the pod
	_, _ = fmt.Fprintln(writer, "📦 Extracting .coverage via kubectl cp...")
	cpCmd := exec.Command("kubectl", "--kubeconfig", opts.KubeconfigPath,
		"cp", fmt.Sprintf("%s/%s:%s", opts.Namespace, podName, covPath), covFile)
	cpOutput, cpErr := cpCmd.CombinedOutput()
	if cpErr != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  kubectl cp failed: %s (%v)\n", cpOutput, cpErr)
		// Fallback: try extracting from Kind node
		_, _ = fmt.Fprintln(writer, "🔄 Falling back to Kind node extraction...")
		return collectPythonCoverageFallback(opts, coverDir, covFile, projectRoot, writer)
	}

	// Step 6: Scale down deployment (cleanup)
	_, _ = fmt.Fprintln(writer, "🔽 Scaling down deployment...")
	scaleCmd := exec.Command("kubectl", "--kubeconfig", opts.KubeconfigPath,
		"scale", "deployment", opts.DeploymentName,
		"-n", opts.Namespace, "--replicas=0")
	_ = scaleCmd.Run()

	// Step 7: Verify and generate report
	if _, err := os.Stat(covFile); os.IsNotExist(err) {
		return fmt.Errorf(".coverage file not found after extraction: %s", covFile)
	}
	fileInfo, _ := os.Stat(covFile)
	_, _ = fmt.Fprintf(writer, "✅ .coverage extracted: %s (%d bytes)\n", covFile, fileInfo.Size())

	// Step 8: Generate text report
	outputFile := filepath.Join(projectRoot, fmt.Sprintf("coverage_e2e_%s_python.txt", opts.ServiceName))
	if err := generatePythonCoverageReport(covFile, outputFile, opts, writer); err != nil {
		return fmt.Errorf("report generation failed: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Python coverage report written to %s\n", outputFile)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	return nil
}

// collectPythonCoverageFallback uses the original Kind node extraction as a fallback.
func collectPythonCoverageFallback(opts E2EPythonCoverageOptions, coverDir, covFile, projectRoot string, writer io.Writer) error {
	// Scale down to flush coverage
	if err := scaleDownDeploymentForCoverage(E2ECoverageOptions{
		ServiceName:    opts.ServiceName,
		ClusterName:    opts.ClusterName,
		DeploymentName: opts.DeploymentName,
		Namespace:      opts.Namespace,
		KubeconfigPath: opts.KubeconfigPath,
	}, writer); err != nil {
		return fmt.Errorf("scale-down failed: %w", err)
	}

	// Extract from Kind node
	if err := extractCoverageFromKindNode(opts.ClusterName, coverDir, writer); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Check for .coverage file
	if _, err := os.Stat(covFile); os.IsNotExist(err) {
		files, listErr := os.ReadDir(coverDir)
		if listErr == nil {
			_, _ = fmt.Fprintf(writer, "📁 Files found in %s:\n", coverDir)
			for _, f := range files {
				info, _ := f.Info()
				if info != nil {
					_, _ = fmt.Fprintf(writer, "   - %s (%d bytes, dir=%v)\n", f.Name(), info.Size(), f.IsDir())
				} else {
					_, _ = fmt.Fprintf(writer, "   - %s (dir=%v)\n", f.Name(), f.IsDir())
				}
			}
		}
		return fmt.Errorf(".coverage file not found in extracted data: %s", covFile)
	}

	// Generate report
	outputFile := filepath.Join(projectRoot, fmt.Sprintf("coverage_e2e_%s_python.txt", opts.ServiceName))
	if err := generatePythonCoverageReport(covFile, outputFile, opts, writer); err != nil {
		return fmt.Errorf("report generation failed: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "✅ Python coverage report written to %s\n", outputFile)
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")
	return nil
}

// generatePythonCoverageReport runs `python3 -m coverage report` with path remapping
// to produce a text report from the extracted .coverage database.
func generatePythonCoverageReport(covFile, outputFile string, opts E2EPythonCoverageOptions, writer io.Writer) error {
	_, _ = fmt.Fprintln(writer, "🔄 Generating Python coverage report with path remapping...")

	projectRoot := getProjectRoot()

	// Create a temporary .coveragerc with path remapping
	// This maps container paths to local source paths
	rcContent := fmt.Sprintf(`[paths]
source =
    %s
    %s

[report]
show_missing = true
skip_covered = false
`, filepath.Join(projectRoot, opts.SourceDir)+"/", opts.ContainerSourceDir+"/")

	rcFile := filepath.Join(projectRoot, ".coveragerc-e2e-"+opts.ServiceName)
	if err := os.WriteFile(rcFile, []byte(rcContent), 0644); err != nil {
		return fmt.Errorf("failed to write .coveragerc: %w", err)
	}
	defer os.Remove(rcFile) // Clean up temp file

	// Copy .coverage to project root for coverage tool (it expects .coverage in cwd)
	tempCovFile := filepath.Join(projectRoot, ".coverage")
	input, err := os.ReadFile(covFile)
	if err != nil {
		return fmt.Errorf("failed to read .coverage: %w", err)
	}
	if err := os.WriteFile(tempCovFile, input, 0644); err != nil {
		return fmt.Errorf("failed to copy .coverage: %w", err)
	}
	defer os.Remove(tempCovFile) // Clean up

	// Run `coverage combine` to remap paths (reads .coveragerc [paths] section)
	combineCmd := exec.Command("python3", "-m", "coverage", "combine",
		"--rcfile="+rcFile)
	combineCmd.Dir = projectRoot
	combineOutput, err := combineCmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  coverage combine output: %s\n", combineOutput)
		// Non-fatal: coverage report might still work without combine
	} else {
		_, _ = fmt.Fprintf(writer, "✅ Paths remapped via coverage combine\n")
	}

	// Run `coverage report` to generate text output
	reportCmd := exec.Command("python3", "-m", "coverage", "report",
		"--rcfile="+rcFile,
		"--include="+filepath.Join(projectRoot, opts.SourceDir, "*"))
	reportCmd.Dir = projectRoot
	reportOutput, err := reportCmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  coverage report output: %s\n", reportOutput)
		return fmt.Errorf("coverage report failed: %w", err)
	}

	// Write the report to the output file
	if err := os.WriteFile(outputFile, reportOutput, 0644); err != nil {
		return fmt.Errorf("failed to write report: %w", err)
	}

	// Log the report summary
	_, _ = fmt.Fprintf(writer, "\n📊 Python coverage report:\n%s\n", reportOutput)
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// YAML Manifest Helpers for GOCOVERDIR instrumentation
//
// These functions return YAML snippets (or empty strings) that can be
// embedded in deployment manifests via fmt.Sprintf to conditionally add
// GOCOVERDIR environment variable and /coverdata volume mounts.
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// coverageEnvYAML returns the YAML snippet for the GOCOVERDIR env var
// when E2E_COVERAGE=true, or an empty string otherwise.
// The indentation assumes container env section (8 spaces).
func coverageEnvYAML(serviceName string) string {
	if os.Getenv("E2E_COVERAGE") != "true" {
		return ""
	}
	return `- name: GOCOVERDIR
          value: /coverdata`
}

// coverageVolumeMountYAML returns the YAML snippet for the /coverdata
// volume mount when E2E_COVERAGE=true, or an empty string otherwise.
func coverageVolumeMountYAML() string {
	if os.Getenv("E2E_COVERAGE") != "true" {
		return ""
	}
	return `- name: coverdata
          mountPath: /coverdata`
}

// coverageVolumeYAML returns the YAML snippet for the coverdata hostPath
// volume when E2E_COVERAGE=true, or an empty string otherwise.
func coverageVolumeYAML() string {
	if os.Getenv("E2E_COVERAGE") != "true" {
		return ""
	}
	return `- name: coverdata
        hostPath:
          path: /coverdata
          type: DirectoryOrCreate`
}
