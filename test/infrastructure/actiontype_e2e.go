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
	"context"
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
	{MetadataName: "fix-authorization-policy", SpecName: "FixAuthorizationPolicy", What: "Fix an overly restrictive Istio AuthorizationPolicy blocking legitimate traffic.", WhenToUse: "High deny rate is caused by a misconfigured AuthorizationPolicy."},
	{MetadataName: "fix-certificate", SpecName: "FixCertificate", What: "Recreate a missing or corrupted CA Secret backing a cert-manager ClusterIssuer.", WhenToUse: "A cert-manager Certificate is stuck in NotReady because the CA Secret has been deleted or corrupted."},
	{MetadataName: "increase-memory-limits", SpecName: "IncreaseMemoryLimits", What: "Increase memory resource limits on containers.", WhenToUse: "OOM kills are caused by memory limits being too low relative to the workload actual requirements."},
	{MetadataName: "restart-deployment", SpecName: "RestartDeployment", What: "Perform a rolling restart of all pods in a workload.", WhenToUse: "Root cause is a workload-wide state issue affecting all or most pods."},
	{MetadataName: "restart-pod", SpecName: "RestartPod", What: "Kill and recreate one or more pods.", WhenToUse: "Root cause is a transient runtime state issue that a fresh process would resolve."},
	{MetadataName: "rollback-deployment", SpecName: "RollbackDeployment", What: "Revert a deployment to its previous stable revision.", WhenToUse: "Root cause is a recent deployment that introduced a regression."},
	{MetadataName: "increase-cpu-limits", SpecName: "IncreaseCPULimits", What: "Increase CPU resource limits on containers.", WhenToUse: "CPU throttling is caused by CPU limits being too low relative to the workload actual requirements."},
	{MetadataName: "scale-replicas", SpecName: "ScaleReplicas", What: "Horizontally scale a workload by adjusting the replica count.", WhenToUse: "Root cause is insufficient capacity to handle current load."},
	{MetadataName: "reconfigure-resource", SpecName: "ReconfigureResource", What: "Reconfigure a Kubernetes resource spec to fix misconfiguration.", WhenToUse: "Root cause is a resource misconfiguration that can be corrected by updating spec fields."},
}

// SeedE2EActionTypes creates the ActionType CRs required by E2E test workflows,
// via the real AuthWebhook admission path (kubectl apply -> AW admits -> AW
// patches .status.registered=true locally, no DS round-trip as of #1661 Change
// 8d). Must be called AFTER CRDs are installed and AuthWebhook is deployed, but
// BEFORE seeding workflows (SeedWorkflowsViaKubectlApply et al.). Use this
// variant when the E2E suite already deploys AuthWebhook and the test wants to
// prove the real admission path works; otherwise use SeedActionTypesViaCRD,
// which has no AuthWebhook dependency.
func SeedE2EActionTypes(ctx context.Context, kubeconfigPath, namespace string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "🏷️  Seeding %d E2E ActionType CRDs in %s\n", len(e2eActionTypes), namespace)
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	for _, at := range e2eActionTypes {
		yaml := buildActionTypeYAML(at, namespace)

		cmd := exec.CommandContext(ctx, "kubectl", "apply",
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
		cmd := exec.CommandContext(ctx, "kubectl", "wait",
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

// SeedActionTypesViaCRD creates the ActionType CRs required by E2E/integration
// test workflows, directly against the K8s API -- with no dependency on
// AuthWebhook being deployed. Use this instead of SeedE2EActionTypes for
// suites that don't run AuthWebhook (e.g. Gateway, AIAnalysis, APIFrontend, KA,
// SignalProcessing, WorkflowExecution-bundles E2E/IT suites, which exercise
// their own component rather than AW's admission path): DataStorage's
// informer-backed cache (pkg/datastorage/workflowcache, #1661 Phase 28-30)
// observes the raw CRD directly and needs no admission-controller status patch
// to make it discoverable.
//
// #1661 Phase 52 (Change 9, discovered gap): the sole DS-catalog-facing
// replacement for SeedActionTypesViaAPI/SeedActionTypesViaAPIWithTLS, which
// call DataStorage's Postgres-backed POST /api/v1/action-types endpoint
// (removed in Phase 55).
func SeedActionTypesViaCRD(ctx context.Context, kubeconfigPath, namespace string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "🏷️  Seeding %d action types via direct CRD creation (no AuthWebhook)\n", len(e2eActionTypes))
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	for _, at := range e2eActionTypes {
		yaml := buildActionTypeYAML(at, namespace)

		cmd := exec.CommandContext(ctx, "kubectl", "apply",
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

	_, _ = fmt.Fprintf(output, "✅ All action types seeded as CRDs (%d types, no AuthWebhook dependency)\n\n", len(e2eActionTypes))
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
