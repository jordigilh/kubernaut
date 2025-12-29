# DisableWorkflowRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**reason** | **str** | Reason for disabling the workflow (required for audit trail) | 
**updated_by** | **str** | User or service that disabled the workflow | [optional] 

## Example

```python
from datastorage.models.disable_workflow_request import DisableWorkflowRequest

# TODO update the JSON string below
json = "{}"
# create an instance of DisableWorkflowRequest from a JSON string
disable_workflow_request_instance = DisableWorkflowRequest.from_json(json)
# print the JSON string representation of the object
print(DisableWorkflowRequest.to_json())

# convert the object into a dict
disable_workflow_request_dict = disable_workflow_request_instance.to_dict()
# create an instance of DisableWorkflowRequest from a dict
disable_workflow_request_from_dict = DisableWorkflowRequest.from_dict(disable_workflow_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


