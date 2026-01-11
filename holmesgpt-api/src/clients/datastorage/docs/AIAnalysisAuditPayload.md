# AIAnalysisAuditPayload

Type-safe audit event payload for AIAnalysis (analysis.completed, analysis.failed)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**analysis_name** | **str** | Name of the AIAnalysis CRD | 
**namespace** | **str** | Kubernetes namespace of the AIAnalysis | 
**phase** | **str** | Current phase of the AIAnalysis | 
**approval_required** | **bool** | Whether manual approval is required | 
**approval_reason** | **str** | Reason for approval requirement | [optional] 
**degraded_mode** | **bool** | Whether operating in degraded mode | 
**warnings_count** | **int** | Number of warnings encountered | 
**confidence** | **float** | Workflow selection confidence (0.0-1.0) | [optional] 
**workflow_id** | **str** | Selected workflow identifier | [optional] 
**target_in_owner_chain** | **bool** | Whether target is in owner chain | [optional] 
**reason** | **str** | Primary failure reason | [optional] 
**sub_reason** | **str** | Detailed failure sub-reason | [optional] 
**provider_response_summary** | [**ProviderResponseSummary**](ProviderResponseSummary.md) |  | [optional] 
**error_details** | [**ErrorDetails**](ErrorDetails.md) |  | [optional] 

## Example

```python
from datastorage.models.ai_analysis_audit_payload import AIAnalysisAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of AIAnalysisAuditPayload from a JSON string
ai_analysis_audit_payload_instance = AIAnalysisAuditPayload.from_json(json)
# print the JSON string representation of the object
print AIAnalysisAuditPayload.to_json()

# convert the object into a dict
ai_analysis_audit_payload_dict = ai_analysis_audit_payload_instance.to_dict()
# create an instance of AIAnalysisAuditPayload from a dict
ai_analysis_audit_payload_form_dict = ai_analysis_audit_payload.from_dict(ai_analysis_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


