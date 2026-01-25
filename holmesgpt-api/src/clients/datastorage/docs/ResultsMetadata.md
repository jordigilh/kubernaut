# ResultsMetadata

Search results metadata (BR-AUDIT-027)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**total_found** | **int** | Total number of workflows matching the query | 
**returned** | **int** | Number of workflows returned in this response | 
**workflows** | [**List[WorkflowResultAudit]**](WorkflowResultAudit.md) |  | 

## Example

```python
from datastorage.models.results_metadata import ResultsMetadata

# TODO update the JSON string below
json = "{}"
# create an instance of ResultsMetadata from a JSON string
results_metadata_instance = ResultsMetadata.from_json(json)
# print the JSON string representation of the object
print ResultsMetadata.to_json()

# convert the object into a dict
results_metadata_dict = results_metadata_instance.to_dict()
# create an instance of ResultsMetadata from a dict
results_metadata_form_dict = results_metadata.from_dict(results_metadata_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


