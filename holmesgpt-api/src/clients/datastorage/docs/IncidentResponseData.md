# IncidentResponseData

Complete IncidentResponse structure from HolmesGPT API (DD-AUDIT-004 - strongly typed, no additionalProperties)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**incident_id** | **str** | Incident identifier from request | 
**analysis** | **str** | Natural language analysis from LLM | 
**root_cause_analysis** | [**IncidentResponseDataRootCauseAnalysis**](IncidentResponseDataRootCauseAnalysis.md) |  | 
**selected_workflow** | [**IncidentResponseDataSelectedWorkflow**](IncidentResponseDataSelectedWorkflow.md) |  | [optional] 
**confidence** | **float** | Overall confidence in analysis | 
**timestamp** | **datetime** | ISO timestamp of analysis completion | 
**needs_human_review** | **bool** | True when AI could not produce reliable result | [optional] [default to False]
**human_review_reason** | **str** | Structured reason when needs_human_review&#x3D;true (BR-HAPI-197, BR-HAPI-200, BR-HAPI-212) | [optional] 
**target_in_owner_chain** | **bool** | Whether RCA target was found in OwnerChain | [optional] [default to True]
**warnings** | **List[str]** | Non-fatal warnings (e.g., OwnerChain validation issues) | [optional] 
**alternative_workflows** | [**List[IncidentResponseDataAlternativeWorkflowsInner]**](IncidentResponseDataAlternativeWorkflowsInner.md) | Other workflows considered but not selected | [optional] 

## Example

```python
from datastorage.models.incident_response_data import IncidentResponseData

# TODO update the JSON string below
json = "{}"
# create an instance of IncidentResponseData from a JSON string
incident_response_data_instance = IncidentResponseData.from_json(json)
# print the JSON string representation of the object
print IncidentResponseData.to_json()

# convert the object into a dict
incident_response_data_dict = incident_response_data_instance.to_dict()
# create an instance of IncidentResponseData from a dict
incident_response_data_form_dict = incident_response_data.from_dict(incident_response_data_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


