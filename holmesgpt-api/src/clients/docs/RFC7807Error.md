# RFC7807Error


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**type** | **str** | URI reference identifying the problem type | 
**title** | **str** | Short, human-readable summary of the problem | 
**status** | **int** | HTTP status code | 
**detail** | **str** | Human-readable explanation specific to this occurrence | [optional] 
**instance** | **str** | URI reference identifying the specific occurrence of the problem | [optional] 

## Example

```python
from datastorage.models.rfc7807_error import RFC7807Error

# TODO update the JSON string below
json = "{}"
# create an instance of RFC7807Error from a JSON string
rfc7807_error_instance = RFC7807Error.from_json(json)
# print the JSON string representation of the object
print(RFC7807Error.to_json())

# convert the object into a dict
rfc7807_error_dict = rfc7807_error_instance.to_dict()
# create an instance of RFC7807Error from a dict
rfc7807_error_from_dict = RFC7807Error.from_dict(rfc7807_error_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


