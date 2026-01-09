# RemediationOrchestratorAuditPayload

Type-safe audit event payload for RemediationOrchestrator (lifecycle.started, lifecycle.completed, lifecycle.failed, lifecycle.transitioned)

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

## Example

```python
from datastorage.models.remediation_orchestrator_audit_payload import RemediationOrchestratorAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationOrchestratorAuditPayload from a JSON string
remediation_orchestrator_audit_payload_instance = RemediationOrchestratorAuditPayload.from_json(json)
# print the JSON string representation of the object
print(RemediationOrchestratorAuditPayload.to_json())

# convert the object into a dict
remediation_orchestrator_audit_payload_dict = remediation_orchestrator_audit_payload_instance.to_dict()
# create an instance of RemediationOrchestratorAuditPayload from a dict
remediation_orchestrator_audit_payload_from_dict = RemediationOrchestratorAuditPayload.from_dict(remediation_orchestrator_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


