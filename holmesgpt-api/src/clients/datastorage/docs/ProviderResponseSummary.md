# ProviderResponseSummary

Summary of Holmes API response from AIAnalysis consumer perspective (BR-AUDIT-005 v2.0 Gap

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**incident_id** | **str** | Incident identifier | 
**analysis_preview** | **str** | First 500 chars of analysis | 
**selected_workflow_id** | **str** | Selected workflow identifier (if any) | [optional] 
**needs_human_review** | **bool** | Whether human review is required | 
**warnings_count** | **int** | Number of warnings from Holmes | 

## Example

```python
from datastorage.models.provider_response_summary import ProviderResponseSummary

# TODO update the JSON string below
json = "{}"
# create an instance of ProviderResponseSummary from a JSON string
provider_response_summary_instance = ProviderResponseSummary.from_json(json)
# print the JSON string representation of the object
print ProviderResponseSummary.to_json()

# convert the object into a dict
provider_response_summary_dict = provider_response_summary_instance.to_dict()
# create an instance of ProviderResponseSummary from a dict
provider_response_summary_form_dict = provider_response_summary.from_dict(provider_response_summary_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


