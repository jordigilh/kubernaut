# RemediationApprovalAuditPayload

Type-safe audit event payload for RemediationApprovalRequest webhooks (approval.decided)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**request_name** | **str** | Name of the RemediationApprovalRequest | 
**decision** | **str** | Approval decision | 
**decided_at** | **datetime** | When the decision was made | 
**decision_message** | **str** | Reason for the decision | 
**ai_analysis_ref** | **str** | Name of the referenced AIAnalysis | 

## Example

```python
from datastorage.models.remediation_approval_audit_payload import RemediationApprovalAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationApprovalAuditPayload from a JSON string
remediation_approval_audit_payload_instance = RemediationApprovalAuditPayload.from_json(json)
# print the JSON string representation of the object
print RemediationApprovalAuditPayload.to_json()

# convert the object into a dict
remediation_approval_audit_payload_dict = remediation_approval_audit_payload_instance.to_dict()
# create an instance of RemediationApprovalAuditPayload from a dict
remediation_approval_audit_payload_form_dict = remediation_approval_audit_payload.from_dict(remediation_approval_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


