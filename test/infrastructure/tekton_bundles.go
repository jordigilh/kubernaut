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
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// Tekton Bundle Infrastructure for E2E Testing
//
// This file provides functions to build Tekton Pipeline bundles as OCI images
// and load them into Kind clusters for E2E testing.
//
// **Design Pattern**: Local bundle building (no external registry required)
// - Build bundles using `tkn bundle push` to local registry
// - Load bundle images into Kind cluster using `kind load image-archive`
// - Reference bundles in WorkflowExecution specs using localhost/ prefix
//
// **Benefits**:
// - No external dependencies (works offline)
// - Faster than pushing/pulling from remote registry
// - Consistent with existing E2E image loading patterns
// - Bundles persist in Kind cluster for all test runs

const (
	// TestBundleRegistry is the local registry prefix for test bundles
	// Using localhost/ ensures Kind uses locally loaded images
	TestBundleRegistry = "localhost/kubernaut-test-workflows"

	// TestBundleVersion is the version tag for test bundles
	TestBundleVersion = "e2e-test"
)

// BuildAndLoadTestBundles builds Tekton Pipeline bundles and loads them into Kind cluster
// This creates OCI bundle images for:
// - hello-world pipeline (successful execution test)
// - failing pipeline (failure handling test)
//
// The bundles are built locally and loaded into the Kind cluster, no external registry needed.
func BuildAndLoadTestBundles(clusterName, kubeconfigPath string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "\n📦 Building Tekton Pipeline bundles for E2E tests...\n")

	// Find project root for accessing test fixtures
	projectRoot, err := findProjectRoot()
	if err != nil {
		return fmt.Errorf("failed to find project root: %w", err)
	}

	fixturesDir := filepath.Join(projectRoot, "test", "fixtures", "tekton")

	// Build hello-world bundle
	helloWorldBundle := fmt.Sprintf("%s/hello-world:%s", TestBundleRegistry, TestBundleVersion)
	if err := buildTektonBundle(
		helloWorldBundle,
		filepath.Join(fixturesDir, "hello-world-pipeline.yaml"),
		output,
	); err != nil {
		return fmt.Errorf("failed to build hello-world bundle: %w", err)
	}

	// Build failing pipeline bundle
	failingBundle := fmt.Sprintf("%s/failing:%s", TestBundleRegistry, TestBundleVersion)
	if err := buildTektonBundle(
		failingBundle,
		filepath.Join(fixturesDir, "failing-pipeline.yaml"),
		output,
	); err != nil {
		return fmt.Errorf("failed to build failing bundle: %w", err)
	}

	// Load bundles into Kind cluster
	_, _ = fmt.Fprintf(output, "\n📥 Loading bundles into Kind cluster...\n")
	if err := loadBundleToKind(clusterName, helloWorldBundle, output); err != nil {
		return fmt.Errorf("failed to load hello-world bundle: %w", err)
	}
	if err := loadBundleToKind(clusterName, failingBundle, output); err != nil {
		return fmt.Errorf("failed to load failing bundle: %w", err)
	}

	_, _ = fmt.Fprintf(output, "✅ Tekton bundles built and loaded successfully\n")
	_, _ = fmt.Fprintf(output, "   • %s\n", helloWorldBundle)
	_, _ = fmt.Fprintf(output, "   • %s\n", failingBundle)
	return nil
}

// buildTektonBundle builds a Tekton Pipeline bundle using tkn CLI
// The bundle is pushed to a local OCI registry (localhost/)
func buildTektonBundle(bundleRef, pipelineYAML string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "  Building bundle: %s\n", bundleRef)

	// Verify tkn CLI is installed
	if _, err := exec.LookPath("tkn"); err != nil {
		return fmt.Errorf("tkn CLI not found in PATH - install from https://tekton.dev/docs/cli/: %w", err)
	}

	// Verify pipeline YAML exists
	if _, err := os.Stat(pipelineYAML); err != nil {
		return fmt.Errorf("pipeline YAML not found at %s: %w", pipelineYAML, err)
	}

	// Build bundle using tkn CLI
	// Note: tkn bundle push creates an OCI image in the local container registry
	// The --override flag does not exist in tkn CLI - bundles are naturally overwritable
	cmd := exec.Command("tkn", "bundle", "push", bundleRef,
		"-f", pipelineYAML,
	)
	cmd.Stdout = output
	cmd.Stderr = output

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tkn bundle push failed for %s: %w", bundleRef, err)
	}

	return nil
}

// loadBundleToKind loads a Tekton bundle OCI image into the Kind cluster
// Uses the same pattern as other E2E image loading: podman save + kind load image-archive
func loadBundleToKind(clusterName, bundleRef string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "  Loading bundle: %s\n", bundleRef)

	// Create temp file for image archive
	tmpFile, err := os.CreateTemp("", "tekton-bundle-*.tar")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile.Name()) }()
	_ = tmpFile.Close()

	// Save bundle image to tar file
	// Note: Bundles are OCI images, so we use podman save
	saveCmd := exec.Command("podman", "save", "-o", tmpFile.Name(), bundleRef)
	saveCmd.Stdout = output
	saveCmd.Stderr = output
	if err := saveCmd.Run(); err != nil {
		return fmt.Errorf("failed to save bundle %s: %w", bundleRef, err)
	}

	// Load bundle archive into Kind cluster
	loadCmd := exec.Command("kind", "load", "image-archive", tmpFile.Name(), "--name", clusterName)
	loadCmd.Stdout = output
	loadCmd.Stderr = output
	if err := loadCmd.Run(); err != nil {
		return fmt.Errorf("failed to load bundle %s into Kind: %w", bundleRef, err)
	}

	return nil
}

// GetTestBundleRef returns the OCI bundle reference for a test workflow
// This is used by E2E tests to reference the correct bundle image
func GetTestBundleRef(workflowName string) string {
	return fmt.Sprintf("%s/%s:%s", TestBundleRegistry, workflowName, TestBundleVersion)
}
