# QueryMetadata

Search query parameters (BR-AUDIT-025)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**top_k** | **int** | Maximum number of results to return | 
**min_score** | **float** | Minimum similarity score threshold | [optional] 
**filters** | [**WorkflowSearchFilters**](WorkflowSearchFilters.md) |  | [optional] 

## Example

```python
from datastorage.models.query_metadata import QueryMetadata

# TODO update the JSON string below
json = "{}"
# create an instance of QueryMetadata from a JSON string
query_metadata_instance = QueryMetadata.from_json(json)
# print the JSON string representation of the object
print QueryMetadata.to_json()

# convert the object into a dict
query_metadata_dict = query_metadata_instance.to_dict()
# create an instance of QueryMetadata from a dict
query_metadata_form_dict = query_metadata.from_dict(query_metadata_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


