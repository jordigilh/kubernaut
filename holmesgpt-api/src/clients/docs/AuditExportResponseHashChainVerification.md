# AuditExportResponseHashChainVerification


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**total_events_verified** | **int** | Total events with hash chain data | 
**valid_chain_events** | **int** | Events with valid hash chains | 
**broken_chain_events** | **int** | Events with broken hash chains (tampered) | 
**chain_integrity_percentage** | **float** | Percentage of events with valid chains | [optional] 
**verification_timestamp** | **datetime** | When verification was performed | 
**tampered_event_ids** | **List[str]** | String UUIDs of tampered events (if any) | [optional] 

## Example

```python
from datastorage.models.audit_export_response_hash_chain_verification import AuditExportResponseHashChainVerification

# TODO update the JSON string below
json = "{}"
# create an instance of AuditExportResponseHashChainVerification from a JSON string
audit_export_response_hash_chain_verification_instance = AuditExportResponseHashChainVerification.from_json(json)
# print the JSON string representation of the object
print(AuditExportResponseHashChainVerification.to_json())

# convert the object into a dict
audit_export_response_hash_chain_verification_dict = audit_export_response_hash_chain_verification_instance.to_dict()
# create an instance of AuditExportResponseHashChainVerification from a dict
audit_export_response_hash_chain_verification_from_dict = AuditExportResponseHashChainVerification.from_dict(audit_export_response_hash_chain_verification_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


