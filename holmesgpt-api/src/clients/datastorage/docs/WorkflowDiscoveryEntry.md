# WorkflowDiscoveryEntry

Workflow summary for discovery (Step 2) - no parameter schema, no scores

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_id** | **str** | UUID primary key | 
**workflow_name** | **str** | Human-readable workflow identifier (e.g., scale-conservative-v1) | 
**name** | **str** | Display name | 
**description** | **str** | Workflow description for LLM comparison | 
**version** | **str** | Semantic version | 
**container_image** | **str** | OCI image reference | 
**execution_engine** | **str** | Execution engine (tekton, job) | [optional] 
**actual_success_rate** | **float** | Historical success rate (0.0-1.0) | [optional] 
**total_executions** | **int** | Total times this workflow has been executed | [optional] 

## Example

```python
from datastorage.models.workflow_discovery_entry import WorkflowDiscoveryEntry

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowDiscoveryEntry from a JSON string
workflow_discovery_entry_instance = WorkflowDiscoveryEntry.from_json(json)
# print the JSON string representation of the object
print WorkflowDiscoveryEntry.to_json()

# convert the object into a dict
workflow_discovery_entry_dict = workflow_discovery_entry_instance.to_dict()
# create an instance of WorkflowDiscoveryEntry from a dict
workflow_discovery_entry_form_dict = workflow_discovery_entry.from_dict(workflow_discovery_entry_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


