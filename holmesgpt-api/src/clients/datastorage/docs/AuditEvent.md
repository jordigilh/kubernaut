# AuditEvent


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**version** | **str** | Schema version (e.g., \&quot;1.0\&quot;) | 
**event_type** | **str** | Event type identifier (e.g., gateway.signal.received) | 
**event_timestamp** | **datetime** | ISO 8601 timestamp when the event occurred | 
**event_category** | **str** | Service-level event category (ADR-034 v1.7). Per ADR-034 v1.2: event_category MUST match the service name that emits the event. Values: - gateway: Gateway Service - notification: Notification Service - analysis: AI Analysis Controller (NOT HolmesGPT API) - aiagent: AI Agent Provider (HolmesGPT API) - autonomous tool-calling agent - signalprocessing: Signal Processing Service - workflow: Workflow Catalog Service - workflowexecution: WorkflowExecution Controller (ADR-034 v1.5) - approval: RemediationApprovalRequest Controller (BR-AUDIT-006) - orchestration: Remediation Orchestrator Service - webhook: Authentication Webhook Service (SOC2 CC8.1 operator attribution)  | 
**event_action** | **str** | Action performed (ADR-034) | 
**event_outcome** | **str** | Result of the event | 
**actor_type** | **str** |  | [optional] 
**actor_id** | **str** |  | [optional] 
**resource_type** | **str** |  | [optional] 
**resource_id** | **str** |  | [optional] 
**correlation_id** | **str** | Unique identifier for request correlation | 
**parent_event_id** | **str** |  | [optional] 
**namespace** | **str** |  | [optional] 
**cluster_name** | **str** |  | [optional] 
**severity** | **str** |  | [optional] 
**duration_ms** | **int** |  | [optional] 
**event_data** | [**AuditEventRequestEventData**](AuditEventRequestEventData.md) |  | 
**event_id** | **str** |  | [optional] 
**event_date** | **date** | Date of the event (YYYY-MM-DD). Nullable to handle format mismatches from DataStorage. | [optional] 

## Example

```python
from datastorage.models.audit_event import AuditEvent

# TODO update the JSON string below
json = "{}"
# create an instance of AuditEvent from a JSON string
audit_event_instance = AuditEvent.from_json(json)
# print the JSON string representation of the object
print AuditEvent.to_json()

# convert the object into a dict
audit_event_dict = audit_event_instance.to_dict()
# create an instance of AuditEvent from a dict
audit_event_form_dict = audit_event.from_dict(audit_event_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


