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
)

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Shared Kind Cluster Helpers - Reusable across all E2E test services
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// ExtraMount represents a Kind cluster extraMount configuration
type ExtraMount struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// CreateKindClusterWithExtraMounts creates a Kind cluster with dynamically added extraMounts
//
// This is the AUTHORITATIVE shared helper for all E2E services that need custom mounts.
//
// Parameters:
//   - clusterName: Name of the Kind cluster (e.g., "notification-e2e")
//   - kubeconfigPath: Path where kubeconfig will be written
//   - baseConfigPath: Path to base Kind YAML config (relative to workspace root)
//   - extraMounts: Slice of additional mounts to add to control-plane node
//   - writer: Output writer for logging
//
// Example:
//
//	mounts := []infrastructure.ExtraMount{
//	    {HostPath: "/Users/me/.kubernaut/e2e-notifications", ContainerPath: "/tmp/e2e-notifications", ReadOnly: false},
//	    {HostPath: "./coverdata", ContainerPath: "/coverdata", ReadOnly: false},
//	}
//	err := infrastructure.CreateKindClusterWithExtraMounts("notification-e2e", kubeconfig, "test/infrastructure/kind-notification-config.yaml", mounts, writer)
//
// Benefits over service-specific implementations:
//   - ✅ Single source of truth for Kind cluster creation with mounts
//   - ✅ Consistent YAML manipulation logic
//   - ✅ Reusable across Notification, Gateway, WorkflowExecution, etc.
//   - ✅ Easier to maintain and test
func CreateKindClusterWithExtraMounts(
	clusterName string,
	kubeconfigPath string,
	baseConfigPath string,
	extraMounts []ExtraMount,
	writer io.Writer,
) error {
	// 0. Validate Kind version (v0.30.x required for E2E tests)
	versionCmd := exec.Command("kind", "version")
	versionOutput, err := versionCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get kind version: %w", err)
	}
	versionStr := string(versionOutput)
	
	// Extract version (format: "kind v0.30.0 go1.25.0 darwin/arm64")
	if !strings.Contains(versionStr, "kind v0.30.") {
		_, _ = fmt.Fprintf(writer, "  ⚠️  WARNING: Unexpected Kind version detected\n")
		_, _ = fmt.Fprintf(writer, "     Current: %s", versionStr)
		_, _ = fmt.Fprintf(writer, "     Expected: kind v0.30.x\n")
		_, _ = fmt.Fprintf(writer, "     Install: go install sigs.k8s.io/kind@v0.30.0\n")
		return fmt.Errorf("kind version mismatch: expected v0.30.x, got: %s", versionStr)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Kind version validated: %s", versionStr)

	// 1. Find workspace root
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// 2. Read base Kind config
	fullConfigPath := filepath.Join(workspaceRoot, baseConfigPath)
	configData, err := os.ReadFile(fullConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read Kind config %s: %w", fullConfigPath, err)
	}

	// 3. Generate extraMounts YAML
	extraMountsYAML := generateExtraMountsYAML(extraMounts)

	// 4. Insert extraMounts into config (before kubeadmConfigPatches)
	configStr := string(configData)
	var updatedConfig string

	// Try to insert before kubeadmConfigPatches (most common insertion point)
	if strings.Contains(configStr, "  kubeadmConfigPatches:") {
		updatedConfig = strings.Replace(configStr, "  kubeadmConfigPatches:", extraMountsYAML+"\n  kubeadmConfigPatches:", 1)
	} else {
		// Fallback: Insert after control-plane node definition (before next role or end)
		// This handles configs without kubeadmConfigPatches
		updatedConfig = insertExtraMountsAfterControlPlane(configStr, extraMountsYAML)
	}

	// 5. Write temporary config file
	tmpConfig, err := os.CreateTemp("", fmt.Sprintf("kind-%s-*.yaml", clusterName))
	if err != nil {
		return fmt.Errorf("failed to create temp config: %w", err)
	}
	defer func() { _ = os.Remove(tmpConfig.Name()) }()

	if _, err := tmpConfig.WriteString(updatedConfig); err != nil {
		return fmt.Errorf("failed to write temp config: %w", err)
	}
	if err := tmpConfig.Close(); err != nil {
		return fmt.Errorf("failed to close temp config: %w", err)
	}

	// 6. Log mount information
	_, _ = fmt.Fprintf(writer, "   📦 Kind cluster with %d extraMount(s):\n", len(extraMounts))
	for _, mount := range extraMounts {
		readOnlyStr := ""
		if mount.ReadOnly {
			readOnlyStr = " (read-only)"
		}
		_, _ = fmt.Fprintf(writer, "      %s → %s%s\n", mount.HostPath, mount.ContainerPath, readOnlyStr)
	}

	// 7. Create Kind cluster
	cmd := exec.Command("kind", "create", "cluster",
		"--name", clusterName,
		"--config", tmpConfig.Name(),
		"--kubeconfig", kubeconfigPath)
	cmd.Stdout = writer
	cmd.Stderr = writer

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	return nil
}

