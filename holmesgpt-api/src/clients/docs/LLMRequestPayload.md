# LLMRequestPayload

LLM API request event payload (llm_request)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**event_id** | **str** | Unique event identifier | 
**incident_id** | **str** | Incident correlation ID (remediation_id) | 
**model** | **str** | LLM model identifier | 
**prompt_length** | **int** | Length of prompt sent to LLM | 
**prompt_preview** | **str** | First 500 characters of prompt for audit | 
**max_tokens** | **int** | Maximum tokens requested | [optional] 
**toolsets_enabled** | **List[str]** | List of enabled toolsets | [optional] 
**mcp_servers** | **List[str]** | List of MCP servers | [optional] 

## Example

```python
from datastorage.models.llm_request_payload import LLMRequestPayload

# TODO update the JSON string below
json = "{}"
# create an instance of LLMRequestPayload from a JSON string
llm_request_payload_instance = LLMRequestPayload.from_json(json)
# print the JSON string representation of the object
print(LLMRequestPayload.to_json())

# convert the object into a dict
llm_request_payload_dict = llm_request_payload_instance.to_dict()
# create an instance of LLMRequestPayload from a dict
llm_request_payload_from_dict = LLMRequestPayload.from_dict(llm_request_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


