# AIAnalysisHolmesGPTCallPayload

HolmesGPT API call event payload (aianalysis.holmesgpt.call)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**endpoint** | **str** | API endpoint called | 
**http_status_code** | **int** | HTTP status code | 
**duration_ms** | **int** | Call duration in milliseconds | 

## Example

```python
from datastorage.models.ai_analysis_holmes_gpt_call_payload import AIAnalysisHolmesGPTCallPayload

# TODO update the JSON string below
json = "{}"
# create an instance of AIAnalysisHolmesGPTCallPayload from a JSON string
ai_analysis_holmes_gpt_call_payload_instance = AIAnalysisHolmesGPTCallPayload.from_json(json)
# print the JSON string representation of the object
print AIAnalysisHolmesGPTCallPayload.to_json()

# convert the object into a dict
ai_analysis_holmes_gpt_call_payload_dict = ai_analysis_holmes_gpt_call_payload_instance.to_dict()
# create an instance of AIAnalysisHolmesGPTCallPayload from a dict
ai_analysis_holmes_gpt_call_payload_form_dict = ai_analysis_holmes_gpt_call_payload.from_dict(ai_analysis_holmes_gpt_call_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


