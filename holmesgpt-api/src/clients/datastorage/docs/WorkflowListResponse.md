# WorkflowListResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflows** | [**List[RemediationWorkflow]**](RemediationWorkflow.md) |  | [optional] 
**limit** | **int** |  | [optional] 
**offset** | **int** |  | [optional] 
**total** | **int** |  | [optional] 

## Example

```python
from datastorage.models.workflow_list_response import WorkflowListResponse

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowListResponse from a JSON string
workflow_list_response_instance = WorkflowListResponse.from_json(json)
# print the JSON string representation of the object
print WorkflowListResponse.to_json()

# convert the object into a dict
workflow_list_response_dict = workflow_list_response_instance.to_dict()
# create an instance of WorkflowListResponse from a dict
workflow_list_response_form_dict = workflow_list_response.from_dict(workflow_list_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


