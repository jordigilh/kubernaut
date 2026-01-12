# SelectedWorkflowSummary

Summary of the workflow that was executed and failed

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_id** | **str** | Workflow identifier that was executed | 
**version** | **str** | Workflow version | 
**container_image** | **str** | Container image used for execution | 
**parameters** | **Dict[str, str]** | Parameters passed to workflow | [optional] 
**rationale** | **str** | Why this workflow was originally selected | 

## Example

```python
from holmesgpt_api_client.models.selected_workflow_summary import SelectedWorkflowSummary

# TODO update the JSON string below
json = "{}"
# create an instance of SelectedWorkflowSummary from a JSON string
selected_workflow_summary_instance = SelectedWorkflowSummary.from_json(json)
# print the JSON string representation of the object
print(SelectedWorkflowSummary.to_json())

# convert the object into a dict
selected_workflow_summary_dict = selected_workflow_summary_instance.to_dict()
# create an instance of SelectedWorkflowSummary from a dict
selected_workflow_summary_from_dict = SelectedWorkflowSummary.from_dict(selected_workflow_summary_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


