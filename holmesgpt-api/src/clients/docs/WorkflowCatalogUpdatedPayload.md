# WorkflowCatalogUpdatedPayload

Audit payload for workflow catalog updates (datastorage.workflow.updated)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_id** | **UUID** | Unique workflow identifier (UUID) | 
**updated_fields** | [**WorkflowCatalogUpdatedFields**](WorkflowCatalogUpdatedFields.md) |  | 

## Example

```python
from datastorage.models.workflow_catalog_updated_payload import WorkflowCatalogUpdatedPayload

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowCatalogUpdatedPayload from a JSON string
workflow_catalog_updated_payload_instance = WorkflowCatalogUpdatedPayload.from_json(json)
# print the JSON string representation of the object
print(WorkflowCatalogUpdatedPayload.to_json())

# convert the object into a dict
workflow_catalog_updated_payload_dict = workflow_catalog_updated_payload_instance.to_dict()
# create an instance of WorkflowCatalogUpdatedPayload from a dict
workflow_catalog_updated_payload_from_dict = WorkflowCatalogUpdatedPayload.from_dict(workflow_catalog_updated_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


