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
