# AIAnalysisAIAgentCallPayload

AI agent call event payload (aianalysis.aiagent.call)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**endpoint** | **str** | API endpoint called | 
**http_status_code** | **int** | HTTP status code | 
**duration_ms** | **int** | Call duration in milliseconds | 

## Example

```python
from datastorage.models.ai_analysis_ai_agent_call_payload import AIAnalysisAIAgentCallPayload

# TODO update the JSON string below
json = "{}"
# create an instance of AIAnalysisAIAgentCallPayload from a JSON string
ai_analysis_ai_agent_call_payload_instance = AIAnalysisAIAgentCallPayload.from_json(json)
# print the JSON string representation of the object
print AIAnalysisAIAgentCallPayload.to_json()

# convert the object into a dict
ai_analysis_ai_agent_call_payload_dict = ai_analysis_ai_agent_call_payload_instance.to_dict()
# create an instance of AIAnalysisAIAgentCallPayload from a dict
ai_analysis_ai_agent_call_payload_form_dict = ai_analysis_ai_agent_call_payload.from_dict(ai_analysis_ai_agent_call_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


