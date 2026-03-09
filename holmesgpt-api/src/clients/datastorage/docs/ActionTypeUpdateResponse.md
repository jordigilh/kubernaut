# ActionTypeUpdateResponse

Response for action type description update

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**action_type** | **str** |  | 
**old_description** | [**ActionTypeDescription**](ActionTypeDescription.md) |  | 
**new_description** | [**ActionTypeDescription**](ActionTypeDescription.md) |  | 
**updated_fields** | **List[str]** | List of changed field names | 

## Example

```python
from datastorage.models.action_type_update_response import ActionTypeUpdateResponse

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeUpdateResponse from a JSON string
action_type_update_response_instance = ActionTypeUpdateResponse.from_json(json)
# print the JSON string representation of the object
print ActionTypeUpdateResponse.to_json()

# convert the object into a dict
action_type_update_response_dict = action_type_update_response_instance.to_dict()
# create an instance of ActionTypeUpdateResponse from a dict
action_type_update_response_form_dict = action_type_update_response.from_dict(action_type_update_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


