# ErrorDetails

Standardized error information for audit events (BR-AUDIT-005 Gap

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**message** | **str** | Human-readable error description | 
**code** | **str** | Machine-readable error classification (ERR_[CATEGORY]_[SPECIFIC]) | 
**component** | **str** | Service emitting the error | 
**retry_possible** | **bool** | Whether the operation can be retried (true&#x3D;transient, false&#x3D;permanent) | 
**stack_trace** | **List[str]** | Top N stack frames for debugging (optional, 5-10 frames max) | [optional] 

## Example

```python
from datastorage.models.error_details import ErrorDetails

# TODO update the JSON string below
json = "{}"
# create an instance of ErrorDetails from a JSON string
error_details_instance = ErrorDetails.from_json(json)
# print the JSON string representation of the object
print ErrorDetails.to_json()

# convert the object into a dict
error_details_dict = error_details_instance.to_dict()
# create an instance of ErrorDetails from a dict
error_details_form_dict = error_details.from_dict(error_details_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


