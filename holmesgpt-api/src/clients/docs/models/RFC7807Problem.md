# datastorage.model.rfc7807_problem.RFC7807Problem

RFC 7807 Problem Details for HTTP APIs. Standard error response format (BR-STORAGE-024). See: https://www.rfc-editor.org/rfc/rfc7807.html 

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  | RFC 7807 Problem Details for HTTP APIs. Standard error response format (BR-STORAGE-024). See: https://www.rfc-editor.org/rfc/rfc7807.html  | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**title** | str,  | str,  | Short, human-readable summary of the problem type. Should not change from occurrence to occurrence.  | 
**type** | str,  | str,  | URI reference identifying the problem type. Dereferenceable URI when possible.  | 
**status** | decimal.Decimal, int,  | decimal.Decimal,  | HTTP status code for this occurrence.  | value must be a 32 bit integer
**detail** | str,  | str,  | Human-readable explanation specific to this occurrence.  | [optional] 
**instance** | str,  | str,  | URI reference identifying the specific occurrence.  | [optional] 
**[field_errors](#field_errors)** | dict, frozendict.frozendict,  | frozendict.frozendict,  | Map of field names to error messages for validation errors. Only present for 400 Bad Request responses.  | [optional] 
**any_string_name** | dict, frozendict.frozendict, str, date, datetime, int, float, bool, decimal.Decimal, None, list, tuple, bytes, io.FileIO, io.BufferedReader | frozendict.frozendict, str, BoolClass, decimal.Decimal, NoneClass, tuple, bytes, FileIO | any string name can be used but the value must be the correct type | [optional]

# field_errors

Map of field names to error messages for validation errors. Only present for 400 Bad Request responses. 

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  | Map of field names to error messages for validation errors. Only present for 400 Bad Request responses.  | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**any_string_name** | str,  | str,  | any string name can be used but the value must be the correct type | [optional] 

[[Back to Model list]](../../README.md#documentation-for-models) [[Back to API list]](../../README.md#documentation-for-api-endpoints) [[Back to README]](../../README.md)

