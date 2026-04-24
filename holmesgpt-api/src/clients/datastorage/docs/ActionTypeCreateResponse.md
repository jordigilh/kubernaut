# ActionTypeCreateResponse

Response for action type create/re-enable

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**action_type** | **str** | PascalCase action type name | 
**description** | [**ActionTypeDescription**](ActionTypeDescription.md) |  | [optional] 
**status** | **str** | Outcome: created, exists, or reenabled | 
**was_reenabled** | **bool** | true if re-enabled from disabled state | 

## Example

```python
from datastorage.models.action_type_create_response import ActionTypeCreateResponse

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeCreateResponse from a JSON string
action_type_create_response_instance = ActionTypeCreateResponse.from_json(json)
# print the JSON string representation of the object
print ActionTypeCreateResponse.to_json()

# convert the object into a dict
action_type_create_response_dict = action_type_create_response_instance.to_dict()
# create an instance of ActionTypeCreateResponse from a dict
action_type_create_response_form_dict = action_type_create_response.from_dict(action_type_create_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


