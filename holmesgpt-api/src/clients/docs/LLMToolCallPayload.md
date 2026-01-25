# LLMToolCallPayload

LLM tool call event payload (llm_tool_call)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_id** | **str** | Unique event identifier | 
**incident_id** | **str** | Incident correlation ID (remediation_id) | 
**tool_call_index** | **int** | Sequential index of tool call in conversation | 
**tool_name** | **str** | Name of tool invoked | 
**tool_arguments** | **Dict[str, object]** | Arguments passed to tool (flexible for different tools) | [optional] 
**tool_result** | **object** | Full result returned by tool | 
**tool_result_preview** | **str** | First 500 characters of tool result | [optional] 

## Example

```python
from datastorage.models.llm_tool_call_payload import LLMToolCallPayload

# TODO update the JSON string below
json = "{}"
# create an instance of LLMToolCallPayload from a JSON string
llm_tool_call_payload_instance = LLMToolCallPayload.from_json(json)
# print the JSON string representation of the object
print(LLMToolCallPayload.to_json())

# convert the object into a dict
llm_tool_call_payload_dict = llm_tool_call_payload_instance.to_dict()
# create an instance of LLMToolCallPayload from a dict
llm_tool_call_payload_from_dict = LLMToolCallPayload.from_dict(llm_tool_call_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


