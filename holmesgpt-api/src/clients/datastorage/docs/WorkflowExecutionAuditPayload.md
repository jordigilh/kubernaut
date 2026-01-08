# WorkflowExecutionAuditPayload

Type-safe audit event payload for WorkflowExecution (workflow.started, workflow.completed, workflow.failed)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**workflow_id** | **str** | ID of the workflow being executed | 
**workflow_version** | **str** | Version of the workflow being executed | 
**target_resource** | **str** | Kubernetes resource being acted upon (format depends on scope) | 
**phase** | **str** | Current phase of the WorkflowExecution | 
**container_image** | **str** | Tekton PipelineRun container image | 
**execution_name** | **str** | Name of the WorkflowExecution CRD | 
**started_at** | **datetime** | When the PipelineRun started execution | [optional] 
**completed_at** | **datetime** | When the PipelineRun finished (success or failure) | [optional] 
**duration** | **str** | Human-readable execution duration | [optional] 
**failure_reason** | **str** | Categorized failure reason | [optional] 
**failure_message** | **str** | Detailed failure message from Tekton | [optional] 
**failed_task_name** | **str** | Name of the failed TaskRun (if identified) | [optional] 
**error_details** | [**ErrorDetails**](ErrorDetails.md) |  | [optional] 
**pipelinerun_name** | **str** | Name of the associated Tekton PipelineRun | [optional] 

## Example

```python
from datastorage.models.workflow_execution_audit_payload import WorkflowExecutionAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowExecutionAuditPayload from a JSON string
workflow_execution_audit_payload_instance = WorkflowExecutionAuditPayload.from_json(json)
# print the JSON string representation of the object
print WorkflowExecutionAuditPayload.to_json()

# convert the object into a dict
workflow_execution_audit_payload_dict = workflow_execution_audit_payload_instance.to_dict()
# create an instance of WorkflowExecutionAuditPayload from a dict
workflow_execution_audit_payload_form_dict = workflow_execution_audit_payload.from_dict(workflow_execution_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


