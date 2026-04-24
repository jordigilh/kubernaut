# WorkflowSearchRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**remediation_id** | **str** | Optional remediation ID for audit correlation | [optional] 
**filters** | [**WorkflowSearchFilters**](WorkflowSearchFilters.md) |  | 
**top_k** | **int** | Maximum number of results to return | [optional] [default to 10]
**min_score** | **float** | Minimum normalized score threshold (0.0-1.0) | [optional] [default to 0.0]
**include_disabled** | **bool** | Include disabled workflows in results | [optional] [default to False]

## Example

```python
from datastorage.models.workflow_search_request import WorkflowSearchRequest

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowSearchRequest from a JSON string
workflow_search_request_instance = WorkflowSearchRequest.from_json(json)
# print the JSON string representation of the object
print WorkflowSearchRequest.to_json()

# convert the object into a dict
workflow_search_request_dict = workflow_search_request_instance.to_dict()
# create an instance of WorkflowSearchRequest from a dict
workflow_search_request_form_dict = workflow_search_request.from_dict(workflow_search_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


