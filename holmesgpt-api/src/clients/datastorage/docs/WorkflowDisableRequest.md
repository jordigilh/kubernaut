# WorkflowDisableRequest

Convenience request to disable a workflow (deprecated: use WorkflowLifecycleRequest)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**reason** | **str** | Why the workflow is being disabled | [optional] 
**updated_by** | **str** | Who is disabling the workflow | [optional] 

## Example

```python
from datastorage.models.workflow_disable_request import WorkflowDisableRequest

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowDisableRequest from a JSON string
workflow_disable_request_instance = WorkflowDisableRequest.from_json(json)
# print the JSON string representation of the object
print WorkflowDisableRequest.to_json()

# convert the object into a dict
workflow_disable_request_dict = workflow_disable_request_instance.to_dict()
# create an instance of WorkflowDisableRequest from a dict
workflow_disable_request_form_dict = workflow_disable_request.from_dict(workflow_disable_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


