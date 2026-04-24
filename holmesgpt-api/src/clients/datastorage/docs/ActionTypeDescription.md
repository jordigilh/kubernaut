# ActionTypeDescription

Structured description for an action type

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**what** | **str** | What this action type concretely does | 
**when_to_use** | **str** | When this action type is appropriate | 
**when_not_to_use** | **str** | Exclusion conditions | [optional] 
**preconditions** | **str** | Conditions to verify before use | [optional] 

## Example

```python
from datastorage.models.action_type_description import ActionTypeDescription

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeDescription from a JSON string
action_type_description_instance = ActionTypeDescription.from_json(json)
# print the JSON string representation of the object
print ActionTypeDescription.to_json()

# convert the object into a dict
action_type_description_dict = action_type_description_instance.to_dict()
# create an instance of ActionTypeDescription from a dict
action_type_description_form_dict = action_type_description.from_dict(action_type_description_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


