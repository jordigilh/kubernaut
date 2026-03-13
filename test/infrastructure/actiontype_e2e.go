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
	"strings"
	"time"
)

// actionTypeDef holds the minimal fields needed to create an ActionType CR.
type actionTypeDef struct {
	MetadataName string
	SpecName     string
	What         string
	WhenToUse    string
}

// e2eActionTypes is the union of all action types referenced by E2E test workflows.
// BR-WORKFLOW-007: ActionType CRD lifecycle management.
var e2eActionTypes = []actionTypeDef{
	{MetadataName: "delete-pod", SpecName: "DeletePod", What: "Delete one or more specific pods without waiting for graceful termination.", WhenToUse: "Pods are stuck in a terminal state (Terminating, Unknown) and cannot be restarted through normal means."},
	{MetadataName: "drain-node", SpecName: "DrainNode", What: "Drain and cordon a Kubernetes node, evicting all pods and preventing new scheduling.", WhenToUse: "Root cause is a node-level issue affecting multiple workloads on the node."},
	{MetadataName: "fix-certificate", SpecName: "FixCertificate", What: "Recreate a missing or corrupted CA Secret backing a cert-manager ClusterIssuer.", WhenToUse: "A cert-manager Certificate is stuck in NotReady because the CA Secret has been deleted or corrupted."},
	{MetadataName: "increase-memory-limits", SpecName: "IncreaseMemoryLimits", What: "Increase memory resource limits on containers.", WhenToUse: "OOM kills are caused by memory limits being too low relative to the workload actual requirements."},
	{MetadataName: "restart-deployment", SpecName: "RestartDeployment", What: "Perform a rolling restart of all pods in a workload.", WhenToUse: "Root cause is a workload-wide state issue affecting all or most pods."},
	{MetadataName: "restart-pod", SpecName: "RestartPod", What: "Kill and recreate one or more pods.", WhenToUse: "Root cause is a transient runtime state issue that a fresh process would resolve."},
	{MetadataName: "rollback-deployment", SpecName: "RollbackDeployment", What: "Revert a deployment to its previous stable revision.", WhenToUse: "Root cause is a recent deployment that introduced a regression."},
	{MetadataName: "scale-replicas", SpecName: "ScaleReplicas", What: "Horizontally scale a workload by adjusting the replica count.", WhenToUse: "Root cause is insufficient capacity to handle current load."},
}

// SeedE2EActionTypes creates the ActionType CRs required by E2E test workflows.
// Must be called AFTER CRDs are installed and the AuthWebhook is deployed, but
// BEFORE SeedWorkflowsInDataStorage — the AW webhook registers each AT in the DB,
// satisfying the action_type_taxonomy FK constraint for workflow registration.
func SeedE2EActionTypes(kubeconfigPath, namespace string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "🏷️  Seeding %d E2E ActionType CRDs in %s\n", len(e2eActionTypes), namespace)
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	for _, at := range e2eActionTypes {
		yaml := buildActionTypeYAML(at, namespace)

		cmd := exec.Command("kubectl", "apply",
			"--kubeconfig", kubeconfigPath,
			"-f", "-")
		cmd.Stdin = strings.NewReader(yaml)

		cmdOutput, err := cmd.CombinedOutput()
		if err != nil {
			_, _ = fmt.Fprintf(output, "  ❌ %s: %s\n", at.SpecName, cmdOutput)
			return fmt.Errorf("failed to apply ActionType %s: %w", at.SpecName, err)
		}
		_, _ = fmt.Fprintf(output, "  ✅ %s\n", at.SpecName)
	}

	_, _ = fmt.Fprintf(output, "\n⏳ Waiting for ActionTypes to register in DataStorage...\n")
	for _, at := range e2eActionTypes {
		cmd := exec.Command("kubectl", "wait",
			"--kubeconfig", kubeconfigPath,
			"--for=jsonpath={.status.registered}=true",
			fmt.Sprintf("actiontype/%s", at.MetadataName),
			"-n", namespace,
			fmt.Sprintf("--timeout=%ds", int(60*time.Second/time.Second)))

		if waitOut, err := cmd.CombinedOutput(); err != nil {
			_, _ = fmt.Fprintf(output, "  ⚠️  %s: not registered (timeout): %s\n", at.SpecName, waitOut)
			return fmt.Errorf("ActionType %s did not register within timeout: %w", at.SpecName, err)
		}
		_, _ = fmt.Fprintf(output, "  ✅ %s registered\n", at.SpecName)
	}

	_, _ = fmt.Fprintf(output, "✅ All E2E ActionTypes seeded and registered\n\n")
	return nil
}

func buildActionTypeYAML(at actionTypeDef, namespace string) string {
	return fmt.Sprintf(`apiVersion: kubernaut.ai/v1alpha1
kind: ActionType
metadata:
  name: %s
  namespace: %s
spec:
  name: %s
  description:
    what: %q
    whenToUse: %q
`, at.MetadataName, namespace, at.SpecName, at.What, at.WhenToUse)
}
