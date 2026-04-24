# NotificationMessageFailedPayload

Message failed event payload (notification.message.failed)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**notification_id** | **str** |  | 
**channel** | **str** |  | 
**subject** | **str** |  | 
**body** | **str** |  | 
**priority** | **str** |  | 
**error_type** | **str** |  | 
**error** | **str** |  | [optional] 
**metadata** | **Dict[str, str]** |  | [optional] 

## Example

```python
from datastorage.models.notification_message_failed_payload import NotificationMessageFailedPayload

# TODO update the JSON string below
json = "{}"
# create an instance of NotificationMessageFailedPayload from a JSON string
notification_message_failed_payload_instance = NotificationMessageFailedPayload.from_json(json)
# print the JSON string representation of the object
print NotificationMessageFailedPayload.to_json()

# convert the object into a dict
notification_message_failed_payload_dict = notification_message_failed_payload_instance.to_dict()
# create an instance of NotificationMessageFailedPayload from a dict
notification_message_failed_payload_form_dict = notification_message_failed_payload.from_dict(notification_message_failed_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


