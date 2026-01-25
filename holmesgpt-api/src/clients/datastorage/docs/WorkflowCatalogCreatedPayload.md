# WorkflowCatalogCreatedPayload

Audit payload for workflow catalog creation (datastorage.workflow.created)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_id** | **str** | Unique workflow identifier (UUID) | 
**workflow_name** | **str** | Human-readable workflow name | 
**version** | **str** | Workflow version | 
**status** | **str** | Workflow status | 
**is_latest_version** | **bool** | Whether this is the latest version | 
**execution_engine** | **str** | Workflow execution engine | 
**name** | **str** | Display name | 
**description** | **str** | Workflow description | [optional] 
**labels** | **Dict[str, object]** | Workflow labels | [optional] 

## Example

```python
from datastorage.models.workflow_catalog_created_payload import WorkflowCatalogCreatedPayload

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowCatalogCreatedPayload from a JSON string
workflow_catalog_created_payload_instance = WorkflowCatalogCreatedPayload.from_json(json)
# print the JSON string representation of the object
print WorkflowCatalogCreatedPayload.to_json()

# convert the object into a dict
workflow_catalog_created_payload_dict = workflow_catalog_created_payload_instance.to_dict()
# create an instance of WorkflowCatalogCreatedPayload from a dict
workflow_catalog_created_payload_form_dict = workflow_catalog_created_payload.from_dict(workflow_catalog_created_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


