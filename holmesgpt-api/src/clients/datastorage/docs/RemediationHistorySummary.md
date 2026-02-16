# RemediationHistorySummary

Compact remediation history entry for Tier 2. Contains only essential fields for historical context. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**remediation_uid** | **str** | RemediationRequest UID (correlation key) | 
**signal_type** | **str** | Type of signal | [optional] 
**workflow_type** | **str** | Workflow type applied (null if escalated) | [optional] 
**outcome** | **str** | Remediation outcome | [optional] 
**effectiveness_score** | **float** | EM effectiveness score (0.0-1.0) | [optional] 
**signal_resolved** | **bool** | Whether the originating signal was resolved | [optional] 
**hash_match** | **str** | Result of three-way hash comparison against currentSpecHash | [optional] 
**assessment_reason** | **str** | Reason/status of the effectiveness assessment (same enum as RemediationHistoryEntry). When \&quot;spec_drift\&quot;, effectiveness score is unreliable (DD-EM-002 v1.1).  | [optional] 
**completed_at** | **datetime** | When the remediation was completed | 

## Example

```python
from datastorage.models.remediation_history_summary import RemediationHistorySummary

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationHistorySummary from a JSON string
remediation_history_summary_instance = RemediationHistorySummary.from_json(json)
# print the JSON string representation of the object
print RemediationHistorySummary.to_json()

# convert the object into a dict
remediation_history_summary_dict = remediation_history_summary_instance.to_dict()
# create an instance of RemediationHistorySummary from a dict
remediation_history_summary_form_dict = remediation_history_summary.from_dict(remediation_history_summary_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


