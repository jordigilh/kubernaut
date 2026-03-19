# AIAgentResponseFailedPayload

AI Agent response failure event payload (aiagent.response.failed) - Emitted when an investigation fails (DD-AUDIT-005, SOC2 CC8.1)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator | 
**event_id** | **str** | Unique event identifier | 
**incident_id** | **str** | Incident correlation ID from request | 
**error_message** | **str** | Error message from the failed investigation | 
**phase** | **str** | Phase in which the failure occurred | 
**duration_seconds** | **float** | Duration of the investigation before failure (seconds) | [optional] 

## Example

```python
from datastorage.models.ai_agent_response_failed_payload import AIAgentResponseFailedPayload

# TODO update the JSON string below
json = "{}"
# create an instance of AIAgentResponseFailedPayload from a JSON string
ai_agent_response_failed_payload_instance = AIAgentResponseFailedPayload.from_json(json)
# print the JSON string representation of the object
print AIAgentResponseFailedPayload.to_json()

# convert the object into a dict
ai_agent_response_failed_payload_dict = ai_agent_response_failed_payload_instance.to_dict()
# create an instance of AIAgentResponseFailedPayload from a dict
ai_agent_response_failed_payload_form_dict = ai_agent_response_failed_payload.from_dict(ai_agent_response_failed_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


