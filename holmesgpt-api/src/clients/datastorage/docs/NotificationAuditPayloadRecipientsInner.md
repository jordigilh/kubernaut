# NotificationAuditPayloadRecipientsInner

Notification recipient (matches CRD Recipient struct)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**email** | **str** | Email address (for email channel) | [optional] 
**slack** | **str** | Slack channel or user (#channel-name or @username) | [optional] 
**teams** | **str** | Teams channel or user | [optional] 
**phone** | **str** | Phone number in E.164 format | [optional] 
**webhook_url** | **str** | Webhook URL for webhook channel | [optional] 

## Example

```python
from datastorage.models.notification_audit_payload_recipients_inner import NotificationAuditPayloadRecipientsInner

# TODO update the JSON string below
json = "{}"
# create an instance of NotificationAuditPayloadRecipientsInner from a JSON string
notification_audit_payload_recipients_inner_instance = NotificationAuditPayloadRecipientsInner.from_json(json)
# print the JSON string representation of the object
print NotificationAuditPayloadRecipientsInner.to_json()

# convert the object into a dict
notification_audit_payload_recipients_inner_dict = notification_audit_payload_recipients_inner_instance.to_dict()
# create an instance of NotificationAuditPayloadRecipientsInner from a dict
notification_audit_payload_recipients_inner_form_dict = notification_audit_payload_recipients_inner.from_dict(notification_audit_payload_recipients_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


