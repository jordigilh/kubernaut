# WorkflowDiscoveryResponse

Response for Step 2: list workflows for an action type (DD-WORKFLOW-016)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**action_type** | **str** | The action type these workflows belong to | 
**workflows** | [**List[WorkflowDiscoveryEntry]**](WorkflowDiscoveryEntry.md) |  | 
**pagination** | [**PaginationMetadata**](PaginationMetadata.md) |  | 

## Example

```python
from datastorage.models.workflow_discovery_response import WorkflowDiscoveryResponse

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowDiscoveryResponse from a JSON string
workflow_discovery_response_instance = WorkflowDiscoveryResponse.from_json(json)
# print the JSON string representation of the object
print WorkflowDiscoveryResponse.to_json()

# convert the object into a dict
workflow_discovery_response_dict = workflow_discovery_response_instance.to_dict()
# create an instance of WorkflowDiscoveryResponse from a dict
workflow_discovery_response_form_dict = workflow_discovery_response.from_dict(workflow_discovery_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


