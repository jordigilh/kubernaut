/*
Copyright 2026 Jordi Gil.

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
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// JobDiagnosticsCommand is one kubectl invocation in the failure-diagnostics
// collection for a WorkflowExecution Job. Label names the artifact being
// collected (e.g. "pod-logs"); Args is the full kubectl argument list
// INCLUDING the leading --kubeconfig pair, so callers can execute it directly.
type JobDiagnosticsCommand struct {
	Label string
	Args  []string
}

// BuildWorkflowJobDiagnosticsCommands returns the kubectl commands that
// capture every piece of evidence needed to diagnose a failed
// WorkflowExecution Job (issue #2688): the Job spec, its pod logs across ALL
// Job-owned pods (job-name label selector, not `kubectl logs job/<name>`
// which only follows the latest pod), pod descriptions and the namespace
// events (BackoffLimitExceeded timeline). Pure function so the command shape
// is unit-testable without a cluster.
//
// Authority: Issue #2688, BR-INTEGRATION-054 (spoke remediation Jobs must be
// diagnosable from CI artifacts).
func BuildWorkflowJobDiagnosticsCommands(kubeconfigPath, jobName, namespace string) []JobDiagnosticsCommand {
	base := []string{"--kubeconfig", kubeconfigPath, "-n", namespace}
	return []JobDiagnosticsCommand{
		{
			Label: "job-spec",
			Args: append(append([]string{}, base...),
				"get", "job", jobName, "-o", "yaml"),
		},
		{
			Label: "pod-logs",
			Args: append(append([]string{}, base...),
				"logs", "-l", "job-name="+jobName,
				"--all-containers", "--timestamps", "--prefix", "--tail=5000"),
		},
		{
			Label: "pod-describe",
			Args: append(append([]string{}, base...),
				"describe", "pods", "-l", "job-name="+jobName),
		},
		{
			Label: "events",
			Args: append(append([]string{}, base...),
				"get", "events", "--sort-by=.lastTimestamp"),
		},
	}
}

// CaptureWorkflowJobDiagnostics runs every command from
// BuildWorkflowJobDiagnosticsCommands against the given cluster, streaming
// each artifact's output to writer (GinkgoWriter in E2E specs, so the
// evidence lands in the CI log) and persisting a copy under
// /tmp/kubernaut-must-gather/fleet-job-failures/<wfeName>/ for the suite's
// artifact upload. Returns the directory the artifacts were written to.
//
// It MUST be called at failure time -- the moment a Job's Status.Failed goes
// non-zero -- because the WFE controller's Cleanup deletes the Job and its
// pods as soon as the WFE reaches a terminal phase, long before the suite's
// end-of-run must-gather (issue #2688: that ordering left jobs/all-jobs.yaml
// empty and zero pod logs for two BackoffLimitExceeded spoke Jobs).
//
// Failures are non-fatal by design: diagnostics collection must never mask
// the assertion failure that triggered it.
func CaptureWorkflowJobDiagnostics(ctx context.Context, kubeconfigPath, wfeName, jobName, namespace string, writer io.Writer) string {
	dir := filepath.Join("/tmp", "kubernaut-must-gather", "fleet-job-failures", wfeName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		_, _ = fmt.Fprintf(writer, "⚠️  Failed to create job diagnostics directory %s: %v\n", dir, err)
		return dir
	}
	_, _ = fmt.Fprintf(writer, "\n📦 Capturing spoke Job diagnostics for %s (job=%s ns=%s) -> %s\n", wfeName, jobName, namespace, dir)
	for _, cmd := range BuildWorkflowJobDiagnosticsCommands(kubeconfigPath, jobName, namespace) {
		out, err := exec.CommandContext(ctx, "kubectl", cmd.Args...).CombinedOutput()
		_, _ = fmt.Fprintf(writer, "--- %s ---\n%s\n", cmd.Label, string(out))
		if err != nil {
			_, _ = fmt.Fprintf(writer, "(kubectl %s exited with: %v)\n", cmd.Label, err)
		}
		_ = os.WriteFile(filepath.Join(dir, cmd.Label+".txt"), out, 0o644) //nolint:gosec // diagnostics artifact, world-readable by design
	}
	_, _ = fmt.Fprintf(writer, "📦 Spoke Job diagnostics complete -> %s\n\n", dir)
	return dir
}
