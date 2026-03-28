# RemediationWorkflowWebhookAuditPayload

Audit payload for RemediationWorkflow CRD admission events (ADR-058). Emitted by the authwebhook when a RemediationWorkflow CRD is created, deleted, or denied. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**workflow_name** | **str** | Name of the RemediationWorkflow CRD (metadata.name) | 
**action** | **str** | Admission action performed | 
**workflow_id** | **str** | DataStorage catalog UUID (set after successful registration) | [optional] 
**catalog_status** | **str** | Catalog registration status (Active, Disabled, etc.) | [optional] 
**denial_reason** | **str** | Reason for denial (only set when action&#x3D;denied) | [optional] 

## Example

```python
from datastorage.models.remediation_workflow_webhook_audit_payload import RemediationWorkflowWebhookAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationWorkflowWebhookAuditPayload from a JSON string
remediation_workflow_webhook_audit_payload_instance = RemediationWorkflowWebhookAuditPayload.from_json(json)
# print the JSON string representation of the object
print RemediationWorkflowWebhookAuditPayload.to_json()

# convert the object into a dict
remediation_workflow_webhook_audit_payload_dict = remediation_workflow_webhook_audit_payload_instance.to_dict()
# create an instance of RemediationWorkflowWebhookAuditPayload from a dict
remediation_workflow_webhook_audit_payload_form_dict = remediation_workflow_webhook_audit_payload.from_dict(remediation_workflow_webhook_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


