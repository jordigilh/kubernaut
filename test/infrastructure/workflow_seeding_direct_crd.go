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
// run envtest + DataStorage WITHOUT a live AuthWebhook instance (#1661 Phase
// 55). DS's cache reads workflow_id/content_hash directly off CRD status; it
// never computes them itself (pkg/datastorage/workflowcache), so a workflow
// created without AuthWebhook admitting it needs this status populated some
// other way to be usable by tests.
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

	rw := &rwv1alpha1.RemediationWorkflow{}
	if err := yaml.Unmarshal([]byte(content), rw); err != nil {
		return "", "", fmt.Errorf("unmarshal fixture %s: %w", wf.FixtureDir, err)
	}
	rw.Namespace = namespace

	if createErr := k8sClient.Create(ctx, rw); createErr != nil {
		if !apierrors.IsAlreadyExists(createErr) {
			return "", "", fmt.Errorf("create RemediationWorkflow %s: %w", rw.Name, createErr)
		}
		// Already exists (e.g. re-seeded in the same long-lived process) -- fetch
		// the live object so the status patch below operates on a fresh copy.
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

	_, _ = fmt.Fprintf(output, "  ✅ %s → %s (status patched locally, no AuthWebhook)\n", name, workflowID)
	return workflowID, name, nil
}
