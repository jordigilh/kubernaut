# AIAgentResponsePayload

AI Agent response completion event payload (aiagent.response.complete) - Provider perspective (DD-AUDIT-005)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**event_id** | **str** | Unique event identifier | 
**incident_id** | **str** | Incident correlation ID from request | 
**response_data** | [**IncidentResponseData**](IncidentResponseData.md) |  | 

## Example

```python
from datastorage.models.ai_agent_response_payload import AIAgentResponsePayload

# TODO update the JSON string below
json = "{}"
# create an instance of AIAgentResponsePayload from a JSON string
ai_agent_response_payload_instance = AIAgentResponsePayload.from_json(json)
# print the JSON string representation of the object
print AIAgentResponsePayload.to_json()

# convert the object into a dict
ai_agent_response_payload_dict = ai_agent_response_payload_instance.to_dict()
# create an instance of AIAgentResponsePayload from a dict
ai_agent_response_payload_form_dict = ai_agent_response_payload.from_dict(ai_agent_response_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


