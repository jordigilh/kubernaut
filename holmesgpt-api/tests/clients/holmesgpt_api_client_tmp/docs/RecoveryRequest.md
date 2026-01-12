# RecoveryRequest

Request model for recovery analysis endpoint  Business Requirements: - BR-HAPI-001: Recovery request schema - BR-AUDIT-001: Unified audit trail (remediation_id)  Design Decision: DD-WORKFLOW-002 v2.2 - remediation_id is MANDATORY for audit trail correlation - remediation_id is for CORRELATION ONLY - do NOT use for RCA or workflow matching  Design Decision: DD-RECOVERY-002, DD-RECOVERY-003 - Structured PreviousExecution for recovery attempts - is_recovery_attempt and recovery_attempt_number for tracking - enrichment_results for DetectedLabels workflow filtering

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**incident_id** | **str** | Unique incident identifier | 
**remediation_id** | **str** | Remediation request ID for audit correlation (e.g., &#39;req-2025-11-27-abc123&#39;). MANDATORY per DD-WORKFLOW-002 v2.2. This ID is for CORRELATION/AUDIT ONLY - do NOT use for RCA analysis or workflow matching. | 
**is_recovery_attempt** | **bool** | True if this is a recovery attempt after failed workflow | [optional] [default to False]
**recovery_attempt_number** | **int** |  | [optional] 
**previous_execution** | [**PreviousExecution**](PreviousExecution.md) |  | [optional] 
**enrichment_results** | **Dict[str, object]** |  | [optional] 
**signal_type** | **str** |  | [optional] 
**severity** | **str** |  | [optional] 
**resource_namespace** | **str** |  | [optional] 
**resource_kind** | **str** |  | [optional] 
**resource_name** | **str** |  | [optional] 
**environment** | **str** | Environment classification | [optional] [default to 'unknown']
**priority** | **str** | Priority level | [optional] [default to 'P2']
**risk_tolerance** | **str** | Risk tolerance | [optional] [default to 'medium']
**business_category** | **str** | Business category | [optional] [default to 'standard']
**error_message** | **str** |  | [optional] 
**cluster_name** | **str** |  | [optional] 
**signal_source** | **str** |  | [optional] 

## Example

```python
from holmesgpt_api_client.models.recovery_request import RecoveryRequest

# TODO update the JSON string below
json = "{}"
# create an instance of RecoveryRequest from a JSON string
recovery_request_instance = RecoveryRequest.from_json(json)
# print the JSON string representation of the object
print(RecoveryRequest.to_json())

# convert the object into a dict
recovery_request_dict = recovery_request_instance.to_dict()
# create an instance of RecoveryRequest from a dict
recovery_request_from_dict = RecoveryRequest.from_dict(recovery_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


