# NotificationAuditPayload

Type-safe audit event payload for NotificationRequest webhooks (notification.cancelled, notification.acknowledged)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**notification_id** | **str** | Name of the NotificationRequest | [optional] 
**notification_name** | **str** | Alias for notification_id | [optional] 
**type** | **str** | Notification type (matches api/notification/v1alpha1/notificationrequest_types.go:31-40) | [optional] 
**notification_type** | **str** | Alias for type (matches CRD NotificationType enum) | [optional] 
**priority** | **str** | Notification priority (matches api/notification/v1alpha1/notificationrequest_types.go:47-50) | [optional] 
**final_status** | **str** | Final status of the notification (matches api/notification/v1alpha1/notificationrequest_types.go:60-65) | [optional] 
**recipients** | [**List[NotificationAuditPayloadRecipientsInner]**](NotificationAuditPayloadRecipientsInner.md) | Array of notification recipients from CRD (BR-NOTIFICATION-001, matches api/notification/v1alpha1/notificationrequest_types.go:80-102) | [optional] 
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
print NotificationAuditPayload.to_json()

# convert the object into a dict
notification_audit_payload_dict = notification_audit_payload_instance.to_dict()
# create an instance of NotificationAuditPayload from a dict
notification_audit_payload_form_dict = notification_audit_payload.from_dict(notification_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


