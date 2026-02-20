# RemediationWorkflow


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_id** | **str** | Unique workflow identifier (UUID, auto-generated) | [optional] 
**workflow_name** | **str** | Workflow name (identifier for versions) | 
**action_type** | **str** | Action type from taxonomy (DD-WORKFLOW-016). FK to action_type_taxonomy. | 
**version** | **str** | Semantic version (e.g., v1.0.0) | 
**name** | **str** | Human-readable workflow title | 
**description** | [**StructuredDescription**](StructuredDescription.md) |  | 
**owner** | **str** | Workflow owner | [optional] 
**maintainer** | **str** | Workflow maintainer email | [optional] 
**content** | **str** | YAML workflow definition | 
**content_hash** | **str** | SHA-256 hash of content | 
**parameters** | **Dict[str, object]** | Workflow parameters (JSONB) | [optional] 
**execution_engine** | **str** | Execution engine (e.g., argo-workflows) | 
**schema_image** | **str** | OCI image used to extract the workflow schema (DD-WORKFLOW-017) | [optional] 
**schema_digest** | **str** | OCI schema image digest | [optional] 
**execution_bundle** | **str** | OCI execution bundle reference (digest-pinned) | [optional] 
**execution_bundle_digest** | **str** | OCI execution bundle digest | [optional] 
**labels** | [**MandatoryLabels**](MandatoryLabels.md) |  | 
**custom_labels** | **Dict[str, List[str]]** | Customer-defined labels (DD-WORKFLOW-001 v1.5) - subdomain-based format | [optional] 
**detected_labels** | [**DetectedLabels**](DetectedLabels.md) |  | [optional] 
**status** | **str** | Workflow lifecycle status | 
**disabled_at** | **datetime** | When workflow was disabled | [optional] 
**disabled_by** | **str** | Who disabled the workflow | [optional] 
**disabled_reason** | **str** | Why workflow was disabled | [optional] 
**is_latest_version** | **bool** | Is this the latest version? | [optional] 
**previous_version** | **str** | Previous version identifier | [optional] 
**deprecation_notice** | **str** | Deprecation notice | [optional] 
**version_notes** | **str** | Version release notes | [optional] 
**change_summary** | **str** | Summary of changes in this version | [optional] 
**approved_by** | **str** | Who approved this version | [optional] 
**approved_at** | **datetime** | When this version was approved | [optional] 
**expected_success_rate** | **float** | Expected success rate (0.0-1.0) | [optional] 
**expected_duration_seconds** | **int** | Expected execution duration | [optional] 
**actual_success_rate** | **float** | Actual success rate (0.0-1.0) | [optional] 
**total_executions** | **int** | Total number of executions | [optional] 
**successful_executions** | **int** | Number of successful executions | [optional] 
**created_at** | **datetime** |  | [optional] 
**updated_at** | **datetime** |  | [optional] 
**created_by** | **str** |  | [optional] 
**updated_by** | **str** |  | [optional] 

## Example

```python
from datastorage.models.remediation_workflow import RemediationWorkflow

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationWorkflow from a JSON string
remediation_workflow_instance = RemediationWorkflow.from_json(json)
# print the JSON string representation of the object
print RemediationWorkflow.to_json()

# convert the object into a dict
remediation_workflow_dict = remediation_workflow_instance.to_dict()
# create an instance of RemediationWorkflow from a dict
remediation_workflow_form_dict = remediation_workflow.from_dict(remediation_workflow_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


