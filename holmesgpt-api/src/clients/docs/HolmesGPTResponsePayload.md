# HolmesGPTResponsePayload

HolmesGPT API response completion event payload (holmesgpt.response.complete) - Provider perspective (DD-AUDIT-005)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**event_id** | **str** | Unique event identifier | 
**incident_id** | **str** | Incident correlation ID from request | 
**response_data** | [**IncidentResponseData**](IncidentResponseData.md) |  | 

## Example

```python
from datastorage.models.holmes_gpt_response_payload import HolmesGPTResponsePayload

# TODO update the JSON string below
json = "{}"
# create an instance of HolmesGPTResponsePayload from a JSON string
holmes_gpt_response_payload_instance = HolmesGPTResponsePayload.from_json(json)
# print the JSON string representation of the object
print(HolmesGPTResponsePayload.to_json())

# convert the object into a dict
holmes_gpt_response_payload_dict = holmes_gpt_response_payload_instance.to_dict()
# create an instance of HolmesGPTResponsePayload from a dict
holmes_gpt_response_payload_from_dict = HolmesGPTResponsePayload.from_dict(holmes_gpt_response_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


