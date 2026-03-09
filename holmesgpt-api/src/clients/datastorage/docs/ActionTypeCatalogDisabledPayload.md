# ActionTypeCatalogDisabledPayload

DS audit payload when an action type is soft-disabled

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** |  | 
**action_type** | **str** |  | 
**disabled_by** | **str** |  | 
**disabled_at** | **datetime** |  | 

## Example

```python
from datastorage.models.action_type_catalog_disabled_payload import ActionTypeCatalogDisabledPayload

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeCatalogDisabledPayload from a JSON string
action_type_catalog_disabled_payload_instance = ActionTypeCatalogDisabledPayload.from_json(json)
# print the JSON string representation of the object
print ActionTypeCatalogDisabledPayload.to_json()

# convert the object into a dict
action_type_catalog_disabled_payload_dict = action_type_catalog_disabled_payload_instance.to_dict()
# create an instance of ActionTypeCatalogDisabledPayload from a dict
action_type_catalog_disabled_payload_form_dict = action_type_catalog_disabled_payload.from_dict(action_type_catalog_disabled_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


