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

package workflow

import (
	"encoding/json"
	"fmt"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	atv1alpha1 "github.com/jordigilh/kubernaut/api/actiontype/v1alpha1"
	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/datastorage/models"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// crdActionTypeToEntry converts a cached ActionType CRD plus its
// caller-computed active-workflow count into models.ActionTypeEntry, the
// Step 1 discovery response shape (GET /api/v1/workflows/actions). The
// workflow count is passed in rather than computed here because it depends
// on the ActionType's sibling RemediationWorkflow CRDs in the cache, which
// this pure converter has no access to.
func crdActionTypeToEntry(at *atv1alpha1.ActionType, workflowCount int) models.ActionTypeEntry {
	return models.ActionTypeEntry{
		ActionType: at.Spec.Name,
		Description: models.ActionTypeDescription{
			What:          at.Spec.Description.What,
			WhenToUse:     at.Spec.Description.WhenToUse,
			WhenNotToUse:  at.Spec.Description.WhenNotToUse,
			Preconditions: at.Spec.Description.Preconditions,
		},
		WorkflowCount: workflowCount,
	}
}

// ========================================
// CRD -> MODELS CONVERTERS (Issue #1661 Change 6)
// ========================================
// Authority: DD-WORKFLOW-018. Lets the cache-backed Step 1/2 discovery path
// (ListActions, ListWorkflowsByActionType) reuse the existing filter/scoring
// predicates (cache_filter.go) and the existing models.RemediationWorkflow
// response shape without a Postgres round-trip.
// ========================================

// crdLabelsToMandatoryLabels maps a RemediationWorkflow CRD's mandatory
// labels 1:1 onto models.MandatoryLabels -- the two types share the same
// shape (severity/environment/component []string, priority string, cluster
// []string) by design (BR-FLEET-003, Issue #1511).
func crdLabelsToMandatoryLabels(l rwv1alpha1.RemediationWorkflowLabels) models.MandatoryLabels {
	return models.MandatoryLabels{
		Severity:    l.Severity,
		Component:   l.Component,
		Environment: l.Environment,
		Priority:    l.Priority,
		Cluster:     l.Cluster,
	}
}

// crdCustomLabelsToModel converts the CRD's single-value-per-key
// spec.customLabels into models.CustomLabels' map[string][]string shape
// (used by customLabelsBoost's containment check), wrapping each value in a
// one-element slice. The CRD schema only allows one value per key -- this
// does not lose any generality the CRD supports, it just matches the
// richer Postgres-catalog shape's calling convention.
func crdCustomLabelsToModel(m map[string]string) models.CustomLabels {
	out := models.NewCustomLabels()
	for k, v := range m {
		out[k] = []string{v}
	}
	return out
}

// crdDetectedLabelsToModel unmarshals a RemediationWorkflow CRD's raw
// spec.detectedLabels JSON into models.DetectedLabels. The CRD-native field
// names (e.g. "gitOpsManaged", "pdbProtected") are identical to
// sharedtypes.DetectedLabels' json tags that models.DetectedLabels aliases,
// so no field-by-field mapping is needed -- a direct json.Unmarshal suffices.
// nil input (author declared no detected labels) maps to the zero value.
func crdDetectedLabelsToModel(raw *apiextensionsv1.JSON) (models.DetectedLabels, error) {
	var out models.DetectedLabels
	if raw == nil || len(raw.Raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw.Raw, &out); err != nil {
		return out, fmt.Errorf("failed to unmarshal detectedLabels: %w", err)
	}
	return out, nil
}

// crdWorkflowToModel converts a cached RemediationWorkflow CRD into
// models.RemediationWorkflow for the discovery-protocol response shape
// (models.WorkflowDiscoveryEntry, built by convertWorkflowsToDiscoveryEntries
// in workflow_discovery_handlers.go).
//
// Catalog-only fields with no CRD equivalent (Owner, Maintainer, SchemaImage/
// SchemaDigest, PreviousVersion, DeprecationNotice, VersionNotes,
// ChangeSummary, ApprovedBy/ApprovedAt, ExpectedSuccessRate/
// ExpectedDurationSeconds, CreatedBy/UpdatedBy) are left zero-valued: etcd
// never captured this Postgres-catalog bookkeeping, and grepping
// pkg/datastorage confirms buildWorkflowCore (the existing inline/CRD
// registration path) never sets them either -- so this is not a regression.
func crdWorkflowToModel(rw *rwv1alpha1.RemediationWorkflow) (models.RemediationWorkflow, error) {
	detectedLabels, err := crdDetectedLabelsToModel(rw.Spec.DetectedLabels)
	if err != nil {
		return models.RemediationWorkflow{}, fmt.Errorf("workflow %s: %w", rw.Name, err)
	}

	wf := models.RemediationWorkflow{
		WorkflowID:      rw.Status.WorkflowID,
		WorkflowName:    rw.Name,
		Name:            rw.Name,
		Version:         rw.Spec.Version,
		Description:     models.FromSharedDescription(crdDescriptionToShared(rw.Spec.Description)),
		ActionType:      rw.Spec.ActionType,
		ExecutionEngine: models.ExecutionEngine(rw.Spec.Execution.Engine),
		Labels:          crdLabelsToMandatoryLabels(rw.Spec.Labels),
		CustomLabels:    crdCustomLabelsToModel(rw.Spec.CustomLabels),
		DetectedLabels:  detectedLabels,
		Status:          string(rw.Status.CatalogStatus),
		ContentHash:     rw.Status.ContentHash,
		IsLatestVersion: true, // no coexisting versions in etcd: metadata.name is the workflow's identity
		CreatedAt:       rw.CreationTimestamp.Time,
		UpdatedAt:       rw.CreationTimestamp.Time,
	}

	if rw.Spec.Execution.Bundle != "" {
		bundle := rw.Spec.Execution.Bundle
		wf.ExecutionBundle = &bundle
	}
	if rw.Spec.Execution.BundleDigest != "" {
		digest := rw.Spec.Execution.BundleDigest
		wf.ExecutionBundleDigest = &digest
	}
	if rw.Spec.Execution.ServiceAccountName != "" {
		sa := rw.Spec.Execution.ServiceAccountName
		wf.ServiceAccountName = &sa
	}
	if rw.Spec.Execution.EngineConfig != nil {
		raw := json.RawMessage(rw.Spec.Execution.EngineConfig.Raw)
		wf.EngineConfig = &raw
	}

	return wf, nil
}

// crdDescriptionToShared adapts a RemediationWorkflow CRD's description
// (its own struct type, field-identical to sharedtypes.StructuredDescription)
// into the shared type so it can flow through models.FromSharedDescription.
func crdDescriptionToShared(d rwv1alpha1.RemediationWorkflowDescription) sharedtypes.StructuredDescription {
	return sharedtypes.StructuredDescription{
		What:          d.What,
		WhenToUse:     d.WhenToUse,
		WhenNotToUse:  d.WhenNotToUse,
		Preconditions: d.Preconditions,
	}
}
