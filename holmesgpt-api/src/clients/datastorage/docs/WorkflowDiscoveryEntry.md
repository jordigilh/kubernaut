# WorkflowDiscoveryEntry

Workflow summary for discovery (Step 2) - no parameter schema, no scores

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_id** | **str** | UUID primary key | 
**workflow_name** | **str** | Human-readable workflow identifier (e.g., scale-conservative-v1) | 
**name** | **str** | Display name | 
**description** | [**StructuredDescription**](StructuredDescription.md) |  | 
**version** | **str** | Semantic version | 
**schema_version** | **str** | Schema format version (e.g., 1.0, 1.1). #255 | [optional] 
**schema_image** | **str** | OCI image used to extract the workflow schema | [optional] 
**execution_bundle** | **str** | OCI execution bundle reference (digest-pinned) | [optional] 
**execution_engine** | **str** | Execution engine (tekton, job) | [optional] 
**service_account_name** | **str** | Per-workflow ServiceAccount name (DD-WE-005 v2.0). Omitted if not set. | [optional] 

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