// generateExtraMountsYAML converts ExtraMount slice to YAML format
func generateExtraMountsYAML(mounts []ExtraMount) string {
	if len(mounts) == 0 {
		return ""
	}

	var yaml strings.Builder
	yaml.WriteString("  extraMounts:")

	for _, mount := range mounts {
		yaml.WriteString(fmt.Sprintf("\n  - hostPath: %s", mount.HostPath))
		yaml.WriteString(fmt.Sprintf("\n    containerPath: %s", mount.ContainerPath))
		yaml.WriteString(fmt.Sprintf("\n    readOnly: %t", mount.ReadOnly))
	}

	return yaml.String()
}

// insertExtraMountsAfterControlPlane inserts extraMounts after control-plane role definition
// This is a fallback for configs without kubeadmConfigPatches
func insertExtraMountsAfterControlPlane(config, extraMountsYAML string) string {
	lines := strings.Split(config, "\n")
	var result []string
	inserted := false

	for i, line := range lines {
		result = append(result, line)

		// Look for "- role: control-plane" followed by next major section or end
		if !inserted && strings.Contains(line, "- role: control-plane") {
			// Find next role or end of nodes section
			for j := i + 1; j < len(lines); j++ {
				nextLine := lines[j]
				// Insert before next role or if we hit end of indented content
				if strings.HasPrefix(nextLine, "- role:") || (strings.TrimSpace(nextLine) != "" && !strings.HasPrefix(nextLine, "  ")) {
					// Insert extraMounts before this line
					mountLines := strings.Split(extraMountsYAML, "\n")
					result = append(result, mountLines...)
					inserted = true
					break
				}
			}

			// If we reached end without finding next role, append to end
			if !inserted {
				mountLines := strings.Split(extraMountsYAML, "\n")
				result = append(result, mountLines...)
				inserted = true
			}
		}
	}

	return strings.Join(result, "\n")
}

// CreateHostDirectoryIfNeeded creates a directory on the host if it doesn't exist
// This is useful for extraMounts that require pre-existing directories
func CreateHostDirectoryIfNeeded(path string, perm os.FileMode, writer io.Writer) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_, _ = fmt.Fprintf(writer, "   📁 Creating directory: %s\n", path)
		if err := os.MkdirAll(path, perm); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}
	} else {
		_, _ = fmt.Fprintf(writer, "   ✅ Directory already exists: %s\n", path)
	}
	return nil
}

// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// PHASE 1 REFACTORING: Unified Kind Cluster Creation
// Authority: docs/handoff/TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

// KindClusterOptions configures Kind cluster creation behavior
type KindClusterOptions struct {
	// ClusterName is the name of the Kind cluster (e.g., "gateway-e2e")
	ClusterName string

	// KubeconfigPath is where the kubeconfig will be written
	KubeconfigPath string

	// ConfigPath is the path to the Kind config YAML (relative to workspace root)
	// Example: "test/infrastructure/kind-gateway-config.yaml"
	ConfigPath string

	// WaitTimeout is the duration to wait for cluster readiness (default: "60s")
	WaitTimeout string

	// ReuseExisting skips creation if cluster already exists
	ReuseExisting bool

	// DeleteExisting deletes existing cluster before creating new one
	DeleteExisting bool

	// CleanupOrphanedContainers removes leftover Podman containers (useful on macOS)
	CleanupOrphanedContainers bool

	// UsePodman sets KIND_EXPERIMENTAL_PROVIDER=podman
	UsePodman bool

	// ProjectRootAsWorkingDir sets working directory to project root (for ./coverdata resolution)
	ProjectRootAsWorkingDir bool
}

