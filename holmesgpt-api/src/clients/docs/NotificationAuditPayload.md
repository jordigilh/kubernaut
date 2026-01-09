# NotificationAuditPayload

Type-safe audit event payload for NotificationRequest webhooks (notification.cancelled, notification.acknowledged)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**notification_id** | **str** | Name of the NotificationRequest | [optional] 
**notification_name** | **str** | Alias for notification_id | [optional] 
**type** | **str** | Notification type | [optional] 
**notification_type** | **str** | Alias for type | [optional] 
**priority** | **str** | Notification priority | [optional] 
**final_status** | **str** | Final status of the notification | [optional] 
**recipients** | **Dict[str, object]** | Notification recipients (structured type from CRD) | [optional] 
**cancelled_by** | **str** | Username who cancelled the notification | [optional] 
**user_uid** | **str** | UID of the user who performed the action | [optional] 
**user_groups** | **List[str]** | Groups of the user who performed the action | [optional] 
**action** | **str** | Webhook action performed | [optional] 

## Example

```python
from datastorage.models.notification_audit_payload import NotificationAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of NotificationAuditPayload from a JSON string
notification_audit_payload_instance = NotificationAuditPayload.from_json(json)
# print the JSON string representation of the object
print(NotificationAuditPayload.to_json())

# convert the object into a dict
notification_audit_payload_dict = notification_audit_payload_instance.to_dict()
# create an instance of NotificationAuditPayload from a dict
notification_audit_payload_from_dict = NotificationAuditPayload.from_dict(notification_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


