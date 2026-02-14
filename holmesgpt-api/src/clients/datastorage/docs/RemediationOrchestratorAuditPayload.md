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
**workflow_type** | **str** | Action type from DD-WORKFLOW-016 taxonomy (e.g., ScaleReplicas, RestartPod). Propagated from AIAnalysis.SelectedWorkflow.ActionType via HAPI three-step discovery. Used by DS remediation history to populate workflowType on entries and summaries.  | [optional] 

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


