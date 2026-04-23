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
	"net/http"
	"os/exec"
	"strings"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	testauth "github.com/jordigilh/kubernaut/test/shared/auth"
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
	{MetadataName: "increase-cpu-limits", SpecName: "IncreaseCPULimits", What: "Increase CPU resource limits on containers.", WhenToUse: "CPU throttling is caused by CPU limits being too low relative to the workload actual requirements."},
	{MetadataName: "scale-replicas", SpecName: "ScaleReplicas", What: "Horizontally scale a workload by adjusting the replica count.", WhenToUse: "Root cause is insufficient capacity to handle current load."},
}

// SeedActionTypesViaAPI populates the action_type_taxonomy table by calling the
// DataStorage POST /api/v1/action-types endpoint for each standard action type.
// Idempotent: the API returns 200 (exists) if the action type is already present.
// Must be called AFTER DataStorage is healthy, BEFORE any workflow registration.
// DD-WORKFLOW-016: FK constraint for remediation_workflow_catalog.
func SeedActionTypesViaAPI(client *ogenclient.Client, writer io.Writer) error {
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(writer, "🏷️  Seeding %d action types via DataStorage API\n", len(e2eActionTypes))
	_, _ = fmt.Fprintf(writer, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	for _, at := range e2eActionTypes {
		req := &ogenclient.ActionTypeCreateRequest{
			Name: at.SpecName,
			Description: ogenclient.ActionTypeDescription{
				What:      at.What,
				WhenToUse: at.WhenToUse,
			},
			RegisteredBy: "test-infrastructure-seeder",
		}

		res, err := client.CreateActionType(ctx, req)
		if err != nil {
			_, _ = fmt.Fprintf(writer, "  ❌ %s: %v\n", at.SpecName, err)
			return fmt.Errorf("failed to seed action type %s via API: %w", at.SpecName, err)
		}

		switch r := res.(type) {
		case *ogenclient.CreateActionTypeCreated:
			_, _ = fmt.Fprintf(writer, "  ✅ %s (created)\n", at.SpecName)
		case *ogenclient.CreateActionTypeOK:
			_, _ = fmt.Fprintf(writer, "  ✅ %s (status: %s)\n", at.SpecName, r.Status)
		default:
			_, _ = fmt.Fprintf(writer, "  ✅ %s (ok)\n", at.SpecName)
		}
	}

	_, _ = fmt.Fprintf(writer, "✅ All action types seeded via DataStorage API (%d types)\n\n", len(e2eActionTypes))
	return nil
}

// SeedActionTypesViaAPIWithURL is a convenience wrapper that creates a temporary
// authenticated ogen client and delegates to SeedActionTypesViaAPI.
// Use when the caller has a DS URL + SA token but not a pre-built ogen client.
func SeedActionTypesViaAPIWithURL(dsURL, token string, timeout time.Duration, writer io.Writer) error {
	httpClient := &http.Client{
		Transport: testauth.NewServiceAccountTransport(token),
		Timeout:   timeout,
	}
	client, err := ogenclient.NewClient(dsURL, ogenclient.WithClient(httpClient))
	if err != nil {
		return fmt.Errorf("failed to create ogen client for action type seeding: %w", err)
	}
	return SeedActionTypesViaAPI(client, writer)
}

// SeedActionTypesViaAPIWithTLS is a TLS-aware convenience wrapper that creates a
// temporary authenticated ogen client with inter-service CA trust and delegates
// to SeedActionTypesViaAPI.
// Use in E2E tests where DataStorage serves HTTPS with a private CA.
//
// Issue #785: E2E HTTPS migration requires TLS-aware seeding.
func SeedActionTypesViaAPIWithTLS(dsURL, token, kubeconfigPath string, timeout time.Duration, writer io.Writer) error {
	tlsTransport, err := NewTLSAwareTransport(kubeconfigPath)
	if err != nil {
		return fmt.Errorf("failed to create TLS-aware transport for action type seeding: %w", err)
	}
	httpClient := &http.Client{
		Transport: testauth.NewServiceAccountTransportWithBase(token, tlsTransport),
		Timeout:   timeout,
	}
	client, err := ogenclient.NewClient(dsURL, ogenclient.WithClient(httpClient))
	if err != nil {
		return fmt.Errorf("failed to create ogen client for action type seeding: %w", err)
	}
	return SeedActionTypesViaAPI(client, writer)
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
