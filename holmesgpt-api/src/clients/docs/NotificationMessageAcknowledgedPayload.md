# NotificationMessageAcknowledgedPayload

Message acknowledged event payload (notification.message.acknowledged)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**notification_id** | **str** |  | 
**subject** | **str** |  | 
**priority** | **str** |  | 
**metadata** | **Dict[str, str]** |  | [optional] 

## Example

```python
from datastorage.models.notification_message_acknowledged_payload import NotificationMessageAcknowledgedPayload

# TODO update the JSON string below
json = "{}"
# create an instance of NotificationMessageAcknowledgedPayload from a JSON string
notification_message_acknowledged_payload_instance = NotificationMessageAcknowledgedPayload.from_json(json)
# print the JSON string representation of the object
print(NotificationMessageAcknowledgedPayload.to_json())

# convert the object into a dict
notification_message_acknowledged_payload_dict = notification_message_acknowledged_payload_instance.to_dict()
# create an instance of NotificationMessageAcknowledgedPayload from a dict
notification_message_acknowledged_payload_from_dict = NotificationMessageAcknowledgedPayload.from_dict(notification_message_acknowledged_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


