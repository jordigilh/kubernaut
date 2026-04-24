# AsyncAcceptanceResponse

Response when audit event is queued for async processing (202 Accepted)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**status** | **str** | Status of the async operation | 
**message** | **str** | Explanation of the async processing | 

## Example

```python
from datastorage.models.async_acceptance_response import AsyncAcceptanceResponse

# TODO update the JSON string below
json = "{}"
# create an instance of AsyncAcceptanceResponse from a JSON string
async_acceptance_response_instance = AsyncAcceptanceResponse.from_json(json)
# print the JSON string representation of the object
print AsyncAcceptanceResponse.to_json()

# convert the object into a dict
async_acceptance_response_dict = async_acceptance_response_instance.to_dict()
# create an instance of AsyncAcceptanceResponse from a dict
async_acceptance_response_form_dict = async_acceptance_response.from_dict(async_acceptance_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


