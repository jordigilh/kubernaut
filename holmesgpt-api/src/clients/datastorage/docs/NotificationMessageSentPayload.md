# NotificationMessageSentPayload

Message sent event payload (notification.message.sent)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**notification_id** | **str** | Name of the NotificationRequest CRD | 
**channel** | **str** | Delivery channel | 
**subject** | **str** | Notification subject line | 
**body** | **str** | Notification message body | 
**priority** | **str** | Notification priority level | 
**type** | **str** | Notification type | 
**metadata** | **Dict[str, str]** |  | [optional] 

## Example

```python
from datastorage.models.notification_message_sent_payload import NotificationMessageSentPayload

# TODO update the JSON string below
json = "{}"
# create an instance of NotificationMessageSentPayload from a JSON string
notification_message_sent_payload_instance = NotificationMessageSentPayload.from_json(json)
# print the JSON string representation of the object
print NotificationMessageSentPayload.to_json()

# convert the object into a dict
notification_message_sent_payload_dict = notification_message_sent_payload_instance.to_dict()
# create an instance of NotificationMessageSentPayload from a dict
notification_message_sent_payload_form_dict = notification_message_sent_payload.from_dict(notification_message_sent_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


