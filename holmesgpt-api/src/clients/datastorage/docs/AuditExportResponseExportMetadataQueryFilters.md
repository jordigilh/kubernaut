# AuditExportResponseExportMetadataQueryFilters

Filters applied to this export

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**start_time** | **datetime** |  | [optional] 
**end_time** | **datetime** |  | [optional] 
**correlation_id** | **str** |  | [optional] 
**event_category** | **str** |  | [optional] 
**offset** | **int** |  | [optional] 
**limit** | **int** |  | [optional] 

## Example

```python
from datastorage.models.audit_export_response_export_metadata_query_filters import AuditExportResponseExportMetadataQueryFilters

# TODO update the JSON string below
json = "{}"
# create an instance of AuditExportResponseExportMetadataQueryFilters from a JSON string
audit_export_response_export_metadata_query_filters_instance = AuditExportResponseExportMetadataQueryFilters.from_json(json)
# print the JSON string representation of the object
print AuditExportResponseExportMetadataQueryFilters.to_json()

# convert the object into a dict
audit_export_response_export_metadata_query_filters_dict = audit_export_response_export_metadata_query_filters_instance.to_dict()
# create an instance of AuditExportResponseExportMetadataQueryFilters from a dict
audit_export_response_export_metadata_query_filters_form_dict = audit_export_response_export_metadata_query_filters.from_dict(audit_export_response_export_metadata_query_filters_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


