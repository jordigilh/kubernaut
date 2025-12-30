# datastorage.model.remediation_workflow.RemediationWorkflow

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  |  | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**execution_engine** | str,  | str,  | Execution engine (e.g., argo-workflows) | 
**workflow_name** | str,  | str,  | Workflow name (identifier for versions) | 
**name** | str,  | str,  | Human-readable workflow title | 
**description** | str,  | str,  | Workflow description | 
**content_hash** | str,  | str,  | SHA-256 hash of content | 
**version** | str,  | str,  | Semantic version (e.g., v1.0.0) | 
**content** | str,  | str,  | YAML workflow definition | 
**labels** | [**MandatoryLabels**](MandatoryLabels.md) | [**MandatoryLabels**](MandatoryLabels.md) |  | 
**status** | str,  | str,  | Workflow lifecycle status | must be one of ["active", "disabled", "deprecated", "archived", ] 
**workflow_id** | str, uuid.UUID,  | str,  | Unique workflow identifier (UUID, auto-generated) | [optional] value must be a uuid
**owner** | str,  | str,  | Workflow owner | [optional] 
**maintainer** | str,  | str,  | Workflow maintainer email | [optional] 
**[parameters](#parameters)** | dict, frozendict.frozendict,  | frozendict.frozendict,  | Workflow parameters (JSONB) | [optional] 
**container_image** | str,  | str,  | OCI image reference | [optional] 
**container_digest** | str,  | str,  | OCI image digest | [optional] 
**custom_labels** | [**CustomLabels**](CustomLabels.md) | [**CustomLabels**](CustomLabels.md) |  | [optional] 
**detected_labels** | [**DetectedLabels**](DetectedLabels.md) | [**DetectedLabels**](DetectedLabels.md) |  | [optional] 
**disabled_at** | str, datetime,  | str,  | When workflow was disabled | [optional] value must conform to RFC-3339 date-time
**disabled_by** | str,  | str,  | Who disabled the workflow | [optional] 
**disabled_reason** | str,  | str,  | Why workflow was disabled | [optional] 
**is_latest_version** | bool,  | BoolClass,  | Is this the latest version? | [optional] 
**previous_version** | str,  | str,  | Previous version identifier | [optional] 
**deprecation_notice** | str,  | str,  | Deprecation notice | [optional] 
**version_notes** | str,  | str,  | Version release notes | [optional] 
**change_summary** | str,  | str,  | Summary of changes in this version | [optional] 
**approved_by** | str,  | str,  | Who approved this version | [optional] 
**approved_at** | str, datetime,  | str,  | When this version was approved | [optional] value must conform to RFC-3339 date-time
**expected_success_rate** | decimal.Decimal, int, float,  | decimal.Decimal,  | Expected success rate (0.0-1.0) | [optional] value must be a 32 bit float
**expected_duration_seconds** | decimal.Decimal, int,  | decimal.Decimal,  | Expected execution duration | [optional] 
**actual_success_rate** | decimal.Decimal, int, float,  | decimal.Decimal,  | Actual success rate (0.0-1.0) | [optional] value must be a 32 bit float
**total_executions** | decimal.Decimal, int,  | decimal.Decimal,  | Total number of executions | [optional] 
**successful_executions** | decimal.Decimal, int,  | decimal.Decimal,  | Number of successful executions | [optional] 
**created_at** | str, datetime,  | str,  |  | [optional] value must conform to RFC-3339 date-time
**updated_at** | str, datetime,  | str,  |  | [optional] value must conform to RFC-3339 date-time
**created_by** | str,  | str,  |  | [optional] 
**updated_by** | str,  | str,  |  | [optional] 
**any_string_name** | dict, frozendict.frozendict, str, date, datetime, int, float, bool, decimal.Decimal, None, list, tuple, bytes, io.FileIO, io.BufferedReader | frozendict.frozendict, str, BoolClass, decimal.Decimal, NoneClass, tuple, bytes, FileIO | any string name can be used but the value must be the correct type | [optional]

# parameters

Workflow parameters (JSONB)

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  | Workflow parameters (JSONB) | 

[[Back to Model list]](../../README.md#documentation-for-models) [[Back to API list]](../../README.md#documentation-for-api-endpoints) [[Back to README]](../../README.md)

