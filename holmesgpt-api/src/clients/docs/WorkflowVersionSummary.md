# WorkflowVersionSummary


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**version** | **str** |  | 
**status** | **str** |  | 
**is_latest_version** | **bool** |  | 
**created_at** | **datetime** |  | 

## Example

```python
from datastorage.models.workflow_version_summary import WorkflowVersionSummary

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowVersionSummary from a JSON string
workflow_version_summary_instance = WorkflowVersionSummary.from_json(json)
# print the JSON string representation of the object
print(WorkflowVersionSummary.to_json())

# convert the object into a dict
workflow_version_summary_dict = workflow_version_summary_instance.to_dict()
# create an instance of WorkflowVersionSummary from a dict
workflow_version_summary_from_dict = WorkflowVersionSummary.from_dict(workflow_version_summary_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


