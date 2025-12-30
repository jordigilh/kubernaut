# datastorage.model.workflow_search_result.WorkflowSearchResult

Flat response structure (DD-WORKFLOW-002 v3.0)

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  | Flat response structure (DD-WORKFLOW-002 v3.0) | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**final_score** | decimal.Decimal, int, float,  | decimal.Decimal,  | Final normalized score (same as confidence) | value must be a 32 bit float
**workflow_id** | str, uuid.UUID,  | str,  | UUID primary key (DD-WORKFLOW-002 v3.0) | value must be a uuid
**confidence** | decimal.Decimal, int, float,  | decimal.Decimal,  | Normalized label score (0.0-1.0) | value must be a 32 bit float
**description** | str,  | str,  | Workflow description | 
**rank** | decimal.Decimal, int,  | decimal.Decimal,  | Position in result set (1-based) | 
**title** | str,  | str,  | Human-readable workflow name | 
**signal_type** | str,  | str,  | Signal type this workflow handles | 
**container_image** | str,  | str,  | OCI image reference | [optional] 
**container_digest** | str,  | str,  | OCI image digest | [optional] 
**label_boost** | decimal.Decimal, int, float,  | decimal.Decimal,  | Boost from matching DetectedLabels | [optional] value must be a 32 bit float
**label_penalty** | decimal.Decimal, int, float,  | decimal.Decimal,  | Penalty from conflicting DetectedLabels | [optional] value must be a 32 bit float
**custom_labels** | [**CustomLabels**](CustomLabels.md) | [**CustomLabels**](CustomLabels.md) |  | [optional] 
**detected_labels** | [**DetectedLabels**](DetectedLabels.md) | [**DetectedLabels**](DetectedLabels.md) |  | [optional] 
**any_string_name** | dict, frozendict.frozendict, str, date, datetime, int, float, bool, decimal.Decimal, None, list, tuple, bytes, io.FileIO, io.BufferedReader | frozendict.frozendict, str, BoolClass, decimal.Decimal, NoneClass, tuple, bytes, FileIO | any string name can be used but the value must be the correct type | [optional]

[[Back to Model list]](../../README.md#documentation-for-models) [[Back to API list]](../../README.md#documentation-for-api-endpoints) [[Back to README]](../../README.md)

