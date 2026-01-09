# AIAnalysisErrorPayload

Error event payload (aianalysis.error.occurred)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**phase** | **str** | Phase in which error occurred | 
**error_message** | **str** | Error message | 

## Example

```python
from datastorage.models.ai_analysis_error_payload import AIAnalysisErrorPayload

# TODO update the JSON string below
json = "{}"
# create an instance of AIAnalysisErrorPayload from a JSON string
ai_analysis_error_payload_instance = AIAnalysisErrorPayload.from_json(json)
# print the JSON string representation of the object
print(AIAnalysisErrorPayload.to_json())

# convert the object into a dict
ai_analysis_error_payload_dict = ai_analysis_error_payload_instance.to_dict()
# create an instance of AIAnalysisErrorPayload from a dict
ai_analysis_error_payload_from_dict = AIAnalysisErrorPayload.from_dict(ai_analysis_error_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


