# ActionTypeCatalogReenabledPayload

DS audit payload when a previously disabled action type is re-enabled

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** |  | 
**action_type** | **str** |  | 
**reenabled_by** | **str** |  | 
**previous_state** | **str** |  | 
**disabled_at** | **datetime** |  | 
**disabled_by** | **str** |  | 

## Example

```python
from datastorage.models.action_type_catalog_reenabled_payload import ActionTypeCatalogReenabledPayload

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeCatalogReenabledPayload from a JSON string
action_type_catalog_reenabled_payload_instance = ActionTypeCatalogReenabledPayload.from_json(json)
# print the JSON string representation of the object
print ActionTypeCatalogReenabledPayload.to_json()

# convert the object into a dict
action_type_catalog_reenabled_payload_dict = action_type_catalog_reenabled_payload_instance.to_dict()
# create an instance of ActionTypeCatalogReenabledPayload from a dict
action_type_catalog_reenabled_payload_form_dict = action_type_catalog_reenabled_payload.from_dict(action_type_catalog_reenabled_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


