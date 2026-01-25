# ReleaseLegalHoldRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**release_reason** | **str** | Reason for releasing legal hold | 

## Example

```python
from datastorage.models.release_legal_hold_request import ReleaseLegalHoldRequest

# TODO update the JSON string below
json = "{}"
# create an instance of ReleaseLegalHoldRequest from a JSON string
release_legal_hold_request_instance = ReleaseLegalHoldRequest.from_json(json)
# print the JSON string representation of the object
print(ReleaseLegalHoldRequest.to_json())

# convert the object into a dict
release_legal_hold_request_dict = release_legal_hold_request_instance.to_dict()
# create an instance of ReleaseLegalHoldRequest from a dict
release_legal_hold_request_from_dict = ReleaseLegalHoldRequest.from_dict(release_legal_hold_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


