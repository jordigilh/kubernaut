# ActionTypeDisableDeniedResponse

409 response when disable is denied due to active workflow dependencies

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**action_type** | **str** |  | 
**dependent_workflow_count** | **int** | Number of active workflows referencing this action type | 
**dependent_workflows** | **List[str]** | Names of dependent workflows | 

## Example

```python
from datastorage.models.action_type_disable_denied_response import ActionTypeDisableDeniedResponse

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeDisableDeniedResponse from a JSON string
action_type_disable_denied_response_instance = ActionTypeDisableDeniedResponse.from_json(json)
# print the JSON string representation of the object
print ActionTypeDisableDeniedResponse.to_json()

# convert the object into a dict
action_type_disable_denied_response_dict = action_type_disable_denied_response_instance.to_dict()
# create an instance of ActionTypeDisableDeniedResponse from a dict
action_type_disable_denied_response_form_dict = action_type_disable_denied_response.from_dict(action_type_disable_denied_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


