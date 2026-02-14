# RemediationHistoryEntry

Full remediation history entry for Tier 1. Correlates RO (remediation.workflow_created) and EM (effectiveness.assessment.completed) audit events by remediation_request_uid. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**remediation_uid** | **str** | RemediationRequest UID (correlation key) | 
**signal_fingerprint** | **str** | Signal fingerprint that triggered the remediation | [optional] 
**signal_type** | **str** | Type of signal (e.g. HighCPULoad, OOMKilled) | [optional] 
**workflow_type** | **str** | Workflow type applied (null if escalated to human review) | [optional] 
**outcome** | **str** | Remediation outcome (Success, Failed, Escalated) | [optional] 
**effectiveness_score** | **float** | EM effectiveness score (0.0-1.0). Null if assessment not yet completed or remediation was escalated.  | [optional] 
**signal_resolved** | **bool** | Whether the originating signal was resolved after remediation | [optional] 
**hash_match** | **str** | Result of three-way hash comparison: - preRemediation: currentSpecHash matches preRemediationSpecHash (regression) - postRemediation: currentSpecHash matches postRemediationSpecHash (unchanged) - none: currentSpecHash matches neither hash  | [optional] 
**pre_remediation_spec_hash** | **str** | Spec hash captured before remediation was applied | [optional] 
**post_remediation_spec_hash** | **str** | Spec hash captured after remediation was applied | [optional] 
**health_checks** | [**RemediationHealthChecks**](RemediationHealthChecks.md) |  | [optional] 
**metric_deltas** | [**RemediationMetricDeltas**](RemediationMetricDeltas.md) |  | [optional] 
**side_effects** | **List[str]** | List of detected side effects from the remediation | [optional] 
**completed_at** | **datetime** | When the remediation was completed | 
**assessment_reason** | **str** | Reason/status of the effectiveness assessment. Null if assessment not yet completed. When \&quot;spec_drift\&quot;, the effectiveness score is unreliable (hard-overridden to 0.0) because the target resource spec was modified during the assessment window (DD-EM-002 v1.1).  | [optional] 
**assessed_at** | **datetime** | When the effectiveness assessment was completed | [optional] 

## Example

```python
from datastorage.models.remediation_history_entry import RemediationHistoryEntry

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationHistoryEntry from a JSON string
remediation_history_entry_instance = RemediationHistoryEntry.from_json(json)
# print the JSON string representation of the object
print RemediationHistoryEntry.to_json()

# convert the object into a dict
remediation_history_entry_dict = remediation_history_entry_instance.to_dict()
# create an instance of RemediationHistoryEntry from a dict
remediation_history_entry_form_dict = remediation_history_entry.from_dict(remediation_history_entry_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


