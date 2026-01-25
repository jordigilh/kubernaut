# PlaceLegalHoldRequest


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**correlation_id** | **str** | Correlation ID of events to place legal hold on | 
**reason** | **str** | Reason for legal hold (e.g., \&quot;litigation\&quot;, \&quot;investigation\&quot;) | 

## Example

```python
from datastorage.models.place_legal_hold_request import PlaceLegalHoldRequest

# TODO update the JSON string below
json = "{}"
# create an instance of PlaceLegalHoldRequest from a JSON string
place_legal_hold_request_instance = PlaceLegalHoldRequest.from_json(json)
# print the JSON string representation of the object
print(PlaceLegalHoldRequest.to_json())

# convert the object into a dict
place_legal_hold_request_dict = place_legal_hold_request_instance.to_dict()
# create an instance of PlaceLegalHoldRequest from a dict
place_legal_hold_request_from_dict = PlaceLegalHoldRequest.from_dict(place_legal_hold_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


