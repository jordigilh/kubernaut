# AuditExportResponse

Signed audit export with tamper-evident hash chain verification

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**export_metadata** | [**AuditExportResponseExportMetadata**](AuditExportResponseExportMetadata.md) |  | 
**events** | [**List[AuditExportResponseEventsInner]**](AuditExportResponseEventsInner.md) | Audit events matching the query filters | 
**hash_chain_verification** | [**AuditExportResponseHashChainVerification**](AuditExportResponseHashChainVerification.md) |  | 
**detached_signature** | **str** | Detached PEM-encoded signature (if requested) | [optional] 

## Example

```python
from datastorage.models.audit_export_response import AuditExportResponse

# TODO update the JSON string below
json = "{}"
# create an instance of AuditExportResponse from a JSON string
audit_export_response_instance = AuditExportResponse.from_json(json)
# print the JSON string representation of the object
print(AuditExportResponse.to_json())

# convert the object into a dict
audit_export_response_dict = audit_export_response_instance.to_dict()
# create an instance of AuditExportResponse from a dict
audit_export_response_from_dict = AuditExportResponse.from_dict(audit_export_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


