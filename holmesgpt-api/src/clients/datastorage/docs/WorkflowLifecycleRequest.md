# WorkflowLifecycleRequest

Request for workflow lifecycle operations (enable, disable, deprecate). Reason is mandatory per DD-WORKFLOW-017 Phase 4.4.

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**reason** | **str** | Why the lifecycle operation is being performed (mandatory) | 
**updated_by** | **str** | Who is performing the operation | [optional] 

## Example

```python
from datastorage.models.workflow_lifecycle_request import WorkflowLifecycleRequest

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowLifecycleRequest from a JSON string
workflow_lifecycle_request_instance = WorkflowLifecycleRequest.from_json(json)
# print the JSON string representation of the object
print WorkflowLifecycleRequest.to_json()

# convert the object into a dict
workflow_lifecycle_request_dict = workflow_lifecycle_request_instance.to_dict()
# create an instance of WorkflowLifecycleRequest from a dict
workflow_lifecycle_request_form_dict = workflow_lifecycle_request.from_dict(workflow_lifecycle_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


