# datastorage.model.workflow_search_response.WorkflowSearchResponse

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  |  | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**[workflows](#workflows)** | list, tuple,  | tuple,  |  | [optional] 
**total_results** | decimal.Decimal, int,  | decimal.Decimal,  | Total number of matching workflows | [optional] 
**filters** | [**WorkflowSearchFilters**](WorkflowSearchFilters.md) | [**WorkflowSearchFilters**](WorkflowSearchFilters.md) |  | [optional] 
**any_string_name** | dict, frozendict.frozendict, str, date, datetime, int, float, bool, decimal.Decimal, None, list, tuple, bytes, io.FileIO, io.BufferedReader | frozendict.frozendict, str, BoolClass, decimal.Decimal, NoneClass, tuple, bytes, FileIO | any string name can be used but the value must be the correct type | [optional]

# workflows

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
list, tuple,  | tuple,  |  | 

### Tuple Items
Class Name | Input Type | Accessed Type | Description | Notes
------------- | ------------- | ------------- | ------------- | -------------
[**WorkflowSearchResult**](WorkflowSearchResult.md) | [**WorkflowSearchResult**](WorkflowSearchResult.md) | [**WorkflowSearchResult**](WorkflowSearchResult.md) |  | 

[[Back to Model list]](../../README.md#documentation-for-models) [[Back to API list]](../../README.md#documentation-for-api-endpoints) [[Back to README]](../../README.md)

