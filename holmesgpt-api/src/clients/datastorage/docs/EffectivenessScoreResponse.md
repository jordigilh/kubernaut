# EffectivenessScoreResponse

On-demand effectiveness score response. DS computes the weighted score from component audit events emitted by the Effectiveness Monitor. Per ADR-EM-001 Principle 5 and DD-017 v2.1 scoring formula. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**correlation_id** | **str** | The correlation ID linking all audit events in the remediation lifecycle. | 
**score** | **float** | Weighted effectiveness score (0.0 to 1.0). Null if no component scores available. Formula: (health * 0.40 + alert * 0.35 + metrics * 0.25) / total_assessed_weight  | [optional] 
**components** | [**EffectivenessComponents**](EffectivenessComponents.md) |  | 
**hash_comparison** | [**HashComparisonData**](HashComparisonData.md) |  | [optional] 
**assessment_status** | **str** | Current assessment status: - no_data: No component events found - in_progress: Some component events present but assessment not completed - full: All components assessed successfully - partial: Some components assessed, others unavailable - spec_drift: Target resource spec changed during assessment (score unreliable, forced to 0.0) - expired: Assessment timed out before completing - no_execution: No workflow execution found for this correlation ID - metrics_timed_out: Prometheus metrics collection timed out - EffectivenessAssessed: Legacy value (equivalent to \&quot;full\&quot;)  | 
**computed_at** | **datetime** | Timestamp when this score was computed. | 

## Example

```python
from datastorage.models.effectiveness_score_response import EffectivenessScoreResponse

# TODO update the JSON string below
json = "{}"
# create an instance of EffectivenessScoreResponse from a JSON string
effectiveness_score_response_instance = EffectivenessScoreResponse.from_json(json)
# print the JSON string representation of the object
print EffectivenessScoreResponse.to_json()

# convert the object into a dict
effectiveness_score_response_dict = effectiveness_score_response_instance.to_dict()
# create an instance of EffectivenessScoreResponse from a dict
effectiveness_score_response_form_dict = effectiveness_score_response.from_dict(effectiveness_score_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


