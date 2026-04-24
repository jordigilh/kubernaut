# ActionTypeCatalogDisableDeniedPayload

DS audit payload when disable is denied due to active workflow dependencies

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** |  | 
**action_type** | **str** |  | 
**denied_reason** | **str** |  | 
**dependent_workflow_count** | **int** |  | 
**dependent_workflows** | **List[str]** |  | 
**requested_by** | **str** |  | 

## Example

```python
from datastorage.models.action_type_catalog_disable_denied_payload import ActionTypeCatalogDisableDeniedPayload

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeCatalogDisableDeniedPayload from a JSON string
action_type_catalog_disable_denied_payload_instance = ActionTypeCatalogDisableDeniedPayload.from_json(json)
# print the JSON string representation of the object
print ActionTypeCatalogDisableDeniedPayload.to_json()

# convert the object into a dict
action_type_catalog_disable_denied_payload_dict = action_type_catalog_disable_denied_payload_instance.to_dict()
# create an instance of ActionTypeCatalogDisableDeniedPayload from a dict
action_type_catalog_disable_denied_payload_form_dict = action_type_catalog_disable_denied_payload.from_dict(action_type_catalog_disable_denied_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


