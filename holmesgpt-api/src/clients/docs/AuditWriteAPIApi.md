# datastorage.AuditWriteAPIApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**create_audit_event**](AuditWriteAPIApi.md#create_audit_event) | **POST** /api/v1/audit/events | Create unified audit event
[**create_audit_events_batch**](AuditWriteAPIApi.md#create_audit_events_batch) | **POST** /api/v1/audit/events/batch | Create audit events batch
[**create_notification_audit**](AuditWriteAPIApi.md#create_notification_audit) | **POST** /api/v1/audit/notifications | Create notification audit record
[**query_audit_events**](AuditWriteAPIApi.md#query_audit_events) | **GET** /api/v1/audit/events | Query audit events


# **create_audit_event**
> AuditEventResponse create_audit_event(audit_event_request)

Create unified audit event

Persists a unified audit event to the audit_events table (ADR-034).

**Business Requirement**: BR-STORAGE-033 (Unified audit trail)

**Behavior**:
- Success: Returns 201 Created with event_id
- Validation Error: Returns 400 Bad Request (RFC 7807)
- Database Error: Returns 202 Accepted (DLQ fallback, DD-009)


### Example


```python
import datastorage
from datastorage.models.audit_event_request import AuditEventRequest
from datastorage.models.audit_event_response import AuditEventResponse
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.AuditWriteAPIApi(api_client)
    audit_event_request = datastorage.AuditEventRequest() # AuditEventRequest | 

    try:
        # Create unified audit event
        api_response = api_instance.create_audit_event(audit_event_request)
        print("The response of AuditWriteAPIApi->create_audit_event:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AuditWriteAPIApi->create_audit_event: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **audit_event_request** | [**AuditEventRequest**](AuditEventRequest.md)|  | 

### Return type

[**AuditEventResponse**](AuditEventResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**201** | Audit event created successfully |  -  |
**202** | Database write failed, queued to DLQ |  -  |
**400** | Validation error |  -  |
**500** | Internal server error |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **create_audit_events_batch**
> BatchAuditEventResponse create_audit_events_batch(audit_event_request)

Create audit events batch

Write multiple audit events in a single request

### Example


```python
import datastorage
from datastorage.models.audit_event_request import AuditEventRequest
from datastorage.models.batch_audit_event_response import BatchAuditEventResponse
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.AuditWriteAPIApi(api_client)
    audit_event_request = [datastorage.AuditEventRequest()] # List[AuditEventRequest] | 

    try:
        # Create audit events batch
        api_response = api_instance.create_audit_events_batch(audit_event_request)
        print("The response of AuditWriteAPIApi->create_audit_events_batch:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AuditWriteAPIApi->create_audit_events_batch: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **audit_event_request** | [**List[AuditEventRequest]**](AuditEventRequest.md)|  | 

### Return type

[**BatchAuditEventResponse**](BatchAuditEventResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**201** | Batch created successfully |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **create_notification_audit**
> NotificationAuditResponse create_notification_audit(notification_audit)

Create notification audit record

Persists a notification delivery attempt audit record.

**Business Requirement**: BR-STORAGE-001 (Notification audit persistence)

**Behavior**:
- Success: Returns 201 Created with created record
- Validation Error: Returns 400 Bad Request (RFC 7807)
- Duplicate: Returns 409 Conflict (RFC 7807)
- Database Error: Returns 202 Accepted (DLQ fallback, DD-009)

**Metrics Emitted** (GAP-10):
- `datastorage_audit_traces_total{service="notification", status="success|failure|dlq_fallback"}`
- `datastorage_audit_lag_seconds{service="notification"}`
- `datastorage_write_duration_seconds{table="notification_audit"}`
- `datastorage_validation_failures_total{field="...", reason="..."}`


### Example


```python
import datastorage
from datastorage.models.notification_audit import NotificationAudit
from datastorage.models.notification_audit_response import NotificationAuditResponse
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.AuditWriteAPIApi(api_client)
    notification_audit = {"remediation_id":"remediation-123","notification_id":"notification-456","recipient":"ops-team@example.com","channel":"email","message_summary":"Incident alert: High CPU usage on pod xyz","status":"sent","sent_at":"2025-11-03T12:00:00Z","delivery_status":"200 OK","escalation_level":0} # NotificationAudit | 

    try:
        # Create notification audit record
        api_response = api_instance.create_notification_audit(notification_audit)
        print("The response of AuditWriteAPIApi->create_notification_audit:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AuditWriteAPIApi->create_notification_audit: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **notification_audit** | [**NotificationAudit**](NotificationAudit.md)|  | 

### Return type

[**NotificationAuditResponse**](NotificationAuditResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**201** | Audit record created successfully in PostgreSQL. Record is immediately available for queries.  |  -  |
**202** | Database write failed, audit record queued to Dead Letter Queue (DD-009). Record will be processed asynchronously.  **Reason**: PostgreSQL temporarily unavailable or under load. **Recovery**: DLQ consumer will retry write every 30 seconds.  |  -  |
**400** | Validation error - request body failed validation. Returns RFC 7807 Problem Details format (BR-STORAGE-024).  |  -  |
**409** | Conflict - duplicate notification_id. Notification audit records use notification_id as unique constraint.  |  -  |
**500** | Internal server error - both database and DLQ unavailable. This indicates a critical system failure requiring immediate attention.  |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **query_audit_events**
> AuditEventsQueryResponse query_audit_events(event_type=event_type, correlation_id=correlation_id, limit=limit, offset=offset)

Query audit events

Query audit events with filters and pagination

### Example


```python
import datastorage
from datastorage.models.audit_events_query_response import AuditEventsQueryResponse
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)


# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.AuditWriteAPIApi(api_client)
    event_type = 'event_type_example' # str |  (optional)
    correlation_id = 'correlation_id_example' # str |  (optional)
    limit = 50 # int |  (optional) (default to 50)
    offset = 0 # int |  (optional) (default to 0)

    try:
        # Query audit events
        api_response = api_instance.query_audit_events(event_type=event_type, correlation_id=correlation_id, limit=limit, offset=offset)
        print("The response of AuditWriteAPIApi->query_audit_events:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AuditWriteAPIApi->query_audit_events: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **event_type** | **str**|  | [optional] 
 **correlation_id** | **str**|  | [optional] 
 **limit** | **int**|  | [optional] [default to 50]
 **offset** | **int**|  | [optional] [default to 0]

### Return type

[**AuditEventsQueryResponse**](AuditEventsQueryResponse.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Query results |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

