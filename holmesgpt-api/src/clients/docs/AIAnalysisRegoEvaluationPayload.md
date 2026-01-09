# AIAnalysisRegoEvaluationPayload

Rego policy evaluation event payload (aianalysis.rego.evaluation)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**outcome** | **str** | Evaluation outcome | 
**degraded** | **bool** | Whether evaluation ran in degraded mode | 
**duration_ms** | **int** | Evaluation duration in milliseconds | 
**reason** | **str** | Reason for the evaluation outcome | 

## Example

```python
from datastorage.models.ai_analysis_rego_evaluation_payload import AIAnalysisRegoEvaluationPayload

# TODO update the JSON string below
json = "{}"
# create an instance of AIAnalysisRegoEvaluationPayload from a JSON string
ai_analysis_rego_evaluation_payload_instance = AIAnalysisRegoEvaluationPayload.from_json(json)
# print the JSON string representation of the object
print(AIAnalysisRegoEvaluationPayload.to_json())

# convert the object into a dict
ai_analysis_rego_evaluation_payload_dict = ai_analysis_rego_evaluation_payload_instance.to_dict()
# create an instance of AIAnalysisRegoEvaluationPayload from a dict
ai_analysis_rego_evaluation_payload_from_dict = AIAnalysisRegoEvaluationPayload.from_dict(ai_analysis_rego_evaluation_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


