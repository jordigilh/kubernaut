# AuditExportResponseEventsInner

Audit event with hash chain metadata

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_id** | **UUID** |  | [optional] 
**version** | **str** |  | [optional] 
**event_type** | **str** |  | [optional] 
**event_timestamp** | **datetime** |  | [optional] 
**event_category** | **str** |  | [optional] 
**event_action** | **str** |  | [optional] 
**event_outcome** | **str** |  | [optional] 
**correlation_id** | **str** |  | [optional] 
**event_data** | **Dict[str, object]** |  | [optional] 
**event_hash** | **str** | SHA256 hash of this event | [optional] 
**previous_event_hash** | **str** | Hash of previous event in chain | [optional] 
**hash_chain_valid** | **bool** | Whether this event&#39;s hash chain is valid | [optional] 
**legal_hold** | **bool** | Whether this event is under legal hold | [optional] 

## Example

```python
from datastorage.models.audit_export_response_events_inner import AuditExportResponseEventsInner

# TODO update the JSON string below
json = "{}"
# create an instance of AuditExportResponseEventsInner from a JSON string
audit_export_response_events_inner_instance = AuditExportResponseEventsInner.from_json(json)
# print the JSON string representation of the object
print(AuditExportResponseEventsInner.to_json())

# convert the object into a dict
audit_export_response_events_inner_dict = audit_export_response_events_inner_instance.to_dict()
# create an instance of AuditExportResponseEventsInner from a dict
audit_export_response_events_inner_from_dict = AuditExportResponseEventsInner.from_dict(audit_export_response_events_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