// CreateKindClusterWithConfig creates a Kind cluster with the specified configuration
//
// This is the SINGLE AUTHORITATIVE function for all E2E test Kind cluster creation.
// It consolidates patterns from 8+ duplicate functions across the codebase.
//
// Benefits:
//   - ✅ Single source of truth for cluster creation
//   - ✅ Consistent error handling across all E2E tests
//   - ✅ Easier to add features (e.g., coverage support, Podman cleanup)
//   - ✅ Reduces code by ~500 lines across the codebase
//
// Example Usage:
//
//	opts := infrastructure.KindClusterOptions{
//	    ClusterName:    "gateway-e2e",
//	    KubeconfigPath: "/tmp/gateway-kubeconfig",
//	    ConfigPath:     "test/infrastructure/kind-gateway-config.yaml",
//	    WaitTimeout:    "5m",
//	    DeleteExisting: true,
//	    UsePodman:      true,
//	    ProjectRootAsWorkingDir: true,
//	}
//	err := infrastructure.CreateKindClusterWithConfig(opts, writer)
//
// Authority: docs/handoff/TEST_INFRASTRUCTURE_REFACTORING_TRIAGE_JAN07.md (Phase 1)
func CreateKindClusterWithConfig(opts KindClusterOptions, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "🔧 Creating Kind cluster: %s\n", opts.ClusterName)

	// 0. Validate Kind version (v0.30.x required for E2E tests)
	versionCmd := exec.Command("kind", "version")
	versionOutput, err := versionCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to get kind version: %w", err)
	}
	versionStr := string(versionOutput)
	
	// Extract version (format: "kind v0.30.0 go1.25.0 darwin/arm64")
	if !strings.Contains(versionStr, "kind v0.30.") {
		_, _ = fmt.Fprintf(writer, "  ⚠️  WARNING: Unexpected Kind version detected\n")
		_, _ = fmt.Fprintf(writer, "     Current: %s", versionStr)
		_, _ = fmt.Fprintf(writer, "     Expected: kind v0.30.x\n")
		_, _ = fmt.Fprintf(writer, "     Install: go install sigs.k8s.io/kind@v0.30.0\n")
		return fmt.Errorf("kind version mismatch: expected v0.30.x, got: %s", versionStr)
	}
	_, _ = fmt.Fprintf(writer, "  ✅ Kind version validated: %s", versionStr)

	// 1. Check if cluster already exists
	checkCmd := exec.Command("kind", "get", "clusters")
	checkOutput, _ := checkCmd.CombinedOutput()
	clusterExists := strings.Contains(string(checkOutput), opts.ClusterName)

	if clusterExists {
		if opts.ReuseExisting {
			_, _ = fmt.Fprintf(writer, "  ℹ️  Cluster %s already exists, reusing...\n", opts.ClusterName)
			return exportKubeconfigIfNeeded(opts.ClusterName, opts.KubeconfigPath, writer)
		}
		if opts.DeleteExisting {
			_, _ = fmt.Fprintf(writer, "  ⚠️  Cluster already exists, deleting...\n")
			delCmd := exec.Command("kind", "delete", "cluster", "--name", opts.ClusterName)
			if output, err := delCmd.CombinedOutput(); err != nil {
				_, _ = fmt.Fprintf(writer, "  ⚠️  Failed to delete existing cluster: %s\n", output)
			}
		}
	}

	// 2. Clean up orphaned Podman containers (macOS fix)
	if opts.CleanupOrphanedContainers {
		_, _ = fmt.Fprintln(writer, "  🧹 Cleaning up any leftover Podman containers...")
		// Only control-plane node (single-node clusters for resource efficiency)
		cleanupCmd := exec.Command("podman", "rm", "-f", opts.ClusterName+"-control-plane")
		_ = cleanupCmd.Run() // Ignore errors - container may not exist
	}

	// 3. Resolve config path relative to workspace root
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}
	absoluteConfigPath := filepath.Join(workspaceRoot, opts.ConfigPath)
	if _, err := os.Stat(absoluteConfigPath); os.IsNotExist(err) {
		return fmt.Errorf("kind config file not found: %s", absoluteConfigPath)
	}

	_, _ = fmt.Fprintf(writer, "  📋 Using Kind config: %s\n", absoluteConfigPath)

	// 4. Ensure kubeconfig directory exists
	kubeconfigDir := filepath.Dir(opts.KubeconfigPath)
	if err := os.MkdirAll(kubeconfigDir, 0755); err != nil {
		return fmt.Errorf("failed to create kubeconfig directory: %w", err)
	}

	// 5. Remove any leftover kubeconfig lock file
	lockFile := opts.KubeconfigPath + ".lock"
	_ = os.Remove(lockFile) // Ignore errors - file may not exist

	// 6. Build Kind create command
	waitTimeout := opts.WaitTimeout
	if waitTimeout == "" {
		waitTimeout = "60s"
	}

	cmd := exec.Command("kind", "create", "cluster",
		"--name", opts.ClusterName,
		"--config", absoluteConfigPath,
		"--kubeconfig", opts.KubeconfigPath,
		"--wait", waitTimeout)

	cmd.Stdout = writer
	cmd.Stderr = writer

	// 7. Set working directory to project root if requested (for ./coverdata resolution)
	if opts.ProjectRootAsWorkingDir {
		cmd.Dir = workspaceRoot
	}

	// 8. Set Podman provider if requested
	if opts.UsePodman {
		cmd.Env = append(os.Environ(), "KIND_EXPERIMENTAL_PROVIDER=podman")
	}

	// 9. Create cluster
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kind create cluster failed: %w", err)
	}

	_, _ = fmt.Fprintln(writer, "  ✅ Kind cluster created successfully")

	// 10. Ensure /coverdata is world-writable inside Kind node (DD-TEST-007)
	// Defense-in-depth: even if the host directory is 0777, DirectoryOrCreate may
	// create a root-owned 0755 directory inside the Kind node. This ensures the
	// container user (UID 1001) can always write coverage data.
	if os.Getenv("E2E_COVERAGE") == "true" {
		ensureCoverdataWritableInKindNode(opts.ClusterName, writer)
	}

	// 11. Export kubeconfig explicitly (kind create --kubeconfig doesn't always work reliably)
	return exportKubeconfigIfNeeded(opts.ClusterName, opts.KubeconfigPath, writer)
}

