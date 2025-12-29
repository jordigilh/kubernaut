# QueryDimensions


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**incident_type** | **str** | Incident type filter (empty if not specified) | [optional] 
**workflow_id** | **str** | Workflow ID filter (empty if not specified) | [optional] 
**workflow_version** | **str** | Workflow version filter (empty if not specified) | [optional] 
**action_type** | **str** | Action type filter (empty if not specified) | [optional] 

## Example

```python
from datastorage.models.query_dimensions import QueryDimensions

# TODO update the JSON string below
json = "{}"
# create an instance of QueryDimensions from a JSON string
query_dimensions_instance = QueryDimensions.from_json(json)
# print the JSON string representation of the object
print(QueryDimensions.to_json())

# convert the object into a dict
query_dimensions_dict = query_dimensions_instance.to_dict()
# create an instance of QueryDimensions from a dict
query_dimensions_from_dict = QueryDimensions.from_dict(query_dimensions_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


