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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/shared/contenthash"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// SeedWorkflowsViaDirectCRDCreation registers workflow fixtures directly via
// the Kubernetes API and stamps .status.workflowId/.status.contentHash/
// .status.catalogStatus using the exact same local computation AuthWebhook
// uses (pkg/shared/contenthash) -- for integration suites that intentionally
// run envtest WITHOUT a live AuthWebhook instance (#1661 Phase 55). Any
// consumer that reads workflow_id/content_hash off the CRD (KubernautAgent's
// informer-backed workflow catalog -- internal/kubernautagent/workflowcatalog,
// #1677 Phase 2g/DD-WORKFLOW-019 -- reads them directly off CRD status and
// never computes them itself) needs this status populated some other way to
// treat a seeded workflow as usable.
//
// Mirrors AuthWebhook's own async status-patch (registerWorkflowStatusAsync,
// pkg/authwebhook/remediationworkflow_handler.go: Get + mutate +
// Status().Update() under retry.RetryOnConflict) so a seeded workflow is
// indistinguishable, from DS's perspective, from one AuthWebhook actually
// admitted. Use SeedWorkflowsViaKubectlApply instead wherever a real
// AuthWebhook is deployed (e.g. full E2E clusters) -- that path exercises the
// real admission pipeline and is the higher-fidelity choice when available.
func SeedWorkflowsViaDirectCRDCreation(ctx context.Context, k8sClient client.Client, namespace string, workflows []WorkflowSeedSpec, output io.Writer) (map[string]string, error) {
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "🌱 Seeding %d workflows via direct CRD creation (no AuthWebhook)\n", len(workflows))
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	workflowUUIDs := make(map[string]string, len(workflows))

	for _, wf := range workflows {
		workflowID, name, err := seedOneWorkflowViaDirectCRDCreation(ctx, k8sClient, namespace, wf, output)
		if err != nil {
			return nil, err
		}
		workflowUUIDs[fmt.Sprintf("%s:%s", name, wf.Environment)] = workflowID
	}

	_, _ = fmt.Fprintf(output, "✅ All workflows seeded via direct CRD creation (%d UUIDs captured)\n\n", len(workflowUUIDs))
	return workflowUUIDs, nil
}

func seedOneWorkflowViaDirectCRDCreation(ctx context.Context, k8sClient client.Client, namespace string, wf WorkflowSeedSpec, output io.Writer) (workflowID, name string, err error) {
	content, err := readWorkflowFixtureContent(wf.FixtureDir)
	if err != nil {
		return "", "", fmt.Errorf("read fixture %s: %w", wf.FixtureDir, err)
	}

	workflowID, name, err = seedWorkflowContentViaDirectCRDCreation(ctx, k8sClient, namespace, content)
	if err != nil {
		return "", "", err
	}

	_, _ = fmt.Fprintf(output, "  ✅ %s → %s (status patched locally, no AuthWebhook)\n", name, workflowID)
	return workflowID, name, nil
}

// seedWorkflowContentViaDirectCRDCreation is the fixture-agnostic core of
// SeedWorkflowsViaDirectCRDCreation: given already-marshaled RemediationWorkflow
// CRD YAML, it creates (or reuses) the CRD and stamps its status the same way
// AuthWebhook would. Shared by the fixture-directory-based seeder above and the
// inline-content-based exported variants below (#1661 Phase 55b).
func seedWorkflowContentViaDirectCRDCreation(ctx context.Context, k8sClient client.Client, namespace, content string) (workflowID, name string, err error) {
	rw := &rwv1alpha1.RemediationWorkflow{}
	if err := yaml.Unmarshal([]byte(content), rw); err != nil {
		return "", "", fmt.Errorf("unmarshal workflow content: %w", err)
	}
	rw.Namespace = namespace

	if createErr := k8sClient.Create(ctx, rw); createErr != nil {
		if !apierrors.IsAlreadyExists(createErr) {
			return "", "", fmt.Errorf("create RemediationWorkflow %s: %w", rw.Name, createErr)
		}
		// Already exists (e.g. re-registered with identical content within the
		// same spec, or re-seeded in a long-lived process) -- fetch the live
		// object so the status patch below operates on a fresh copy.
		if getErr := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: rw.Name}, rw); getErr != nil {
			return "", "", fmt.Errorf("get existing RemediationWorkflow %s: %w", rw.Name, getErr)
		}
	}

	clean, err := contenthash.MarshalCleanCRDContent(rw)
	if err != nil {
		return "", "", fmt.Errorf("marshal content for %s: %w", rw.Name, err)
	}
	contentHashHex := contenthash.ComputeContentHash(string(clean))
	workflowID = contenthash.DeterministicUUID(contentHashHex)
	name = rw.Name

	updateErr := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if getErr := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, rw); getErr != nil {
			return getErr
		}
		now := metav1.Now()
		rw.Status.WorkflowID = workflowID
		rw.Status.ContentHash = contentHashHex
		rw.Status.CatalogStatus = sharedtypes.CatalogStatusActive
		rw.Status.RegisteredBy = "test-infrastructure-seeder"
		rw.Status.RegisteredAt = &now
		return k8sClient.Status().Update(ctx, rw)
	})
	if updateErr != nil {
		return "", "", fmt.Errorf("status update for %s: %w", name, updateErr)
	}

	return workflowID, name, nil
}

