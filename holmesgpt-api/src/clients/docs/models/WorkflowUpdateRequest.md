# datastorage.model.workflow_update_request.WorkflowUpdateRequest

Update mutable workflow fields only (DD-WORKFLOW-012)

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  | Update mutable workflow fields only (DD-WORKFLOW-012) | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**status** | str,  | str,  | Workflow status (mutable) | [optional] must be one of ["active", "disabled", "deprecated", "archived", ] 
**disabled_by** | str,  | str,  | Who disabled the workflow | [optional] 
**disabled_reason** | str,  | str,  | Why the workflow was disabled | [optional] 
**any_string_name** | dict, frozendict.frozendict, str, date, datetime, int, float, bool, decimal.Decimal, None, list, tuple, bytes, io.FileIO, io.BufferedReader | frozendict.frozendict, str, BoolClass, decimal.Decimal, NoneClass, tuple, bytes, FileIO | any string name can be used but the value must be the correct type | [optional]

[[Back to Model list]](../../README.md#documentation-for-models) [[Back to API list]](../../README.md#documentation-for-api-endpoints) [[Back to README]](../../README.md)

