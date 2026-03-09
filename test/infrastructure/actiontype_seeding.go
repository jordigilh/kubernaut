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
	"os/exec"
	"path/filepath"
)

// SeedActionTypeCRDs applies all ActionType CRD YAML files from deploy/action-types/
// using kubectl apply. This must be called BEFORE seeding workflows, since workflows
// reference action types by name.
//
// BR-WORKFLOW-007: ActionType CRD lifecycle management.
// The AW webhook handles DS registration on CREATE (idempotent — NOOP if already active).
func SeedActionTypeCRDs(kubeconfigPath, workspaceRoot, namespace string, output io.Writer) error {
	actionTypesDir := filepath.Join(workspaceRoot, "deploy", "action-types")

	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "🏷️  Seeding ActionType CRDs from %s\n", actionTypesDir)
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	cmd := exec.Command("kubectl", "apply",
		"--kubeconfig", kubeconfigPath,
		"-n", namespace,
		"-f", actionTypesDir)
	cmd.Dir = workspaceRoot

	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		_, _ = fmt.Fprintf(output, "❌ ActionType CRD apply failed: %s\n", cmdOutput)
		return fmt.Errorf("kubectl apply action-types failed: %w", err)
	}

	_, _ = fmt.Fprintf(output, "%s", cmdOutput)
	_, _ = fmt.Fprintf(output, "✅ ActionType CRDs applied successfully\n\n")
	return nil
}
