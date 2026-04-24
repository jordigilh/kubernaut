# ActionTypeDisableResponse

Response for successful action type disable

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**action_type** | **str** |  | 
**status** | **str** |  | 

## Example

```python
from datastorage.models.action_type_disable_response import ActionTypeDisableResponse

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeDisableResponse from a JSON string
action_type_disable_response_instance = ActionTypeDisableResponse.from_json(json)
# print the JSON string representation of the object
print ActionTypeDisableResponse.to_json()

# convert the object into a dict
action_type_disable_response_dict = action_type_disable_response_instance.to_dict()
# create an instance of ActionTypeDisableResponse from a dict
action_type_disable_response_form_dict = action_type_disable_response.from_dict(action_type_disable_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


