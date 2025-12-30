# datastorage.model.audit_event_request.AuditEventRequest

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  |  | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**event_action** | str,  | str,  | Action performed (ADR-034) | 
**event_type** | str,  | str,  | Event type identifier (e.g., gateway.signal.received) | 
**event_outcome** | str,  | str,  | Result of the event | must be one of ["success", "failure", "pending", ] 
**correlation_id** | str,  | str,  | Unique identifier for request correlation | 
**event_data** | dict, frozendict.frozendict, str, date, datetime, uuid.UUID, int, float, decimal.Decimal, bool, None, list, tuple, bytes, io.FileIO, io.BufferedReader,  | frozendict.frozendict, str, decimal.Decimal, BoolClass, NoneClass, tuple, bytes, FileIO | Service-specific event data as structured Go type. Accepts any JSON-marshalable type (structs, maps, etc.). V1.0: Eliminates map[string]interface{} - use structured types directly. See DD-AUDIT-004 for structured type requirements.  | 
**event_category** | str,  | str,  | Service-level event category (ADR-034 v1.2). Values: - gateway: Gateway Service - notification: Notification Service - analysis: AI Analysis Service - signalprocessing: Signal Processing Service - workflow: Workflow Catalog Service - execution: Remediation Execution Service - orchestration: Remediation Orchestrator Service  | must be one of ["gateway", "notification", "analysis", "signalprocessing", "workflow", "execution", "orchestration", ] 
**event_timestamp** | str, datetime,  | str,  | ISO 8601 timestamp when the event occurred | value must conform to RFC-3339 date-time
**version** | str,  | str,  | Schema version (e.g., \&quot;1.0\&quot;) | 
**actor_type** | str,  | str,  |  | [optional] 
**actor_id** | str,  | str,  |  | [optional] 
**resource_type** | str,  | str,  |  | [optional] 
**resource_id** | str,  | str,  |  | [optional] 
**parent_event_id** | None, str, uuid.UUID,  | NoneClass, str,  |  | [optional] value must be a uuid
**namespace** | None, str,  | NoneClass, str,  |  | [optional] 
**cluster_name** | None, str,  | NoneClass, str,  |  | [optional] 
**severity** | None, str,  | NoneClass, str,  |  | [optional] 
**duration_ms** | None, decimal.Decimal, int,  | NoneClass, decimal.Decimal,  |  | [optional] 
**any_string_name** | dict, frozendict.frozendict, str, date, datetime, int, float, bool, decimal.Decimal, None, list, tuple, bytes, io.FileIO, io.BufferedReader | frozendict.frozendict, str, BoolClass, decimal.Decimal, NoneClass, tuple, bytes, FileIO | any string name can be used but the value must be the correct type | [optional]

[[Back to Model list]](../../README.md#documentation-for-models) [[Back to API list]](../../README.md#documentation-for-api-endpoints) [[Back to README]](../../README.md)

