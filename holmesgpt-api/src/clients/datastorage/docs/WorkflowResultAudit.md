# WorkflowResultAudit

Audit information for a single workflow result (BR-AUDIT-027)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_id** | **str** | Workflow UUID (DD-WORKFLOW-002 v3.0) | 
**title** | **str** | Workflow title | 
**rank** | **int** | Search result ranking (1-indexed) | 
**scoring** | [**ScoringV1Audit**](ScoringV1Audit.md) |  | 
**owner** | **str** | Workflow owner | [optional] 
**maintainer** | **str** | Workflow maintainer | [optional] 
**description** | **str** | Workflow description | [optional] 
**labels** | **Dict[str, object]** | Workflow labels | [optional] 

## Example

```python
from datastorage.models.workflow_result_audit import WorkflowResultAudit

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowResultAudit from a JSON string
workflow_result_audit_instance = WorkflowResultAudit.from_json(json)
# print the JSON string representation of the object
print WorkflowResultAudit.to_json()

# convert the object into a dict
workflow_result_audit_dict = workflow_result_audit_instance.to_dict()
# create an instance of WorkflowResultAudit from a dict
workflow_result_audit_form_dict = workflow_result_audit.from_dict(workflow_result_audit_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


