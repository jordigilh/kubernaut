# ActionTypeEntryDescription

Curated description with what, when_to_use, when_not_to_use, preconditions

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**what** | **str** | What this action type does | [optional] 
**when_to_use** | **str** | When to use this action type | [optional] 
**when_not_to_use** | **str** | When NOT to use this action type | [optional] 
**preconditions** | **str** | Preconditions that must be met | [optional] 

## Example

```python
from datastorage.models.action_type_entry_description import ActionTypeEntryDescription

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeEntryDescription from a JSON string
action_type_entry_description_instance = ActionTypeEntryDescription.from_json(json)
# print the JSON string representation of the object
print ActionTypeEntryDescription.to_json()

# convert the object into a dict
action_type_entry_description_dict = action_type_entry_description_instance.to_dict()
# create an instance of ActionTypeEntryDescription from a dict
action_type_entry_description_form_dict = action_type_entry_description.from_dict(action_type_entry_description_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


