# ListLegalHolds200ResponseHoldsInner


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**correlation_id** | **str** |  | [optional] 
**events_affected** | **int** |  | [optional] 
**placed_by** | **str** |  | [optional] 
**placed_at** | **datetime** |  | [optional] 
**reason** | **str** |  | [optional] 

## Example

```python
from datastorage.models.list_legal_holds200_response_holds_inner import ListLegalHolds200ResponseHoldsInner

# TODO update the JSON string below
json = "{}"
# create an instance of ListLegalHolds200ResponseHoldsInner from a JSON string
list_legal_holds200_response_holds_inner_instance = ListLegalHolds200ResponseHoldsInner.from_json(json)
# print the JSON string representation of the object
print ListLegalHolds200ResponseHoldsInner.to_json()

# convert the object into a dict
list_legal_holds200_response_holds_inner_dict = list_legal_holds200_response_holds_inner_instance.to_dict()
# create an instance of ListLegalHolds200ResponseHoldsInner from a dict
list_legal_holds200_response_holds_inner_form_dict = list_legal_holds200_response_holds_inner.from_dict(list_legal_holds200_response_holds_inner_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


