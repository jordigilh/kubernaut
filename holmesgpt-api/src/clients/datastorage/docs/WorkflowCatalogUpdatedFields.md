# WorkflowCatalogUpdatedFields

Fields that were updated in the workflow catalog

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**status** | **str** | Updated workflow status | [optional] 
**disabled_by** | **str** | User who disabled the workflow (only for status&#x3D;disabled) | [optional] 
**disabled_reason** | **str** | Reason for disabling (only for status&#x3D;disabled) | [optional] 
**version** | **str** | Updated version | [optional] 
**description** | **str** | Updated description | [optional] 

## Example

```python
from datastorage.models.workflow_catalog_updated_fields import WorkflowCatalogUpdatedFields

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowCatalogUpdatedFields from a JSON string
workflow_catalog_updated_fields_instance = WorkflowCatalogUpdatedFields.from_json(json)
# print the JSON string representation of the object
print WorkflowCatalogUpdatedFields.to_json()

# convert the object into a dict
workflow_catalog_updated_fields_dict = workflow_catalog_updated_fields_instance.to_dict()
# create an instance of WorkflowCatalogUpdatedFields from a dict
workflow_catalog_updated_fields_form_dict = workflow_catalog_updated_fields.from_dict(workflow_catalog_updated_fields_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


