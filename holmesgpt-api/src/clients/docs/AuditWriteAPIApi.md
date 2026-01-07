# datastorage.AuditWriteAPIApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**create_audit_event**](AuditWriteAPIApi.md#create_audit_event) | **POST** /api/v1/audit/events | Create unified audit event
[**create_audit_events_batch**](AuditWriteAPIApi.md#create_audit_events_batch) | **POST** /api/v1/audit/events/batch | Create audit events batch
[**create_notification_audit**](AuditWriteAPIApi.md#create_notification_audit) | **POST** /api/v1/audit/notifications | Create notification audit record
[**list_legal_holds**](AuditWriteAPIApi.md#list_legal_holds) | **GET** /api/v1/audit/legal-hold | List all active legal holds
[**place_legal_hold**](AuditWriteAPIApi.md#place_legal_hold) | **POST** /api/v1/audit/legal-hold | Place legal hold on audit events
[**query_audit_events**](AuditWriteAPIApi.md#query_audit_events) | **GET** /api/v1/audit/events | Query audit events
[**release_legal_hold**](AuditWriteAPIApi.md#release_legal_hold) | **DELETE** /api/v1/audit/legal-hold/{correlation_id} | Release legal hold on audit events


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

# **list_legal_holds**
> ListLegalHolds200Response list_legal_holds()

List all active legal holds

Returns a list of all active legal holds across all audit events.

**Business Requirement**: BR-AUDIT-006 (Legal Hold & Retention)
**SOC2 Gap**: Gap #8 (Legal Hold enforcement)

**Behavior**:
- Success: Returns 200 OK with array of active legal holds
- No holds: Returns empty array

**Authorization**: No authentication required (read-only operation)

**Metrics Emitted**:
- `datastorage_legal_hold_successes_total{operation="list"}`


### Example


```python
import datastorage
from datastorage.models.list_legal_holds200_response import ListLegalHolds200Response
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

    try:
        # List all active legal holds
        api_response = api_instance.list_legal_holds()
        print("The response of AuditWriteAPIApi->list_legal_holds:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AuditWriteAPIApi->list_legal_holds: %s\n" % e)
```



### Parameters

This endpoint does not need any parameter.

### Return type

[**ListLegalHolds200Response**](ListLegalHolds200Response.md)

### Authorization

No authorization required

### HTTP request headers

 - **Content-Type**: Not defined
 - **Accept**: application/json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | List of active legal holds |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **place_legal_hold**
> ListLegalHolds200ResponseHoldsInner place_legal_hold(place_legal_hold_request)

Place legal hold on audit events

Places a legal hold on all audit events for a given correlation_id.
Events with legal hold cannot be deleted (enforced by database trigger).

**Business Requirement**: BR-AUDIT-006 (Legal Hold & Retention)
**SOC2 Gap**: Gap #8 (Legal Hold enforcement for Sarbanes-Oxley, HIPAA)

**Behavior**:
- Success: Returns 200 OK with legal hold metadata
- Validation Error: Returns 400 Bad Request (RFC 7807)
- Not Found: Returns 404 Not Found if correlation_id doesn't exist
- Unauthorized: Returns 401 if X-User-ID header missing

**Authorization**: Requires X-User-ID header to track who placed the hold

**Metrics Emitted**:
- `datastorage_legal_hold_successes_total{operation="place"}`
- `datastorage_legal_hold_failures_total{reason="missing_correlation_id|unauthorized|..."}`


### Example

* Api Key Authentication (oauthProxyAuth):

```python
import datastorage
from datastorage.models.list_legal_holds200_response_holds_inner import ListLegalHolds200ResponseHoldsInner
from datastorage.models.place_legal_hold_request import PlaceLegalHoldRequest
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# The client must configure the authentication and authorization parameters
# in accordance with the API server security policy.
# Examples for each auth method are provided below, use the example that
# satisfies your auth use case.

# Configure API key authorization: oauthProxyAuth
configuration.api_key['oauthProxyAuth'] = os.environ["API_KEY"]

# Uncomment below to setup prefix (e.g. Bearer) for API key, if needed
# configuration.api_key_prefix['oauthProxyAuth'] = 'Bearer'

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.AuditWriteAPIApi(api_client)
    place_legal_hold_request = datastorage.PlaceLegalHoldRequest() # PlaceLegalHoldRequest | 

    try:
        # Place legal hold on audit events
        api_response = api_instance.place_legal_hold(place_legal_hold_request)
        print("The response of AuditWriteAPIApi->place_legal_hold:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AuditWriteAPIApi->place_legal_hold: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **place_legal_hold_request** | [**PlaceLegalHoldRequest**](PlaceLegalHoldRequest.md)|  | 

### Return type

[**ListLegalHolds200ResponseHoldsInner**](ListLegalHolds200ResponseHoldsInner.md)

### Authorization

[oauthProxyAuth](../README.md#oauthProxyAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Legal hold placed successfully |  -  |
**400** | Validation error |  -  |
**401** | Unauthorized - X-User-ID header required |  -  |
**404** | Not Found - correlation_id doesn&#39;t exist |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

# **query_audit_events**
> AuditEventsQueryResponse query_audit_events(event_type=event_type, event_category=event_category, event_outcome=event_outcome, severity=severity, correlation_id=correlation_id, since=since, until=until, limit=limit, offset=offset)

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
    event_type = 'notification.message.sent' # str | Filter by event type (ADR-034) (optional)
    event_category = 'notification' # str | Filter by event category (ADR-034) (optional)
    event_outcome = 'success' # str | Filter by event outcome (ADR-034) (optional)
    severity = 'critical' # str | Filter by severity level (optional)
    correlation_id = 'rr-2025-001' # str | Filter by correlation ID (optional)
    since = '24h' # str | Start time (relative like \"24h\" or absolute RFC3339) (optional)
    until = '2025-12-18T23:59:59Z' # str | End time (absolute RFC3339) (optional)
    limit = 50 # int | Page size (1-1000, default 50) (optional) (default to 50)
    offset = 0 # int | Page offset (default 0) (optional) (default to 0)

    try:
        # Query audit events
        api_response = api_instance.query_audit_events(event_type=event_type, event_category=event_category, event_outcome=event_outcome, severity=severity, correlation_id=correlation_id, since=since, until=until, limit=limit, offset=offset)
        print("The response of AuditWriteAPIApi->query_audit_events:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AuditWriteAPIApi->query_audit_events: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **event_type** | **str**| Filter by event type (ADR-034) | [optional] 
 **event_category** | **str**| Filter by event category (ADR-034) | [optional] 
 **event_outcome** | **str**| Filter by event outcome (ADR-034) | [optional] 
 **severity** | **str**| Filter by severity level | [optional] 
 **correlation_id** | **str**| Filter by correlation ID | [optional] 
 **since** | **str**| Start time (relative like \&quot;24h\&quot; or absolute RFC3339) | [optional] 
 **until** | **str**| End time (absolute RFC3339) | [optional] 
 **limit** | **int**| Page size (1-1000, default 50) | [optional] [default to 50]
 **offset** | **int**| Page offset (default 0) | [optional] [default to 0]

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

# **release_legal_hold**
> ReleaseLegalHold200Response release_legal_hold(correlation_id, release_legal_hold_request)

Release legal hold on audit events

Releases a legal hold on all audit events for a given correlation_id.
Events can be deleted after legal hold is released.

**Business Requirement**: BR-AUDIT-006 (Legal Hold & Retention)
**SOC2 Gap**: Gap #8 (Legal Hold enforcement)

**Behavior**:
- Success: Returns 200 OK with release metadata
- Validation Error: Returns 400 Bad Request (RFC 7807)
- Not Found: Returns 404 Not Found if legal hold doesn't exist
- Unauthorized: Returns 401 if X-User-ID header missing

**Authorization**: Requires X-User-ID header to track who released the hold

**Metrics Emitted**:
- `datastorage_legal_hold_successes_total{operation="release"}`
- `datastorage_legal_hold_failures_total{reason="unauthorized|not_found|..."}`


### Example

* Api Key Authentication (oauthProxyAuth):

```python
import datastorage
from datastorage.models.release_legal_hold200_response import ReleaseLegalHold200Response
from datastorage.models.release_legal_hold_request import ReleaseLegalHoldRequest
from datastorage.rest import ApiException
from pprint import pprint

# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# The client must configure the authentication and authorization parameters
# in accordance with the API server security policy.
# Examples for each auth method are provided below, use the example that
# satisfies your auth use case.

# Configure API key authorization: oauthProxyAuth
configuration.api_key['oauthProxyAuth'] = os.environ["API_KEY"]

# Uncomment below to setup prefix (e.g. Bearer) for API key, if needed
# configuration.api_key_prefix['oauthProxyAuth'] = 'Bearer'

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = datastorage.AuditWriteAPIApi(api_client)
    correlation_id = 'correlation_id_example' # str | Correlation ID of events to release legal hold from
    release_legal_hold_request = datastorage.ReleaseLegalHoldRequest() # ReleaseLegalHoldRequest | 

    try:
        # Release legal hold on audit events
        api_response = api_instance.release_legal_hold(correlation_id, release_legal_hold_request)
        print("The response of AuditWriteAPIApi->release_legal_hold:\n")
        pprint(api_response)
    except Exception as e:
        print("Exception when calling AuditWriteAPIApi->release_legal_hold: %s\n" % e)
```



### Parameters


Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
 **correlation_id** | **str**| Correlation ID of events to release legal hold from | 
 **release_legal_hold_request** | [**ReleaseLegalHoldRequest**](ReleaseLegalHoldRequest.md)|  | 

### Return type

[**ReleaseLegalHold200Response**](ReleaseLegalHold200Response.md)

### Authorization

[oauthProxyAuth](../README.md#oauthProxyAuth)

### HTTP request headers

 - **Content-Type**: application/json
 - **Accept**: application/json, application/problem+json

### HTTP response details

| Status code | Description | Response headers |
|-------------|-------------|------------------|
**200** | Legal hold released successfully |  -  |
**400** | Validation error |  -  |
**401** | Unauthorized - X-User-ID header required |  -  |
**404** | Not Found - legal hold doesn&#39;t exist for correlation_id |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to Model list]](../README.md#documentation-for-models) [[Back to README]](../README.md)

