# WorkflowBreakdownItem


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_id** | **str** | Workflow identifier | 
**workflow_version** | **str** | Workflow version | 
**executions** | **int** | Number of times this workflow was used | 
**success_rate** | **float** | Success rate for this specific workflow | 

## Example

```python
from datastorage.models.workflow_breakdown_item import WorkflowBreakdownItem

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowBreakdownItem from a JSON string
workflow_breakdown_item_instance = WorkflowBreakdownItem.from_json(json)
# print the JSON string representation of the object
print(WorkflowBreakdownItem.to_json())

# convert the object into a dict
workflow_breakdown_item_dict = workflow_breakdown_item_instance.to_dict()
# create an instance of WorkflowBreakdownItem from a dict
workflow_breakdown_item_from_dict = WorkflowBreakdownItem.from_dict(workflow_breakdown_item_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


