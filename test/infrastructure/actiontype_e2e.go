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
		if err := applyActionTypeWithWebhookRetry(ctx, kubeconfigPath, at, yaml, output); err != nil {
			return err
		}
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
// their own component rather than AW's admission path).
//
// #1661 Phase 52 (Change 9, discovered gap): the sole DS-catalog-facing
// replacement for SeedActionTypesViaAPI/SeedActionTypesViaAPIWithTLS, which
// call DataStorage's Postgres-backed POST /api/v1/action-types endpoint
// (removed in Phase 55).
//
// #1661 discovered gap (this fix): listActionsFromCache (discovery_cache.go,
// Step 1 of the discovery protocol) only counts ActionTypes whose
// .status.catalogStatus == Active -- it is NOT enough for the raw CRD to
// exist, contrary to this function's original doc comment. In production,
// AuthWebhook's admission handler (actiontype_handler.go
// updateCRDStatusCreate) sets catalogStatus=Active as part of admitting the
// CRD; suites using this no-AuthWebhook seeding path must replicate that
// status patch themselves or Step 1 discovery silently returns zero action
// types for every filter, no matter how long callers poll.
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

		// Mirror AuthWebhook's updateCRDStatusCreate (actiontype_handler.go):
		// listActionsFromCache requires catalogStatus == Active, which kubectl
		// apply above never sets (no status: block in the YAML, and Active is
		// not the Kubernetes zero-value for the CatalogStatus enum).
		const statusPatch = `{"status":{"registered":true,"catalogStatus":"Active","registeredBy":"test-infrastructure-seeder"}}`
		patchCmd := exec.CommandContext(ctx, "kubectl", "patch",
			"--kubeconfig", kubeconfigPath,
			"actiontype", at.MetadataName,
			"-n", namespace,
			"--type=merge",
			"--subresource=status",
			"-p", statusPatch)
		if patchOutput, patchErr := patchCmd.CombinedOutput(); patchErr != nil {
			_, _ = fmt.Fprintf(output, "  ❌ %s status patch: %s\n", at.SpecName, patchOutput)
			return fmt.Errorf("failed to patch ActionType %s status to Active: %w", at.SpecName, patchErr)
		}
		_, _ = fmt.Fprintf(output, "  ✅ %s (status.catalogStatus=Active)\n", at.SpecName)
	}

	_, _ = fmt.Fprintf(output, "✅ All action types seeded as CRDs (%d types, no AuthWebhook dependency)\n\n", len(e2eActionTypes))
	return nil
}

// isTransientWebhookError reports whether err/output looks like a transient
// admission-webhook connectivity failure (Service endpoint not yet
// programmed, or briefly flapped out of rotation because #1985's DataStorage
// readiness gate made AuthWebhook's own /readyz momentarily fail under CI
// resource contention) rather than a genuine, permanent failure such as a
// malformed manifest or an admission rejection on the merits.
func isTransientWebhookError(combinedOutput string) bool {
	return strings.Contains(combinedOutput, "connect: connection refused") ||
		strings.Contains(combinedOutput, "failed calling webhook") ||
		strings.Contains(combinedOutput, "context deadline exceeded") ||
		strings.Contains(combinedOutput, "no endpoints available")
}

// applyActionTypeWithWebhookRetry applies one ActionType CR via kubectl,
// retrying on transient AuthWebhook admission-webhook connectivity errors.
//
// waitForDeploymentRollout("authwebhook") reports success the instant the
// Deployment's replica first passes its readiness probe, but the Service's
// own Endpoints/kube-proxy DNAT programming is a separate, asynchronous
// step -- and #1985's DataStorage readiness gate means AuthWebhook's /readyz
// can keep flapping under CI resource contention even after that first
// pass, briefly pulling it back out of the Service's endpoints. A single
// pre-flight endpoint check (see waitForServiceEndpointReady) closes the
// first race but not the second; retrying the actual apply call here
// tolerates both (confirmed recurring symptom: CI runs 31842923960,
// 31845258440, both "DeletePod: ... dial tcp ...:443: connect: connection
// refused" on the very first ActionType apply of the batch).
func applyActionTypeWithWebhookRetry(ctx context.Context, kubeconfigPath string, at actionTypeDef, yaml string, output io.Writer) error {
	const maxAttempts = 6
	const retryDelay = 5 * time.Second

	var lastOutput []byte
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		cmd := exec.CommandContext(ctx, "kubectl", "apply",
			"--kubeconfig", kubeconfigPath,
			"-f", "-")
		cmd.Stdin = strings.NewReader(yaml)

		cmdOutput, err := cmd.CombinedOutput()
		if err == nil {
			_, _ = fmt.Fprintf(output, "  ✅ %s\n", at.SpecName)
			return nil
		}

		lastOutput, lastErr = cmdOutput, err
		if !isTransientWebhookError(string(cmdOutput)) || attempt == maxAttempts {
			break
		}
		_, _ = fmt.Fprintf(output, "  ⚠️  %s: transient webhook error on attempt %d/%d, retrying in %s: %s\n",
			at.SpecName, attempt, maxAttempts, retryDelay, cmdOutput)
		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
			return fmt.Errorf("context cancelled while retrying ActionType %s apply: %w", at.SpecName, ctx.Err())
		}
	}

	_, _ = fmt.Fprintf(output, "  ❌ %s: %s\n", at.SpecName, lastOutput)
	return fmt.Errorf("failed to apply ActionType %s: %w", at.SpecName, lastErr)
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
