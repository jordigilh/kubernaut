# AuditEventsQueryResponsePagination


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**limit** | **int** | Maximum number of events per page | [optional] 
**offset** | **int** | Number of events to skip | [optional] 
**total** | **int** | Total number of events matching the query | [optional] 
**has_more** | **bool** | Whether more results are available beyond current page | [optional] 

## Example

```python
from datastorage.models.audit_events_query_response_pagination import AuditEventsQueryResponsePagination

# TODO update the JSON string below
json = "{}"
# create an instance of AuditEventsQueryResponsePagination from a JSON string
audit_events_query_response_pagination_instance = AuditEventsQueryResponsePagination.from_json(json)
# print the JSON string representation of the object
print AuditEventsQueryResponsePagination.to_json()

# convert the object into a dict
audit_events_query_response_pagination_dict = audit_events_query_response_pagination_instance.to_dict()
# create an instance of AuditEventsQueryResponsePagination from a dict
audit_events_query_response_pagination_form_dict = audit_events_query_response_pagination.from_dict(audit_events_query_response_pagination_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


