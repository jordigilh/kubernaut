# LLMResponsePayload

LLM API response event payload (llm_response)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**event_id** | **str** | Unique event identifier | 
**incident_id** | **str** | Incident correlation ID (remediation_id) | 
**has_analysis** | **bool** | Whether LLM provided analysis | 
**analysis_length** | **int** | Length of LLM response | 
**analysis_preview** | **str** | First 500 characters of response for audit | 
**tokens_used** | **int** | Tokens consumed by LLM | [optional] 
**tool_call_count** | **int** | Number of tool calls made by LLM | [optional] [default to 0]

## Example

```python
from datastorage.models.llm_response_payload import LLMResponsePayload

# TODO update the JSON string below
json = "{}"
# create an instance of LLMResponsePayload from a JSON string
llm_response_payload_instance = LLMResponsePayload.from_json(json)
# print the JSON string representation of the object
print LLMResponsePayload.to_json()

# convert the object into a dict
llm_response_payload_dict = llm_response_payload_instance.to_dict()
# create an instance of LLMResponsePayload from a dict
llm_response_payload_form_dict = llm_response_payload.from_dict(llm_response_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


