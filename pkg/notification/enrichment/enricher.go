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

package enrichment

import (
	"context"
	"strings"

	"github.com/go-logr/logr"

	notificationv1alpha1 "github.com/jordigilh/kubernaut/api/notification/v1alpha1"
)

// Enricher enriches notification bodies before delivery by replacing a
// workflow UUID with its catalog-authoritative human-readable name.
type Enricher struct {
	logger logr.Logger
}

// NewEnricher creates a new Enricher.
func NewEnricher(logger logr.Logger) *Enricher {
	return &Enricher{logger: logger}
}

// EnrichNotification returns a copy of the notification with the workflow UUID
// replaced by the human-readable workflow name. Returns the original notification
// unchanged if no workflow metadata is present or no name can be determined.
//
// Issue #1677 Phase 1 (DD-WORKFLOW-018 v1.1): the name comes exclusively from
// the catalog-authoritative WorkflowName RemediationOrchestrator already
// populates on the context (sourced from
// AIAnalysis.Status.SelectedWorkflow.WorkflowName) -- no live DataStorage call
// is made or needed here.
//
// #1677 follow-up cleanup: this used to fall back to a live
// WorkflowNameResolver (calling DS's now-retired GET /api/v1/workflows/
// {workflow_id}) when WorkflowName was absent. That fallback was deleted
// outright: WorkflowName is +kubebuilder:validation:Required on the
// underlying WorkflowSnapshot type -- always equal to
// RemediationWorkflow.metadata.name, a Kubernetes-guaranteed non-empty value
// -- so "workflow ID present, WorkflowName absent" cannot occur via either of
// RemediationOrchestrator's two notification-creation call sites
// (pkg/remediationorchestrator/creator/notification.go). If WorkflowName is
// ever absent regardless (e.g. a future notification path that doesn't
// populate it), this degrades gracefully to leaving the raw UUID in the body,
// exactly as it did when the deleted resolver failed or returned "".
func (e *Enricher) EnrichNotification(_ context.Context, notification *notificationv1alpha1.NotificationRequest) *notificationv1alpha1.NotificationRequest {
	workflowID := extractWorkflowID(notification.Spec.Context)
	if workflowID == "" {
		return notification
	}

	name := extractWorkflowName(notification.Spec.Context)
	if name == "" {
		return notification
	}

	enriched := notification.DeepCopy()
	enriched.Spec.Body = strings.ReplaceAll(enriched.Spec.Body, workflowID, name)
	return enriched
}

// extractWorkflowID reads the workflow UUID from the typed notification context.
// Checks WorkflowID (completion) first, then SelectedWorkflow (approval).
func extractWorkflowID(ctx *notificationv1alpha1.NotificationContext) string {
	if ctx == nil || ctx.Workflow == nil {
		return ""
	}
	if ctx.Workflow.WorkflowID != "" {
		return ctx.Workflow.WorkflowID
	}
	if ctx.Workflow.SelectedWorkflow != "" {
		return ctx.Workflow.SelectedWorkflow
	}
	return ""
}

// extractWorkflowName reads the catalog-authoritative WorkflowName from the
// typed notification context, when already populated by the caller (Issue
// #1677 Phase 1). Returns "" if absent, signaling the live-resolver fallback.
func extractWorkflowName(ctx *notificationv1alpha1.NotificationContext) string {
	if ctx == nil || ctx.Workflow == nil {
		return ""
	}
	return ctx.Workflow.WorkflowName
}