// SeedWorkflowContentViaDirectCRDCreation registers a single workflow whose CRD
// YAML is already available inline (e.g. via testutil.MarshalWorkflowCRD),
// stamping .status.workflowId/.status.contentHash/.status.catalogStatus the same
// way AuthWebhook would. Use when the caller already has a controller-runtime
// client.Client (e.g. test/e2e/fleet, which is envtest/client-wired) and wants
// to seed one workflow at a time without a fixture directory (#1661 Phase 55b).
func SeedWorkflowContentViaDirectCRDCreation(ctx context.Context, k8sClient client.Client, namespace, content string, output io.Writer) (string, error) {
	workflowID, name, err := seedWorkflowContentViaDirectCRDCreation(ctx, k8sClient, namespace, content)
	if err != nil {
		return "", err
	}
	_, _ = fmt.Fprintf(output, "  ✅ %s → %s (status patched locally, no AuthWebhook)\n", name, workflowID)
	return workflowID, nil
}

// SeedWorkflowsViaDirectCRDCreationFromKubeconfig is a drop-in,
// kubeconfig-path-signature-compatible replacement for
// SeedWorkflowsViaKubectlApply, for E2E suites that intentionally run
// without a live AuthWebhook (kubernautagent, aianalysis, apifrontend --
// #1661 Phase 55 discovered gap: SeedWorkflowsViaKubectlApply's wait on
// .status.workflowId can never resolve for these suites, since nothing but
// AuthWebhook's admission handler ever populates it). Builds a throwaway
// client.Client from kubeconfigPath and delegates to
// SeedWorkflowsViaDirectCRDCreation. Suites that already deploy a real
// AuthWebhook (fullpipeline, fleet, authwebhook's own E2E) should keep using
// SeedWorkflowsViaKubectlApply -- it exercises the real admission pipeline
// and is the higher-fidelity choice when available.
func SeedWorkflowsViaDirectCRDCreationFromKubeconfig(ctx context.Context, kubeconfigPath, namespace string, workflows []WorkflowSeedSpec, output io.Writer) (map[string]string, error) {
	k8sClient, err := NewKubeconfigWorkflowClient(kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("build workflow client from kubeconfig: %w", err)
	}
	return SeedWorkflowsViaDirectCRDCreation(ctx, k8sClient, namespace, workflows, output)
}

// NewKubeconfigWorkflowClient builds a minimal controller-runtime client.Client
// (RemediationWorkflow scheme only) from a kubeconfig path, for E2E suites that
// only carry a kubeconfig string (e.g. test/e2e/datastorage, which talks to a
// real Kind cluster via kubectl subprocess calls and has no pre-built
// client.Client) but need direct CRD creation to seed workflows without a live
// AuthWebhook (#1661 Phase 55b).
func NewKubeconfigWorkflowClient(kubeconfigPath string) (client.Client, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("build rest.Config from kubeconfig %s: %w", kubeconfigPath, err)
	}

	scheme := runtime.NewScheme()
	if err := rwv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("register RemediationWorkflow scheme: %w", err)
	}

	c, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("create controller-runtime client: %w", err)
	}
	return c, nil
}
