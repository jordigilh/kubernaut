# IncidentResponseDataRootCauseAnalysis

Structured RCA with summary, severity, contributing_factors

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**summary** | **str** | Brief RCA summary | 
**severity** | **str** | Incident severity | 
**contributing_factors** | **List[str]** | List of contributing factors | 

## Example

```python
from datastorage.models.incident_response_data_root_cause_analysis import IncidentResponseDataRootCauseAnalysis

# TODO update the JSON string below
json = "{}"
# create an instance of IncidentResponseDataRootCauseAnalysis from a JSON string
incident_response_data_root_cause_analysis_instance = IncidentResponseDataRootCauseAnalysis.from_json(json)
# print the JSON string representation of the object
print IncidentResponseDataRootCauseAnalysis.to_json()

# convert the object into a dict
incident_response_data_root_cause_analysis_dict = incident_response_data_root_cause_analysis_instance.to_dict()
# create an instance of IncidentResponseDataRootCauseAnalysis from a dict
incident_response_data_root_cause_analysis_form_dict = incident_response_data_root_cause_analysis.from_dict(incident_response_data_root_cause_analysis_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


