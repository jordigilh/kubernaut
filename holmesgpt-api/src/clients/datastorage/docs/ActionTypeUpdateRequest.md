# ActionTypeUpdateRequest

Request body for updating action type description. updatedBy is optional — the audit trail (Phase 6a) captures the actor authoritatively.

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**description** | [**ActionTypeDescription**](ActionTypeDescription.md) |  | 
**updated_by** | **str** | Identity of who made the change | [optional] 

## Example

```python
from datastorage.models.action_type_update_request import ActionTypeUpdateRequest

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeUpdateRequest from a JSON string
action_type_update_request_instance = ActionTypeUpdateRequest.from_json(json)
# print the JSON string representation of the object
print ActionTypeUpdateRequest.to_json()

# convert the object into a dict
action_type_update_request_dict = action_type_update_request_instance.to_dict()
# create an instance of ActionTypeUpdateRequest from a dict
action_type_update_request_form_dict = action_type_update_request.from_dict(action_type_update_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


