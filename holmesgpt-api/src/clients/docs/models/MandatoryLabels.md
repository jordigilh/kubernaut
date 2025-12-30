# datastorage.model.mandatory_labels.MandatoryLabels

5 mandatory workflow labels (DD-WORKFLOW-001 v2.3)

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  | 5 mandatory workflow labels (DD-WORKFLOW-001 v2.3) | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**severity** | str,  | str,  | Severity level this workflow is designed for | must be one of ["critical", "high", "medium", "low", ] 
**component** | str,  | str,  | Kubernetes resource type this workflow targets (e.g., pod, deployment, node) | 
**environment** | str,  | str,  | Target environment (production, staging, development, test, * for any) | 
**priority** | str,  | str,  | Business priority level (P0, P1, P2, P3, * for any) | must be one of ["P0", "P1", "P2", "P3", "*", ] 
**signal_type** | str,  | str,  | Signal type this workflow handles (e.g., OOMKilled, CrashLoopBackOff) | 
**any_string_name** | dict, frozendict.frozendict, str, date, datetime, int, float, bool, decimal.Decimal, None, list, tuple, bytes, io.FileIO, io.BufferedReader | frozendict.frozendict, str, BoolClass, decimal.Decimal, NoneClass, tuple, bytes, FileIO | any string name can be used but the value must be the correct type | [optional]

[[Back to Model list]](../../README.md#documentation-for-models) [[Back to API list]](../../README.md#documentation-for-api-endpoints) [[Back to README]](../../README.md)

