# ActionTypeListResponse

Response for Step 1: list available action types (DD-WORKFLOW-016)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**action_types** | [**List[ActionTypeEntry]**](ActionTypeEntry.md) |  | 
**pagination** | [**PaginationMetadata**](PaginationMetadata.md) |  | 

## Example

```python
from datastorage.models.action_type_list_response import ActionTypeListResponse

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeListResponse from a JSON string
action_type_list_response_instance = ActionTypeListResponse.from_json(json)
# print the JSON string representation of the object
print ActionTypeListResponse.to_json()

# convert the object into a dict
action_type_list_response_dict = action_type_list_response_instance.to_dict()
# create an instance of ActionTypeListResponse from a dict
action_type_list_response_form_dict = action_type_list_response.from_dict(action_type_list_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


