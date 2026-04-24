# AIAnalysisApprovalDecisionPayload

Approval decision event payload (aianalysis.approval.decision)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**approval_required** | **bool** | Whether manual approval is required | 
**approval_reason** | **str** | Reason for approval requirement | 
**auto_approved** | **bool** | Whether auto-approved | 
**decision** | **str** | Decision made | 
**reason** | **str** | Reason for decision | 
**environment** | **str** | Environment context | 
**confidence** | **float** | Workflow confidence level (optional) | [optional] 
**workflow_id** | **str** | Selected workflow identifier (optional) | [optional] 

## Example

```python
from datastorage.models.ai_analysis_approval_decision_payload import AIAnalysisApprovalDecisionPayload

# TODO update the JSON string below
json = "{}"
# create an instance of AIAnalysisApprovalDecisionPayload from a JSON string
ai_analysis_approval_decision_payload_instance = AIAnalysisApprovalDecisionPayload.from_json(json)
# print the JSON string representation of the object
print AIAnalysisApprovalDecisionPayload.to_json()

# convert the object into a dict
ai_analysis_approval_decision_payload_dict = ai_analysis_approval_decision_payload_instance.to_dict()
# create an instance of AIAnalysisApprovalDecisionPayload from a dict
ai_analysis_approval_decision_payload_form_dict = ai_analysis_approval_decision_payload.from_dict(ai_analysis_approval_decision_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


