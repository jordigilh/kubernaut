# WorkflowSearchResult

Flat response structure (DD-WORKFLOW-002 v3.0)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_id** | **str** | UUID primary key (DD-WORKFLOW-002 v3.0) | 
**title** | **str** | Human-readable workflow name | 
**description** | [**StructuredDescription**](StructuredDescription.md) |  | 
**schema_version** | **str** | Schema format version (e.g., 1.0, 1.1). #255 | [optional] 
**schema_image** | **str** | OCI image used to extract the workflow schema | [optional] 
**schema_digest** | **str** | OCI schema image digest | [optional] 
**execution_bundle** | **str** | OCI execution bundle reference (digest-pinned) | [optional] 
**execution_bundle_digest** | **str** | OCI execution bundle digest | [optional] 
**execution_engine** | **str** | Execution engine (tekton, job, ansible) | [optional] 
**service_account_name** | **str** | Per-workflow ServiceAccount name (DD-WE-005 v2.0). Omitted if not set. | [optional] 
**confidence** | **float** | Normalized label score (0.0-1.0) | 
**label_boost** | **float** | Boost from matching DetectedLabels | [optional] 
**label_penalty** | **float** | Penalty from conflicting DetectedLabels | [optional] 
**final_score** | **float** | Final normalized score (same as confidence) | 
**rank** | **int** | Position in result set (1-based) | 
**custom_labels** | **Dict[str, List[str]]** | Customer-defined labels (DD-WORKFLOW-001 v1.5) - subdomain-based format | [optional] 
**detected_labels** | [**DetectedLabels**](DetectedLabels.md) |  | [optional] 
**parameters** | **Dict[str, object]** | Workflow parameter schema (JSONB) - describes expected parameters | [optional] 

## Example

```python
from datastorage.models.workflow_search_result import WorkflowSearchResult

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowSearchResult from a JSON string
workflow_search_result_instance = WorkflowSearchResult.from_json(json)
# print the JSON string representation of the object
print WorkflowSearchResult.to_json()

# convert the object into a dict
workflow_search_result_dict = workflow_search_result_instance.to_dict()
# create an instance of WorkflowSearchResult from a dict
workflow_search_result_form_dict = workflow_search_result.from_dict(workflow_search_result_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


