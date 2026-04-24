# AuditEventResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_id** | **str** |  | 
**event_timestamp** | **datetime** |  | 
**message** | **str** |  | 

## Example

```python
from datastorage.models.audit_event_response import AuditEventResponse

# TODO update the JSON string below
json = "{}"
# create an instance of AuditEventResponse from a JSON string
audit_event_response_instance = AuditEventResponse.from_json(json)
# print the JSON string representation of the object
print AuditEventResponse.to_json()

# convert the object into a dict
audit_event_response_dict = audit_event_response_instance.to_dict()
# create an instance of AuditEventResponse from a dict
audit_event_response_form_dict = audit_event_response.from_dict(audit_event_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


