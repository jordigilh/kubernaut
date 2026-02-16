# PaginationMetadata

Pagination metadata for discovery endpoints (DD-WORKFLOW-016)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**total_count** | **int** | Total number of results across all pages | 
**offset** | **int** | Current offset (0-based) | 
**limit** | **int** | Page size | 
**has_more** | **bool** | True if more pages exist beyond current offset+limit | 

## Example

```python
from datastorage.models.pagination_metadata import PaginationMetadata

# TODO update the JSON string below
json = "{}"
# create an instance of PaginationMetadata from a JSON string
pagination_metadata_instance = PaginationMetadata.from_json(json)
# print the JSON string representation of the object
print PaginationMetadata.to_json()

# convert the object into a dict
pagination_metadata_dict = pagination_metadata_instance.to_dict()
# create an instance of PaginationMetadata from a dict
pagination_metadata_form_dict = pagination_metadata.from_dict(pagination_metadata_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


