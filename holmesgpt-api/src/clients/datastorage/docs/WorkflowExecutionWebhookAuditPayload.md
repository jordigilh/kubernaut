# WorkflowExecutionWebhookAuditPayload

Type-safe audit event payload for WorkflowExecution webhooks (workflow.unblocked)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**workflow_name** | **str** | Name of the WorkflowExecution | 
**clear_reason** | **str** | Reason for clearing the block | 
**cleared_at** | **datetime** | When the block was cleared | 
**previous_state** | **str** | State before unblocking (always \&quot;Blocked\&quot;) | 
**new_state** | **str** | State after unblocking (always \&quot;Running\&quot;) | 

## Example

```python
from datastorage.models.workflow_execution_webhook_audit_payload import WorkflowExecutionWebhookAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowExecutionWebhookAuditPayload from a JSON string
workflow_execution_webhook_audit_payload_instance = WorkflowExecutionWebhookAuditPayload.from_json(json)
# print the JSON string representation of the object
print WorkflowExecutionWebhookAuditPayload.to_json()

# convert the object into a dict
workflow_execution_webhook_audit_payload_dict = workflow_execution_webhook_audit_payload_instance.to_dict()
# create an instance of WorkflowExecutionWebhookAuditPayload from a dict
workflow_execution_webhook_audit_payload_form_dict = workflow_execution_webhook_audit_payload.from_dict(workflow_execution_webhook_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


