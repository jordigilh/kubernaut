# ActionTypeCatalogUpdatedPayload

DS audit payload when an action type description is updated (SOC2: old+new)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** |  | 
**action_type** | **str** |  | 
**old_description** | [**ActionTypeDescriptionPayload**](ActionTypeDescriptionPayload.md) |  | 
**new_description** | [**ActionTypeDescriptionPayload**](ActionTypeDescriptionPayload.md) |  | 
**updated_by** | **str** |  | 
**updated_fields** | **List[str]** |  | 

## Example

```python
from datastorage.models.action_type_catalog_updated_payload import ActionTypeCatalogUpdatedPayload

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeCatalogUpdatedPayload from a JSON string
action_type_catalog_updated_payload_instance = ActionTypeCatalogUpdatedPayload.from_json(json)
# print the JSON string representation of the object
print ActionTypeCatalogUpdatedPayload.to_json()

# convert the object into a dict
action_type_catalog_updated_payload_dict = action_type_catalog_updated_payload_instance.to_dict()
# create an instance of ActionTypeCatalogUpdatedPayload from a dict
action_type_catalog_updated_payload_form_dict = action_type_catalog_updated_payload.from_dict(action_type_catalog_updated_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


