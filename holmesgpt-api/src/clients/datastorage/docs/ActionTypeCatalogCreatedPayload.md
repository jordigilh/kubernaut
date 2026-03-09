# ActionTypeCatalogCreatedPayload

DS audit payload when a new action type is created or re-enabled

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** |  | 
**action_type** | **str** |  | 
**description** | [**ActionTypeDescriptionPayload**](ActionTypeDescriptionPayload.md) |  | 
**registered_by** | **str** |  | 
**was_reenabled** | **bool** |  | 

## Example

```python
from datastorage.models.action_type_catalog_created_payload import ActionTypeCatalogCreatedPayload

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeCatalogCreatedPayload from a JSON string
action_type_catalog_created_payload_instance = ActionTypeCatalogCreatedPayload.from_json(json)
# print the JSON string representation of the object
print ActionTypeCatalogCreatedPayload.to_json()

# convert the object into a dict
action_type_catalog_created_payload_dict = action_type_catalog_created_payload_instance.to_dict()
# create an instance of ActionTypeCatalogCreatedPayload from a dict
action_type_catalog_created_payload_form_dict = action_type_catalog_created_payload.from_dict(action_type_catalog_created_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


