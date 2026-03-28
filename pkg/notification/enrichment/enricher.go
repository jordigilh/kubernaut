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

// Enricher enriches notification bodies before delivery by resolving workflow
// UUIDs to human-readable names via the DataStorage catalog.
type Enricher struct {
	resolver WorkflowNameResolver
	logger   logr.Logger
}

// NewEnricher creates a new Enricher. resolver may be nil (no-op enrichment).
func NewEnricher(resolver WorkflowNameResolver, logger logr.Logger) *Enricher {
	return &Enricher{resolver: resolver, logger: logger}
}

// EnrichNotification returns a copy of the notification with the workflow UUID
// replaced by the human-readable workflow name. Returns the original notification
// unchanged if resolution fails or no workflow metadata is present.
func (e *Enricher) EnrichNotification(ctx context.Context, notification *notificationv1alpha1.NotificationRequest) *notificationv1alpha1.NotificationRequest {
	if e.resolver == nil {
		return notification
	}

	workflowID := extractWorkflowID(notification.Spec.Context)
	if workflowID == "" {
		return notification
	}

	name, err := e.resolver.ResolveWorkflowName(ctx, workflowID)
	if err != nil || name == "" {
		e.logger.Info("Workflow name resolution failed or empty, keeping UUID",
			"workflowId", workflowID, "error", err)
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
