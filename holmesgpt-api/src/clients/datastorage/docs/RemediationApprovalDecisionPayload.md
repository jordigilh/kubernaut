# RemediationApprovalDecisionPayload

Audit payload for approval decision event (approval.decision). Captures WHO, WHEN, WHAT, WHY for SOC 2 CC8.1 (User Attribution) compliance. Emitted by RemediationApprovalRequest controller when decision is made. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**remediation_request_name** | **str** | Parent RemediationRequest name (correlation ID) | 
**ai_analysis_name** | **str** | AIAnalysis CRD that required approval | 
**decision** | **str** | Final approval decision | 
**decided_by** | **str** | Authenticated username from webhook (SOC 2 CC8.1) | 
**decided_at** | **datetime** | When decision was made | [optional] 
**decision_message** | **str** | Optional rationale from operator | [optional] 
**confidence** | **float** | AI confidence score that triggered approval | 
**workflow_id** | **str** | Workflow being approved | 
**workflow_version** | **str** | Workflow version | [optional] 
**target_resource** | **str** | Target resource being remediated | [optional] 
**timeout_deadline** | **datetime** | Approval deadline | [optional] 
**decision_duration_seconds** | **int** | Time to decision (seconds) | [optional] 
**approval_reason** | **str** | Reason for requiring approval (for request.created event) | [optional] 
**timeout_reason** | **str** | Reason for timeout (for timeout event) | [optional] 
**timeout_duration_seconds** | **int** | Timeout duration in seconds (for timeout event) | [optional] 

## Example

```python
from datastorage.models.remediation_approval_decision_payload import RemediationApprovalDecisionPayload

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationApprovalDecisionPayload from a JSON string
remediation_approval_decision_payload_instance = RemediationApprovalDecisionPayload.from_json(json)
# print the JSON string representation of the object
print RemediationApprovalDecisionPayload.to_json()

# convert the object into a dict
remediation_approval_decision_payload_dict = remediation_approval_decision_payload_instance.to_dict()
# create an instance of RemediationApprovalDecisionPayload from a dict
remediation_approval_decision_payload_form_dict = remediation_approval_decision_payload.from_dict(remediation_approval_decision_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


