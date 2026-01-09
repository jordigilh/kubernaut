# NotificationMessageEscalatedPayload

Message escalated event payload (notification.message.escalated)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**notification_id** | **str** |  | 
**subject** | **str** |  | 
**priority** | **str** |  | 
**reason** | **str** |  | 
**metadata** | **Dict[str, str]** |  | [optional] 

## Example

```python
from datastorage.models.notification_message_escalated_payload import NotificationMessageEscalatedPayload

# TODO update the JSON string below
json = "{}"
# create an instance of NotificationMessageEscalatedPayload from a JSON string
notification_message_escalated_payload_instance = NotificationMessageEscalatedPayload.from_json(json)
# print the JSON string representation of the object
print(NotificationMessageEscalatedPayload.to_json())

# convert the object into a dict
notification_message_escalated_payload_dict = notification_message_escalated_payload_instance.to_dict()
# create an instance of NotificationMessageEscalatedPayload from a dict
notification_message_escalated_payload_from_dict = NotificationMessageEscalatedPayload.from_dict(notification_message_escalated_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


