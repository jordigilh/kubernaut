# WorkflowSearchResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflows** | [**List[WorkflowSearchResult]**](WorkflowSearchResult.md) |  | [optional] 
**total_results** | **int** | Total number of matching workflows | [optional] 
**filters** | [**WorkflowSearchFilters**](WorkflowSearchFilters.md) |  | [optional] 

## Example

```python
from datastorage.models.workflow_search_response import WorkflowSearchResponse

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowSearchResponse from a JSON string
workflow_search_response_instance = WorkflowSearchResponse.from_json(json)
# print the JSON string representation of the object
print WorkflowSearchResponse.to_json()

# convert the object into a dict
workflow_search_response_dict = workflow_search_response_instance.to_dict()
# create an instance of WorkflowSearchResponse from a dict
workflow_search_response_form_dict = workflow_search_response.from_dict(workflow_search_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


