# WorkflowSearchAuditPayload

Type-safe audit event payload for workflow search operations (DD-WORKFLOW-014 v2.1)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Discriminator for event data union type | 
**query** | [**QueryMetadata**](QueryMetadata.md) |  | 
**results** | [**ResultsMetadata**](ResultsMetadata.md) |  | 
**search_metadata** | [**SearchExecutionMetadata**](SearchExecutionMetadata.md) |  | 

## Example

```python
from datastorage.models.workflow_search_audit_payload import WorkflowSearchAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowSearchAuditPayload from a JSON string
workflow_search_audit_payload_instance = WorkflowSearchAuditPayload.from_json(json)
# print the JSON string representation of the object
print WorkflowSearchAuditPayload.to_json()

# convert the object into a dict
workflow_search_audit_payload_dict = workflow_search_audit_payload_instance.to_dict()
# create an instance of WorkflowSearchAuditPayload from a dict
workflow_search_audit_payload_form_dict = workflow_search_audit_payload.from_dict(workflow_search_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


