# WorkflowSearchFilters


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**signal_name** | **str** | Signal name (mandatory: OOMKilled, CrashLoopBackOff, etc.) | 
**severity** | **str** | Severity level (mandatory: critical, high, medium, low) | 
**component** | **str** | Component type (mandatory: pod, node, deployment, etc.) | 
**environment** | **str** | Environment filter (mandatory, single value from Signal Processing) | 
**priority** | **str** | Priority level (mandatory: P0, P1, P2, P3) | 
**custom_labels** | **Dict[str, List[str]]** | Customer-defined labels (DD-WORKFLOW-001 v1.5) - subdomain-based format | [optional] 
**detected_labels** | [**DetectedLabels**](DetectedLabels.md) |  | [optional] 
**status** | **List[str]** | Workflow lifecycle status filter | [optional] 

## Example

```python
from datastorage.models.workflow_search_filters import WorkflowSearchFilters

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowSearchFilters from a JSON string
workflow_search_filters_instance = WorkflowSearchFilters.from_json(json)
# print the JSON string representation of the object
print WorkflowSearchFilters.to_json()

# convert the object into a dict
workflow_search_filters_dict = workflow_search_filters_instance.to_dict()
# create an instance of WorkflowSearchFilters from a dict
workflow_search_filters_form_dict = workflow_search_filters.from_dict(workflow_search_filters_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


