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

package authwebhook

import (
	"context"

	rwv1alpha1 "github.com/jordigilh/kubernaut/api/remediationworkflow/v1alpha1"
	api "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// emitAdmitAudit emits a success audit event for a CREATE, UPDATE, or DELETE
// operation. spec and contentHash are best-effort: pass nil/"" when there is
// no workflow content to attach (e.g. DELETE, #1661 Change 2).
func (h *RemediationWorkflowHandler) emitAdmitAudit(
	ctx context.Context,
	req admission.Request,
	eventType, workflowID, resourceName string,
	spec *rwv1alpha1.RemediationWorkflowSpec,
	contentHash string,
) {
	if h.auditStore == nil {
		return
	}

	resourceID := workflowID
	if resourceID == "" {
		resourceID = resourceName
	}

	event := buildAuditEnvelope(req, WebhookAuditOpts{
		EventType:    eventType,
		Category:     EventCategoryWorkflow,
		Action:       "admitted",
		Outcome:      api.AuditEventRequestEventOutcomeSuccess,
		ResourceKind: "RemediationWorkflow",
		ResourceID:   resourceID,
	})

	action := api.RemediationWorkflowWebhookAuditPayloadActionCreate
	ogenEventType := api.RemediationWorkflowWebhookAuditPayloadEventTypeRemediationworkflowAdmittedCreate
	wrapFn := api.NewAuditEventRequestEventDataRemediationworkflowAdmittedCreateAuditEventRequestEventData
	switch eventType {
	case EventTypeRWAdmittedUpdate:
		action = api.RemediationWorkflowWebhookAuditPayloadActionUpdate
		ogenEventType = api.RemediationWorkflowWebhookAuditPayloadEventTypeRemediationworkflowAdmittedUpdate
		wrapFn = api.NewAuditEventRequestEventDataRemediationworkflowAdmittedUpdateAuditEventRequestEventData
	case EventTypeRWAdmittedDelete:
		action = api.RemediationWorkflowWebhookAuditPayloadActionDelete
		ogenEventType = api.RemediationWorkflowWebhookAuditPayloadEventTypeRemediationworkflowAdmittedDelete
		wrapFn = api.NewAuditEventRequestEventDataRemediationworkflowAdmittedDeleteAuditEventRequestEventData
	}

	payload := api.RemediationWorkflowWebhookAuditPayload{
		EventType:    ogenEventType,
		WorkflowName: resourceName,
		Action:       action,
	}
	if workflowID != "" {
		payload.WorkflowID.SetTo(workflowID)
	}
	attachWorkflowContent(&payload, spec, contentHash)
	event.EventData = wrapFn(payload)

	storeAuditBestEffort(ctx, h.auditStore, event, "rw-webhook", eventType)
}

// emitDeniedAudit emits a denied audit event when CREATE or UPDATE is
// rejected. spec is the already-unmarshaled RemediationWorkflow spec, or nil
// when unmarshal itself failed (nothing to capture). #1661 Change 2: denied
// events attach the same best-effort workflow_content/content_hash as
// admitted ones whenever spec is available, giving forensic visibility into
// what was rejected and why (BR-AUDIT-005 v2.0 #7, SOC2 CC7.2).
func (h *RemediationWorkflowHandler) emitDeniedAudit(
	ctx context.Context,
	req admission.Request,
	reason string,
	spec *rwv1alpha1.RemediationWorkflowSpec,
	contentHash string,
) {
	if h.auditStore == nil {
		return
	}

	event := buildAuditEnvelope(req, WebhookAuditOpts{
		EventType:    EventTypeRWAdmittedDenied,
		Category:     EventCategoryWorkflow,
		Action:       "denied",
		Outcome:      api.AuditEventRequestEventOutcomeFailure,
		ResourceKind: "RemediationWorkflow",
		ResourceID:   req.Name,
	})

	payload := api.RemediationWorkflowWebhookAuditPayload{
		EventType:    api.RemediationWorkflowWebhookAuditPayloadEventTypeRemediationworkflowAdmittedDenied,
		WorkflowName: req.Name,
		Action:       api.RemediationWorkflowWebhookAuditPayloadActionDenied,
	}
	payload.DenialReason.SetTo(reason)
	attachWorkflowContent(&payload, spec, contentHash)
	event.EventData = api.NewAuditEventRequestEventDataRemediationworkflowAdmittedDeniedAuditEventRequestEventData(payload)

	storeAuditBestEffort(ctx, h.auditStore, event, "rw-webhook", EventTypeRWAdmittedDenied)
}

// attachWorkflowContent sets payload.WorkflowContent/ContentHash from spec
// when available. No-op when spec is nil (unmarshal failed, or the caller
// intentionally omits content, e.g. DELETE).
func attachWorkflowContent(payload *api.RemediationWorkflowWebhookAuditPayload, spec *rwv1alpha1.RemediationWorkflowSpec, contentHash string) {
	if spec == nil {
		return
	}
	payload.WorkflowContent.SetTo(buildWorkflowContentPayload(*spec))
	if contentHash != "" {
		payload.ContentHash.SetTo(contentHash)
	}
}

// buildWorkflowContentPayload maps a RemediationWorkflowSpec
// (api/remediationworkflow/v1alpha1) field-for-field onto the audit-trail's
// RemediationWorkflowContentPayload (api/openapi/data-storage-v1.yaml), so the
// audit trail can reconstruct the exact workflow definition independent of
// etcd or DataStorage's cache (#1661).
func buildWorkflowContentPayload(spec rwv1alpha1.RemediationWorkflowSpec) api.RemediationWorkflowContentPayload {
	content := api.RemediationWorkflowContentPayload{
		Version:    spec.Version,
		ActionType: spec.ActionType,
		Description: api.StructuredDescription{
			What:      spec.Description.What,
			WhenToUse: spec.Description.WhenToUse,
		},
		Labels: api.RemediationWorkflowContentLabels{
			Severity:    spec.Labels.Severity,
			Environment: spec.Labels.Environment,
			Component:   spec.Labels.Component,
			Priority:    spec.Labels.Priority,
			Cluster:     spec.Labels.Cluster,
		},
		Maintainers:        buildContentMaintainers(spec.Maintainers),
		Parameters:         buildContentParameters(spec.Parameters),
		RollbackParameters: buildContentParameters(spec.RollbackParameters),
	}

	setOptString(&content.Description.WhenNotToUse, spec.Description.WhenNotToUse)
	setOptString(&content.Description.Preconditions, spec.Description.Preconditions)

	if len(spec.CustomLabels) > 0 {
		content.CustomLabels.SetTo(api.RemediationWorkflowContentPayloadCustomLabels(spec.CustomLabels))
	}
	if spec.DetectedLabels != nil {
		content.DetectedLabels = spec.DetectedLabels.Raw
	}

	content.Execution = api.RemediationWorkflowContentExecution{
		EngineConfig: engineConfigRaw(spec.Execution),
	}
	setOptString(&content.Execution.Engine, spec.Execution.Engine)
	setOptString(&content.Execution.Bundle, spec.Execution.Bundle)
	setOptString(&content.Execution.BundleDigest, spec.Execution.BundleDigest)
	setOptString(&content.Execution.ServiceAccountName, spec.Execution.ServiceAccountName)

	if spec.Dependencies != nil {
		content.Dependencies.SetTo(api.RemediationWorkflowContentDependencies{
			Secrets:    buildContentResourceDeps(spec.Dependencies.Secrets),
			ConfigMaps: buildContentResourceDeps(spec.Dependencies.ConfigMaps),
		})
	}

	return content
}

// setOptString assigns val to opt only when non-empty, deduping the
// empty-means-absent guard repeated across every genuinely-optional CRD
// string field mapped onto an ogen Opt wrapper below.
func setOptString(opt *api.OptString, val string) {
	if val != "" {
		opt.SetTo(val)
	}
}

// setOptFloat64 assigns *val to opt only when val is non-nil, deduping the
// pointer-means-absent guard shared by Parameter.Minimum/Maximum.
func setOptFloat64(opt *api.OptFloat64, val *float64) {
	if val != nil {
		opt.SetTo(*val)
	}
}

func engineConfigRaw(execution rwv1alpha1.RemediationWorkflowExecution) []byte {
	if execution.EngineConfig == nil {
		return nil
	}
	return execution.EngineConfig.Raw
}

func buildContentResourceDeps(deps []rwv1alpha1.RemediationWorkflowResourceDependency) []api.RemediationWorkflowContentResourceDependency {
	if len(deps) == 0 {
		return nil
	}
	result := make([]api.RemediationWorkflowContentResourceDependency, 0, len(deps))
	for _, d := range deps {
		result = append(result, api.RemediationWorkflowContentResourceDependency{Name: d.Name})
	}
	return result
}

func buildContentMaintainers(maintainers []rwv1alpha1.RemediationWorkflowMaintainer) []api.RemediationWorkflowContentMaintainer {
	if len(maintainers) == 0 {
		return nil
	}
	result := make([]api.RemediationWorkflowContentMaintainer, 0, len(maintainers))
	for _, m := range maintainers {
		result = append(result, api.RemediationWorkflowContentMaintainer{Name: m.Name, Email: m.Email})
	}
	return result
}

func buildContentParameters(params []rwv1alpha1.RemediationWorkflowParameter) []api.RemediationWorkflowContentParameter {
	if len(params) == 0 {
		return nil
	}
	result := make([]api.RemediationWorkflowContentParameter, 0, len(params))
	for _, p := range params {
		cp := api.RemediationWorkflowContentParameter{
			Name:        p.Name,
			Type:        p.Type,
			Required:    p.Required,
			Description: p.Description,
			Enum:        p.Enum,
			DependsOn:   p.DependsOn,
		}
		setOptString(&cp.Pattern, p.Pattern)
		setOptFloat64(&cp.Minimum, p.Minimum)
		setOptFloat64(&cp.Maximum, p.Maximum)
		if p.Default != nil {
			cp.Default = p.Default.Raw
		}
		result = append(result, cp)
	}
	return result
}
