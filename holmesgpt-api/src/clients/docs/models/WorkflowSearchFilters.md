# datastorage.model.workflow_search_filters.WorkflowSearchFilters

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  |  | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**severity** | str,  | str,  | Severity level (mandatory: critical, high, medium, low) | must be one of ["critical", "high", "medium", "low", ] 
**component** | str,  | str,  | Component type (mandatory: pod, node, deployment, etc.) | 
**environment** | str,  | str,  | Environment (mandatory: production, staging, development) | 
**priority** | str,  | str,  | Priority level (mandatory: P0, P1, P2, P3) | must be one of ["P0", "P1", "P2", "P3", ] 
**signal_type** | str,  | str,  | Signal type (mandatory: OOMKilled, CrashLoopBackOff, etc.) | 
**custom_labels** | [**CustomLabels**](CustomLabels.md) | [**CustomLabels**](CustomLabels.md) |  | [optional] 
**detected_labels** | [**DetectedLabels**](DetectedLabels.md) | [**DetectedLabels**](DetectedLabels.md) |  | [optional] 
**[status](#status)** | list, tuple,  | tuple,  | Workflow lifecycle status filter | [optional] 
**any_string_name** | dict, frozendict.frozendict, str, date, datetime, int, float, bool, decimal.Decimal, None, list, tuple, bytes, io.FileIO, io.BufferedReader | frozendict.frozendict, str, BoolClass, decimal.Decimal, NoneClass, tuple, bytes, FileIO | any string name can be used but the value must be the correct type | [optional]

# status

Workflow lifecycle status filter

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
list, tuple,  | tuple,  | Workflow lifecycle status filter | 

### Tuple Items
Class Name | Input Type | Accessed Type | Description | Notes
------------- | ------------- | ------------- | ------------- | -------------
items | str,  | str,  |  | must be one of ["active", "disabled", "deprecated", "archived", ] 

[[Back to Model list]](../../README.md#documentation-for-models) [[Back to API list]](../../README.md#documentation-for-api-endpoints) [[Back to README]](../../README.md)