// exportKubeconfigIfNeeded exports the kubeconfig for a Kind cluster
// This is a workaround for unreliable --kubeconfig flag behavior
func exportKubeconfigIfNeeded(clusterName, kubeconfigPath string, writer io.Writer) error {
	kubeconfigCmd := exec.Command("kind", "get", "kubeconfig", "--name", clusterName)
	kubeconfigOutput, err := kubeconfigCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Write kubeconfig to file
	if err := os.WriteFile(kubeconfigPath, kubeconfigOutput, 0600); err != nil {
		return fmt.Errorf("failed to write kubeconfig: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "  ✅ Kubeconfig exported to %s\n", kubeconfigPath)
	return nil
}

// ensureCoverdataWritableInKindNode ensures /coverdata inside the Kind node is
// world-writable (0777) so that any container user (e.g. UID 1001) can write
// coverage data. This is a defense-in-depth measure: if the hostPath volume's
// DirectoryOrCreate creates a root-owned directory, the container user would
// be unable to write without this fix.
func ensureCoverdataWritableInKindNode(clusterName string, writer io.Writer) {
	nodeName := clusterName + "-control-plane"

	// Try both runtimes (podman for local, docker for CI)
	for _, runtime := range []string{"podman", "docker"} {
		cmd := exec.Command(runtime, "exec", nodeName,
			"sh", "-c", "mkdir -p /coverdata && chmod 777 /coverdata")
		output, err := cmd.CombinedOutput()
		if err == nil {
			_, _ = fmt.Fprintf(writer, "  ✅ /coverdata set to 0777 inside Kind node via %s\n", runtime)
			return
		}
		_ = output // Suppress unused variable
	}
	_, _ = fmt.Fprintln(writer, "  ⚠️  Could not chmod /coverdata inside Kind node (non-fatal)")
}
