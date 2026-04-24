# ActionTypeCreateRequest

Request body for creating or re-enabling an action type

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**name** | **str** | PascalCase action type name (e.g., RestartPod) | 
**description** | [**ActionTypeDescription**](ActionTypeDescription.md) |  | 
**registered_by** | **str** | Identity of the registrant (K8s SA or user) | 

## Example

```python
from datastorage.models.action_type_create_request import ActionTypeCreateRequest

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeCreateRequest from a JSON string
action_type_create_request_instance = ActionTypeCreateRequest.from_json(json)
# print the JSON string representation of the object
print ActionTypeCreateRequest.to_json()

# convert the object into a dict
action_type_create_request_dict = action_type_create_request_instance.to_dict()
# create an instance of ActionTypeCreateRequest from a dict
action_type_create_request_form_dict = action_type_create_request.from_dict(action_type_create_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


