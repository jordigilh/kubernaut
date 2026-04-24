# ReleaseLegalHold200Response


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**correlation_id** | **str** |  | [optional] 
**events_released** | **int** |  | [optional] 
**released_by** | **str** |  | [optional] 
**released_at** | **datetime** |  | [optional] 

## Example

```python
from datastorage.models.release_legal_hold200_response import ReleaseLegalHold200Response

# TODO update the JSON string below
json = "{}"
# create an instance of ReleaseLegalHold200Response from a JSON string
release_legal_hold200_response_instance = ReleaseLegalHold200Response.from_json(json)
# print the JSON string representation of the object
print ReleaseLegalHold200Response.to_json()

# convert the object into a dict
release_legal_hold200_response_dict = release_legal_hold200_response_instance.to_dict()
# create an instance of ReleaseLegalHold200Response from a dict
release_legal_hold200_response_form_dict = release_legal_hold200_response.from_dict(release_legal_hold200_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


