# ActionTypeWebhookAuditPayload

AW audit payload for ActionType CRD admission events

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** |  | 
**action_type_name** | **str** | PascalCase name from spec.name | 
**crd_name** | **str** | K8s metadata.name | 
**crd_namespace** | **str** | K8s namespace | 
**action** | **str** |  | 
**previously_existed** | **bool** |  | [optional] 
**catalog_status** | **str** |  | [optional] 
**denial_reason** | **str** |  | [optional] 
**denial_operation** | **str** |  | [optional] 

## Example

```python
from datastorage.models.action_type_webhook_audit_payload import ActionTypeWebhookAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeWebhookAuditPayload from a JSON string
action_type_webhook_audit_payload_instance = ActionTypeWebhookAuditPayload.from_json(json)
# print the JSON string representation of the object
print ActionTypeWebhookAuditPayload.to_json()

# convert the object into a dict
action_type_webhook_audit_payload_dict = action_type_webhook_audit_payload_instance.to_dict()
# create an instance of ActionTypeWebhookAuditPayload from a dict
action_type_webhook_audit_payload_form_dict = action_type_webhook_audit_payload.from_dict(action_type_webhook_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


