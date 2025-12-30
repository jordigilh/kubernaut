<a id="__pageTop"></a>
# datastorage.apis.tags.audit_write_api_api.AuditWriteAPIApi

All URIs are relative to *http://localhost:8080*

Method | HTTP request | Description
------------- | ------------- | -------------
[**create_audit_event**](#create_audit_event) | **post** /api/v1/audit/events | Create unified audit event
[**create_audit_events_batch**](#create_audit_events_batch) | **post** /api/v1/audit/events/batch | Create audit events batch
[**create_notification_audit**](#create_notification_audit) | **post** /api/v1/audit/notifications | Create notification audit record
[**query_audit_events**](#query_audit_events) | **get** /api/v1/audit/events | Query audit events

# **create_audit_event**
<a id="create_audit_event"></a>
> AuditEventResponse create_audit_event(audit_event_request)

Create unified audit event

Persists a unified audit event to the audit_events table (ADR-034).  **Business Requirement**: BR-STORAGE-033 (Unified audit trail)  **Behavior**: - Success: Returns 201 Created with event_id - Validation Error: Returns 400 Bad Request (RFC 7807) - Database Error: Returns 202 Accepted (DLQ fallback, DD-009) 

### Example

```python
import datastorage
from datastorage.apis.tags import audit_write_api_api
from datastorage.model.audit_event_response import AuditEventResponse
from datastorage.model.audit_event_request import AuditEventRequest
from datastorage.model.rfc7807_problem import RFC7807Problem
from pprint import pprint
# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = audit_write_api_api.AuditWriteAPIApi(api_client)

    # example passing only required values which don't have defaults set
    body = AuditEventRequest(
        version="1.0",
        event_type="gateway.signal.received",
        event_timestamp="2025-12-13T02:00Z",
        event_category="gateway",
        event_action="received",
        event_outcome="success",
        actor_type="service",
        actor_id="gateway-service",
        resource_type="Signal",
        resource_id="fp-abc123",
        correlation_id="rr-2025-001",
        parent_event_id="parent_event_id_example",
        namespace="namespace_example",
        cluster_name="cluster_name_example",
        severity="severity_example",
        duration_ms=1,
        event_data=None,
    )
    try:
        # Create unified audit event
        api_response = api_instance.create_audit_event(
            body=body,
        )
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling AuditWriteAPIApi->create_audit_event: %s\n" % e)
```
### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
body | typing.Union[SchemaForRequestBodyApplicationJson] | required |
content_type | str | optional, default is 'application/json' | Selects the schema and serialization of the request body
accept_content_types | typing.Tuple[str] | default is ('application/json', 'application/problem+json', ) | Tells the server the content type(s) that are accepted by the client
stream | bool | default is False | if True then the response.content will be streamed and loaded from a file like object. When downloading a file, set this to True to force the code to deserialize the content to a FileSchema file
timeout | typing.Optional[typing.Union[int, typing.Tuple]] | default is None | the timeout used by the rest client
skip_deserialization | bool | default is False | when True, headers and body will be unset and an instance of api_client.ApiResponseWithoutDeserialization will be returned

### body

# SchemaForRequestBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**AuditEventRequest**](../../models/AuditEventRequest.md) |  | 


### Return Types, Responses

Code | Class | Description
------------- | ------------- | -------------
n/a | api_client.ApiResponseWithoutDeserialization | When skip_deserialization is True this response is returned
201 | [ApiResponseFor201](#create_audit_event.ApiResponseFor201) | Audit event created successfully
202 | [ApiResponseFor202](#create_audit_event.ApiResponseFor202) | Database write failed, queued to DLQ
400 | [ApiResponseFor400](#create_audit_event.ApiResponseFor400) | Validation error
500 | [ApiResponseFor500](#create_audit_event.ApiResponseFor500) | Internal server error

#### create_audit_event.ApiResponseFor201
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor201ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor201ResponseBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**AuditEventResponse**](../../models/AuditEventResponse.md) |  | 


#### create_audit_event.ApiResponseFor202
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor202ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor202ResponseBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**AuditEventResponse**](../../models/AuditEventResponse.md) |  | 


#### create_audit_event.ApiResponseFor400
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor400ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor400ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


#### create_audit_event.ApiResponseFor500
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor500ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor500ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


### Authorization

No authorization required

[[Back to top]](#__pageTop) [[Back to API list]](../../../README.md#documentation-for-api-endpoints) [[Back to Model list]](../../../README.md#documentation-for-models) [[Back to README]](../../../README.md)

# **create_audit_events_batch**
<a id="create_audit_events_batch"></a>
> BatchAuditEventResponse create_audit_events_batch(audit_event_request)

Create audit events batch

Write multiple audit events in a single request

### Example

```python
import datastorage
from datastorage.apis.tags import audit_write_api_api
from datastorage.model.audit_event_request import AuditEventRequest
from datastorage.model.batch_audit_event_response import BatchAuditEventResponse
from pprint import pprint
# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = audit_write_api_api.AuditWriteAPIApi(api_client)

    # example passing only required values which don't have defaults set
    body = [
        AuditEventRequest(
            version="1.0",
            event_type="gateway.signal.received",
            event_timestamp="2025-12-13T02:00Z",
            event_category="gateway",
            event_action="received",
            event_outcome="success",
            actor_type="service",
            actor_id="gateway-service",
            resource_type="Signal",
            resource_id="fp-abc123",
            correlation_id="rr-2025-001",
            parent_event_id="parent_event_id_example",
            namespace="namespace_example",
            cluster_name="cluster_name_example",
            severity="severity_example",
            duration_ms=1,
            event_data=None,
        )
    ]
    try:
        # Create audit events batch
        api_response = api_instance.create_audit_events_batch(
            body=body,
        )
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling AuditWriteAPIApi->create_audit_events_batch: %s\n" % e)
```
### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
body | typing.Union[SchemaForRequestBodyApplicationJson] | required |
content_type | str | optional, default is 'application/json' | Selects the schema and serialization of the request body
accept_content_types | typing.Tuple[str] | default is ('application/json', ) | Tells the server the content type(s) that are accepted by the client
stream | bool | default is False | if True then the response.content will be streamed and loaded from a file like object. When downloading a file, set this to True to force the code to deserialize the content to a FileSchema file
timeout | typing.Optional[typing.Union[int, typing.Tuple]] | default is None | the timeout used by the rest client
skip_deserialization | bool | default is False | when True, headers and body will be unset and an instance of api_client.ApiResponseWithoutDeserialization will be returned

### body

# SchemaForRequestBodyApplicationJson

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
list, tuple,  | tuple,  |  | 

### Tuple Items
Class Name | Input Type | Accessed Type | Description | Notes
------------- | ------------- | ------------- | ------------- | -------------
[**AuditEventRequest**]({{complexTypePrefix}}AuditEventRequest.md) | [**AuditEventRequest**]({{complexTypePrefix}}AuditEventRequest.md) | [**AuditEventRequest**]({{complexTypePrefix}}AuditEventRequest.md) |  | 

### Return Types, Responses

Code | Class | Description
------------- | ------------- | -------------
n/a | api_client.ApiResponseWithoutDeserialization | When skip_deserialization is True this response is returned
201 | [ApiResponseFor201](#create_audit_events_batch.ApiResponseFor201) | Batch created successfully

#### create_audit_events_batch.ApiResponseFor201
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor201ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor201ResponseBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**BatchAuditEventResponse**](../../models/BatchAuditEventResponse.md) |  | 


### Authorization

No authorization required

[[Back to top]](#__pageTop) [[Back to API list]](../../../README.md#documentation-for-api-endpoints) [[Back to Model list]](../../../README.md#documentation-for-models) [[Back to README]](../../../README.md)

# **create_notification_audit**
<a id="create_notification_audit"></a>
> NotificationAuditResponse create_notification_audit(notification_audit)

Create notification audit record

Persists a notification delivery attempt audit record.  **Business Requirement**: BR-STORAGE-001 (Notification audit persistence)  **Behavior**: - Success: Returns 201 Created with created record - Validation Error: Returns 400 Bad Request (RFC 7807) - Duplicate: Returns 409 Conflict (RFC 7807) - Database Error: Returns 202 Accepted (DLQ fallback, DD-009)  **Metrics Emitted** (GAP-10): - `datastorage_audit_traces_total{service=\"notification\", status=\"success|failure|dlq_fallback\"}` - `datastorage_audit_lag_seconds{service=\"notification\"}` - `datastorage_write_duration_seconds{table=\"notification_audit\"}` - `datastorage_validation_failures_total{field=\"...\", reason=\"...\"}` 

### Example

```python
import datastorage
from datastorage.apis.tags import audit_write_api_api
from datastorage.model.notification_audit import NotificationAudit
from datastorage.model.notification_audit_response import NotificationAuditResponse
from datastorage.model.rfc7807_problem import RFC7807Problem
from pprint import pprint
# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = audit_write_api_api.AuditWriteAPIApi(api_client)

    # example passing only required values which don't have defaults set
    body = NotificationAudit(
        remediation_id="remediation-123",
        notification_id="notification-456",
        recipient="ops-team@example.com",
        channel="email",
        message_summary="Incident alert: High CPU usage on pod xyz",
        status="sent",
        sent_at="2025-11-03T12:00Z",
        delivery_status="200 OK",
        error_message="Slack API timeout after 30 seconds",
        escalation_level=0,
    )
    try:
        # Create notification audit record
        api_response = api_instance.create_notification_audit(
            body=body,
        )
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling AuditWriteAPIApi->create_notification_audit: %s\n" % e)
```
### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
body | typing.Union[SchemaForRequestBodyApplicationJson] | required |
content_type | str | optional, default is 'application/json' | Selects the schema and serialization of the request body
accept_content_types | typing.Tuple[str] | default is ('application/json', 'application/problem+json', ) | Tells the server the content type(s) that are accepted by the client
stream | bool | default is False | if True then the response.content will be streamed and loaded from a file like object. When downloading a file, set this to True to force the code to deserialize the content to a FileSchema file
timeout | typing.Optional[typing.Union[int, typing.Tuple]] | default is None | the timeout used by the rest client
skip_deserialization | bool | default is False | when True, headers and body will be unset and an instance of api_client.ApiResponseWithoutDeserialization will be returned

### body

# SchemaForRequestBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**NotificationAudit**](../../models/NotificationAudit.md) |  | 


### Return Types, Responses

Code | Class | Description
------------- | ------------- | -------------
n/a | api_client.ApiResponseWithoutDeserialization | When skip_deserialization is True this response is returned
201 | [ApiResponseFor201](#create_notification_audit.ApiResponseFor201) | Audit record created successfully in PostgreSQL. Record is immediately available for queries. 
202 | [ApiResponseFor202](#create_notification_audit.ApiResponseFor202) | Database write failed, audit record queued to Dead Letter Queue (DD-009). Record will be processed asynchronously.  **Reason**: PostgreSQL temporarily unavailable or under load. **Recovery**: DLQ consumer will retry write every 30 seconds. 
400 | [ApiResponseFor400](#create_notification_audit.ApiResponseFor400) | Validation error - request body failed validation. Returns RFC 7807 Problem Details format (BR-STORAGE-024). 
409 | [ApiResponseFor409](#create_notification_audit.ApiResponseFor409) | Conflict - duplicate notification_id. Notification audit records use notification_id as unique constraint. 
500 | [ApiResponseFor500](#create_notification_audit.ApiResponseFor500) | Internal server error - both database and DLQ unavailable. This indicates a critical system failure requiring immediate attention. 

#### create_notification_audit.ApiResponseFor201
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor201ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor201ResponseBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**NotificationAuditResponse**](../../models/NotificationAuditResponse.md) |  | 


#### create_notification_audit.ApiResponseFor202
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor202ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor202ResponseBodyApplicationJson

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  |  | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**message** | str,  | str,  |  | 
**status** | str,  | str,  |  | must be one of ["accepted", ] 
**any_string_name** | dict, frozendict.frozendict, str, date, datetime, int, float, bool, decimal.Decimal, None, list, tuple, bytes, io.FileIO, io.BufferedReader | frozendict.frozendict, str, BoolClass, decimal.Decimal, NoneClass, tuple, bytes, FileIO | any string name can be used but the value must be the correct type | [optional]

#### create_notification_audit.ApiResponseFor400
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor400ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor400ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


#### create_notification_audit.ApiResponseFor409
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor409ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor409ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


#### create_notification_audit.ApiResponseFor500
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor500ResponseBodyApplicationProblemjson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor500ResponseBodyApplicationProblemjson
Type | Description  | Notes
------------- | ------------- | -------------
[**RFC7807Problem**](../../models/RFC7807Problem.md) |  | 


### Authorization

No authorization required

[[Back to top]](#__pageTop) [[Back to API list]](../../../README.md#documentation-for-api-endpoints) [[Back to Model list]](../../../README.md#documentation-for-models) [[Back to README]](../../../README.md)

# **query_audit_events**
<a id="query_audit_events"></a>
> AuditEventsQueryResponse query_audit_events()

Query audit events

Query audit events with filters and pagination

### Example

```python
import datastorage
from datastorage.apis.tags import audit_write_api_api
from datastorage.model.audit_events_query_response import AuditEventsQueryResponse
from pprint import pprint
# Defining the host is optional and defaults to http://localhost:8080
# See configuration.py for a list of all supported configuration parameters.
configuration = datastorage.Configuration(
    host = "http://localhost:8080"
)

# Enter a context with an instance of the API client
with datastorage.ApiClient(configuration) as api_client:
    # Create an instance of the API class
    api_instance = audit_write_api_api.AuditWriteAPIApi(api_client)

    # example passing only optional values
    query_params = {
        'event_type': "notification.message.sent",
        'event_category': "notification",
        'event_outcome': "success",
        'severity': "critical",
        'correlation_id': "rr-2025-001",
        'since': "24h",
        'until': "2025-12-18T23:59:59Z",
        'limit': 50,
        'offset': 0,
    }
    try:
        # Query audit events
        api_response = api_instance.query_audit_events(
            query_params=query_params,
        )
        pprint(api_response)
    except datastorage.ApiException as e:
        print("Exception when calling AuditWriteAPIApi->query_audit_events: %s\n" % e)
```
### Parameters

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
query_params | RequestQueryParams | |
accept_content_types | typing.Tuple[str] | default is ('application/json', ) | Tells the server the content type(s) that are accepted by the client
stream | bool | default is False | if True then the response.content will be streamed and loaded from a file like object. When downloading a file, set this to True to force the code to deserialize the content to a FileSchema file
timeout | typing.Optional[typing.Union[int, typing.Tuple]] | default is None | the timeout used by the rest client
skip_deserialization | bool | default is False | when True, headers and body will be unset and an instance of api_client.ApiResponseWithoutDeserialization will be returned

### query_params
#### RequestQueryParams

Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
event_type | EventTypeSchema | | optional
event_category | EventCategorySchema | | optional
event_outcome | EventOutcomeSchema | | optional
severity | SeveritySchema | | optional
correlation_id | CorrelationIdSchema | | optional
since | SinceSchema | | optional
until | UntilSchema | | optional
limit | LimitSchema | | optional
offset | OffsetSchema | | optional


# EventTypeSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | 

# EventCategorySchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | 

# EventOutcomeSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | must be one of ["success", "failure", "pending", ] 

# SeveritySchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | 

# CorrelationIdSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | 

# SinceSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | 

# UntilSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
str,  | str,  |  | 

# LimitSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
decimal.Decimal, int,  | decimal.Decimal,  |  | if omitted the server will use the default value of 50

# OffsetSchema

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
decimal.Decimal, int,  | decimal.Decimal,  |  | if omitted the server will use the default value of 0

### Return Types, Responses

Code | Class | Description
------------- | ------------- | -------------
n/a | api_client.ApiResponseWithoutDeserialization | When skip_deserialization is True this response is returned
200 | [ApiResponseFor200](#query_audit_events.ApiResponseFor200) | Query results

#### query_audit_events.ApiResponseFor200
Name | Type | Description  | Notes
------------- | ------------- | ------------- | -------------
response | urllib3.HTTPResponse | Raw response |
body | typing.Union[SchemaFor200ResponseBodyApplicationJson, ] |  |
headers | Unset | headers were not defined |

# SchemaFor200ResponseBodyApplicationJson
Type | Description  | Notes
------------- | ------------- | -------------
[**AuditEventsQueryResponse**](../../models/AuditEventsQueryResponse.md) |  | 


### Authorization

No authorization required

[[Back to top]](#__pageTop) [[Back to API list]](../../../README.md#documentation-for-api-endpoints) [[Back to Model list]](../../../README.md#documentation-for-models) [[Back to README]](../../../README.md)

