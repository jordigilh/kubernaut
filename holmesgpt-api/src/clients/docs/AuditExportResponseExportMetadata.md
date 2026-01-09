# AuditExportResponseExportMetadata


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**export_timestamp** | **datetime** | When this export was generated | 
**export_format** | **str** | Format of exported data | 
**query_filters** | [**AuditExportResponseExportMetadataQueryFilters**](AuditExportResponseExportMetadataQueryFilters.md) |  | [optional] 
**total_events** | **int** | Total number of events in this export | 
**signature** | **str** | Digital signature of export (base64-encoded) | 
**signature_algorithm** | **str** | Signature algorithm used | [optional] 
**certificate_fingerprint** | **str** | SHA256 fingerprint of signing certificate | [optional] 
**exported_by** | **str** | User who initiated the export (from X-Auth-Request-User) | [optional] 

## Example

```python
from datastorage.models.audit_export_response_export_metadata import AuditExportResponseExportMetadata

# TODO update the JSON string below
json = "{}"
# create an instance of AuditExportResponseExportMetadata from a JSON string
audit_export_response_export_metadata_instance = AuditExportResponseExportMetadata.from_json(json)
# print the JSON string representation of the object
print(AuditExportResponseExportMetadata.to_json())

# convert the object into a dict
audit_export_response_export_metadata_dict = audit_export_response_export_metadata_instance.to_dict()
# create an instance of AuditExportResponseExportMetadata from a dict
audit_export_response_export_metadata_from_dict = AuditExportResponseExportMetadata.from_dict(audit_export_response_export_metadata_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


