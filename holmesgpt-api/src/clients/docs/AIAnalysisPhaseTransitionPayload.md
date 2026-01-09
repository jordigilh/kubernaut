# AIAnalysisPhaseTransitionPayload

Phase transition event payload (aianalysis.phase.transition)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**old_phase** | **str** | Previous phase | 
**new_phase** | **str** | New phase | 

## Example

```python
from datastorage.models.ai_analysis_phase_transition_payload import AIAnalysisPhaseTransitionPayload

# TODO update the JSON string below
json = "{}"
# create an instance of AIAnalysisPhaseTransitionPayload from a JSON string
ai_analysis_phase_transition_payload_instance = AIAnalysisPhaseTransitionPayload.from_json(json)
# print the JSON string representation of the object
print(AIAnalysisPhaseTransitionPayload.to_json())

# convert the object into a dict
ai_analysis_phase_transition_payload_dict = ai_analysis_phase_transition_payload_instance.to_dict()
# create an instance of AIAnalysisPhaseTransitionPayload from a dict
ai_analysis_phase_transition_payload_from_dict = AIAnalysisPhaseTransitionPayload.from_dict(ai_analysis_phase_transition_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


