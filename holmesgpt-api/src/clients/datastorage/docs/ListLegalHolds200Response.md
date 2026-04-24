# ListLegalHolds200Response


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**holds** | [**List[ListLegalHolds200ResponseHoldsInner]**](ListLegalHolds200ResponseHoldsInner.md) |  | [optional] 
**total** | **int** |  | [optional] 

## Example

```python
from datastorage.models.list_legal_holds200_response import ListLegalHolds200Response

# TODO update the JSON string below
json = "{}"
# create an instance of ListLegalHolds200Response from a JSON string
list_legal_holds200_response_instance = ListLegalHolds200Response.from_json(json)
# print the JSON string representation of the object
print ListLegalHolds200Response.to_json()

# convert the object into a dict
list_legal_holds200_response_dict = list_legal_holds200_response_instance.to_dict()
# create an instance of ListLegalHolds200Response from a dict
list_legal_holds200_response_form_dict = list_legal_holds200_response.from_dict(list_legal_holds200_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


