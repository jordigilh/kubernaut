# AuditEventsQueryResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**data** | [**List[AuditEvent]**](AuditEvent.md) |  | [optional] 
**pagination** | [**AuditEventsQueryResponsePagination**](AuditEventsQueryResponsePagination.md) |  | [optional] 

## Example

```python
from datastorage.models.audit_events_query_response import AuditEventsQueryResponse

# TODO update the JSON string below
json = "{}"
# create an instance of AuditEventsQueryResponse from a JSON string
audit_events_query_response_instance = AuditEventsQueryResponse.from_json(json)
# print the JSON string representation of the object
print AuditEventsQueryResponse.to_json()

# convert the object into a dict
audit_events_query_response_dict = audit_events_query_response_instance.to_dict()
# create an instance of AuditEventsQueryResponse from a dict
audit_events_query_response_form_dict = audit_events_query_response.from_dict(audit_events_query_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


