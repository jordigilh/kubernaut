# RemediationRequestWebhookAuditPayload

Type-safe audit event payload for RemediationRequest webhooks (timeout_modified)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**rr_name** | **str** | Name of the RemediationRequest | 
**namespace** | **str** | Kubernetes namespace | 
**modified_by** | **str** | User who modified the timeout configuration | 
**modified_at** | **datetime** | When the modification occurred | 
**old_timeout_config** | [**TimeoutConfig**](TimeoutConfig.md) |  | [optional] 
**new_timeout_config** | [**TimeoutConfig**](TimeoutConfig.md) |  | [optional] 

## Example

```python
from datastorage.models.remediation_request_webhook_audit_payload import RemediationRequestWebhookAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationRequestWebhookAuditPayload from a JSON string
remediation_request_webhook_audit_payload_instance = RemediationRequestWebhookAuditPayload.from_json(json)
# print the JSON string representation of the object
print(RemediationRequestWebhookAuditPayload.to_json())

# convert the object into a dict
remediation_request_webhook_audit_payload_dict = remediation_request_webhook_audit_payload_instance.to_dict()
# create an instance of RemediationRequestWebhookAuditPayload from a dict
remediation_request_webhook_audit_payload_from_dict = RemediationRequestWebhookAuditPayload.from_dict(remediation_request_webhook_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


