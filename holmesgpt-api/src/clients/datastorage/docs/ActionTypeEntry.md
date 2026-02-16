# ActionTypeEntry

Single action type with description and workflow count

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**action_type** | **str** | Action type identifier (e.g., ScaleReplicas, RestartPod) | 
**description** | [**StructuredDescription**](StructuredDescription.md) |  | 
**workflow_count** | **int** | Number of active workflows matching this action type and context filters | 

## Example

```python
from datastorage.models.action_type_entry import ActionTypeEntry

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeEntry from a JSON string
action_type_entry_instance = ActionTypeEntry.from_json(json)
# print the JSON string representation of the object
print ActionTypeEntry.to_json()

# convert the object into a dict
action_type_entry_dict = action_type_entry_instance.to_dict()
# create an instance of ActionTypeEntry from a dict
action_type_entry_form_dict = action_type_entry.from_dict(action_type_entry_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


