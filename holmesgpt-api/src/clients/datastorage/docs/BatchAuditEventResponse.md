# BatchAuditEventResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_ids** | **List[str]** |  | [optional] 
**message** | **str** |  | [optional] 

## Example

```python
from datastorage.models.batch_audit_event_response import BatchAuditEventResponse

# TODO update the JSON string below
json = "{}"
# create an instance of BatchAuditEventResponse from a JSON string
batch_audit_event_response_instance = BatchAuditEventResponse.from_json(json)
# print the JSON string representation of the object
print BatchAuditEventResponse.to_json()

# convert the object into a dict
batch_audit_event_response_dict = batch_audit_event_response_instance.to_dict()
# create an instance of BatchAuditEventResponse from a dict
batch_audit_event_response_form_dict = batch_audit_event_response.from_dict(batch_audit_event_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


