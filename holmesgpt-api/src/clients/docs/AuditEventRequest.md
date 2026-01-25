# AuditEventRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**version** | **str** | Schema version (e.g., \&quot;1.0\&quot;) | 
**event_type** | **str** | Event type identifier (e.g., gateway.signal.received) | 
**event_timestamp** | **datetime** | ISO 8601 timestamp when the event occurred | 
**event_category** | **str** | Service-level event category (ADR-034 v1.4). Per ADR-034 v1.2: event_category MUST match the service name that emits the event. Values: - gateway: Gateway Service - notification: Notification Service - analysis: AI Analysis Service - signalprocessing: Signal Processing Service - workflow: Workflow Catalog Service - execution: Remediation Execution Service - orchestration: Remediation Orchestrator Service - webhook: Authentication Webhook Service (SOC2 CC8.1 operator attribution)  | 
**event_action** | **str** | Action performed (ADR-034) | 
**event_outcome** | **str** | Result of the event | 
**actor_type** | **str** |  | [optional] 
**actor_id** | **str** |  | [optional] 
**resource_type** | **str** |  | [optional] 
**resource_id** | **str** |  | [optional] 
**correlation_id** | **str** | Unique identifier for request correlation | 
**parent_event_id** | **UUID** |  | [optional] 
**namespace** | **str** |  | [optional] 
**cluster_name** | **str** |  | [optional] 
**severity** | **str** |  | [optional] 
**duration_ms** | **int** |  | [optional] 
**event_data** | [**AuditEventRequestEventData**](AuditEventRequestEventData.md) |  | 

## Example

```python
from datastorage.models.audit_event_request import AuditEventRequest

# TODO update the JSON string below
json = "{}"
# create an instance of AuditEventRequest from a JSON string
audit_event_request_instance = AuditEventRequest.from_json(json)
# print the JSON string representation of the object
print(AuditEventRequest.to_json())

# convert the object into a dict
audit_event_request_dict = audit_event_request_instance.to_dict()
# create an instance of AuditEventRequest from a dict
audit_event_request_from_dict = AuditEventRequest.from_dict(audit_event_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


