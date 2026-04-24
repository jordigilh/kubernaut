# ActionTypeDisableRequest

Request body for disabling an action type

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**disabled_by** | **str** | Identity of who is disabling | 

## Example

```python
from datastorage.models.action_type_disable_request import ActionTypeDisableRequest

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeDisableRequest from a JSON string
action_type_disable_request_instance = ActionTypeDisableRequest.from_json(json)
# print the JSON string representation of the object
print ActionTypeDisableRequest.to_json()

# convert the object into a dict
action_type_disable_request_dict = action_type_disable_request_instance.to_dict()
# create an instance of ActionTypeDisableRequest from a dict
action_type_disable_request_form_dict = action_type_disable_request.from_dict(action_type_disable_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


