# RemediationOrchestratorAuditPayload

Type-safe audit event payload for RemediationOrchestrator (lifecycle.started, lifecycle.created, lifecycle.completed, lifecycle.failed, lifecycle.transitioned, approval.requested, approval.approved, approval.rejected)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**rr_name** | **str** | Name of the RemediationRequest being orchestrated | 
**namespace** | **str** | Kubernetes namespace of the RemediationRequest | 
**outcome** | **str** | Final outcome of the orchestration | [optional] 
**duration_ms** | **int** | Orchestration duration in milliseconds | [optional] 
**failure_phase** | **str** | Phase where the failure occurred | [optional] 
**failure_reason** | **str** | Categorized failure reason | [optional] 
**error_details** | [**ErrorDetails**](ErrorDetails.md) |  | [optional] 
**from_phase** | **str** | Phase being transitioned from | [optional] 
**to_phase** | **str** | Phase being transitioned to | [optional] 
**transition_reason** | **str** | Reason for the transition | [optional] 
**rar_name** | **str** | Name of the RemediationApprovalRequest | [optional] 
**required_by** | **datetime** | Approval deadline (RFC3339) | [optional] 
**workflow_id** | **str** | Selected workflow identifier | [optional] 
**confidence_str** | **str** | Workflow selection confidence as string | [optional] 
**decision** | **str** | Approval decision | [optional] 
**approved_by** | **str** | User who approved the request | [optional] 
**rejected_by** | **str** | User who rejected the request | [optional] 
**rejection_reason** | **str** | Reason for rejection | [optional] 
**message** | **str** | Additional message or context for the event | [optional] 
**reason** | **str** | Reason for manual review or other actions | [optional] 
**sub_reason** | **str** | Sub-categorization of the reason | [optional] 
**notification_name** | **str** | Associated notification name | [optional] 
**timeout_config** | [**TimeoutConfig**](TimeoutConfig.md) |  | [optional] 
**pre_remediation_spec_hash** | **str** | Canonical SHA-256 hash of the target resource&#39;s .spec before remediation. Computed using DD-EM-002 canonical spec hash algorithm. Format: \&quot;sha256:&lt;hex&gt;\&quot;  | [optional] 
**target_resource** | **str** | Target resource identifier in format \&quot;namespace/Kind/name\&quot; or \&quot;Kind/name\&quot; for cluster-scoped. Used by remediation.workflow_created to capture what resource is being remediated.  | [optional] 
**workflow_version** | **str** | Version of the selected workflow | [optional] 
**action_type** | **str** | Action type from DD-WORKFLOW-016 taxonomy (e.g., ScaleReplicas, RestartPod). Propagated from AIAnalysis.SelectedWorkflow.ActionType via HAPI three-step discovery. Used by DS remediation history to populate actionType on entries and summaries.  | [optional] 
**ea_name** | **str** | Name of the EffectivenessAssessment CRD created by the RO. Only present for orchestrator.ea.created events.  | [optional] 
**hash_compute_delay** | **str** | Duration-based hash compute delay set on the EA config by the RO. Computed from GitOps sync + operator reconcile delays for async targets. Format: Go duration string. Only present for orchestrator.ea.created events. Reference: DD-EM-004, BR-RO-103, Issue #277  | [optional] 
**alert_check_delay** | **str** | Duration-based alert check delay set on the EA config by the RO. Set for proactive signals where the triggering alert needs extra time to resolve. Format: Go duration string. Reference: BR-EM-009, BR-RO-103, Issue #277  | [optional] 
**gitops_sync_delay** | **str** | GitOps sync delay from RO async propagation config. Only present for orchestrator.ea.created events when target is GitOps-managed. Format: Go duration string. Reference: DD-EM-004 v2.0, BR-RO-103.4  | [optional] 
**operator_reconcile_delay** | **str** | Operator reconcile delay from RO async propagation config. Only present for orchestrator.ea.created events when target is CRD-managed. Format: Go duration string. Reference: DD-EM-004 v2.0, BR-RO-103.4  | [optional] 
**is_gitops_managed** | **bool** | Whether the remediation target was detected as GitOps-managed. Only present for orchestrator.ea.created events.  | [optional] 
**is_crd** | **bool** | Whether the remediation target is a CRD (non-built-in group). Only present for orchestrator.ea.created events.  | [optional] 

## Example

```python
from datastorage.models.remediation_orchestrator_audit_payload import RemediationOrchestratorAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationOrchestratorAuditPayload from a JSON string
remediation_orchestrator_audit_payload_instance = RemediationOrchestratorAuditPayload.from_json(json)
# print the JSON string representation of the object
print RemediationOrchestratorAuditPayload.to_json()

# convert the object into a dict
remediation_orchestrator_audit_payload_dict = remediation_orchestrator_audit_payload_instance.to_dict()
# create an instance of RemediationOrchestratorAuditPayload from a dict
remediation_orchestrator_audit_payload_form_dict = remediation_orchestrator_audit_payload.from_dict(remediation_orchestrator_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


