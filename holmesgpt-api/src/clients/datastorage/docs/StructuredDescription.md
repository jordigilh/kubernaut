# StructuredDescription

Structured workflow description for LLM comparison and operator guidance (BR-WORKFLOW-004)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**what** | **str** | What this workflow concretely does. One sentence. | 
**when_to_use** | **str** | Root cause conditions under which this workflow is appropriate. | 
**when_not_to_use** | **str** | Specific exclusion conditions. | [optional] 
**preconditions** | **str** | Conditions that must be verified through investigation. | [optional] 

## Example

```python
from datastorage.models.structured_description import StructuredDescription

# TODO update the JSON string below
json = "{}"
# create an instance of StructuredDescription from a JSON string
structured_description_instance = StructuredDescription.from_json(json)
# print the JSON string representation of the object
print StructuredDescription.to_json()

# convert the object into a dict
structured_description_dict = structured_description_instance.to_dict()
# create an instance of StructuredDescription from a dict
structured_description_form_dict = structured_description.from_dict(structured_description_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


