# RFC7807Problem

RFC 7807 Problem Details for HTTP APIs. Standard error response format (BR-STORAGE-024). See: https://www.rfc-editor.org/rfc/rfc7807.html 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**type** | **str** | URI reference identifying the problem type. Dereferenceable URI when possible.  | 
**title** | **str** | Short, human-readable summary of the problem type. Should not change from occurrence to occurrence.  | 
**status** | **int** | HTTP status code for this occurrence.  | 
**detail** | **str** | Human-readable explanation specific to this occurrence.  | [optional] 
**instance** | **str** | URI reference identifying the specific occurrence.  | [optional] 
**field_errors** | **Dict[str, str]** | Map of field names to error messages for validation errors. Only present for 400 Bad Request responses.  | [optional] 

## Example

```python
from datastorage.models.rfc7807_problem import RFC7807Problem

# TODO update the JSON string below
json = "{}"
# create an instance of RFC7807Problem from a JSON string
rfc7807_problem_instance = RFC7807Problem.from_json(json)
# print the JSON string representation of the object
print RFC7807Problem.to_json()

# convert the object into a dict
rfc7807_problem_dict = rfc7807_problem_instance.to_dict()
# create an instance of RFC7807Problem from a dict
rfc7807_problem_form_dict = rfc7807_problem.from_dict(rfc7807_problem_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


