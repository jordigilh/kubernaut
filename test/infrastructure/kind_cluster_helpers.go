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
	defer os.Remove(tmpConfig.Name())

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



