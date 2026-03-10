# ActionTypeDescriptionPayload

Structured description of an action type for audit payloads

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**what** | **str** |  | 
**when_to_use** | **str** |  | 
**when_not_to_use** | **str** |  | [optional] 
**preconditions** | **str** |  | [optional] 

## Example

```python
from datastorage.models.action_type_description_payload import ActionTypeDescriptionPayload

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeDescriptionPayload from a JSON string
action_type_description_payload_instance = ActionTypeDescriptionPayload.from_json(json)
# print the JSON string representation of the object
print ActionTypeDescriptionPayload.to_json()

# convert the object into a dict
action_type_description_payload_dict = action_type_description_payload_instance.to_dict()
# create an instance of ActionTypeDescriptionPayload from a dict
action_type_description_payload_form_dict = action_type_description_payload.from_dict(action_type_description_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


