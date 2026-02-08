# SearchExecutionMetadata

Search execution details (BR-AUDIT-028). V1.0 label-only (DD-WORKFLOW-015).

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**duration_ms** | **int** | Search execution time in milliseconds | 

## Example

```python
from datastorage.models.search_execution_metadata import SearchExecutionMetadata

# TODO update the JSON string below
json = "{}"
# create an instance of SearchExecutionMetadata from a JSON string
search_execution_metadata_instance = SearchExecutionMetadata.from_json(json)
# print the JSON string representation of the object
print SearchExecutionMetadata.to_json()

# convert the object into a dict
search_execution_metadata_dict = search_execution_metadata_instance.to_dict()
# create an instance of SearchExecutionMetadata from a dict
search_execution_metadata_form_dict = search_execution_metadata.from_dict(search_execution_metadata_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


