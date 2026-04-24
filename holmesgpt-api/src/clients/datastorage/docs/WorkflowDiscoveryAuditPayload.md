# WorkflowDiscoveryAuditPayload

Audit event payload for three-step workflow discovery operations. Authority: DD-HAPI-017 (Three-Step Workflow Discovery Integration) Authority: DD-WORKFLOW-014 v3.0 (Workflow Selection Audit Trail) Replaces WorkflowSearchAuditPayload (search endpoint removed). 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Discriminator for event data union type (matches parent event_type) | 
**query** | [**QueryMetadata**](QueryMetadata.md) |  | 
**results** | [**ResultsMetadata**](ResultsMetadata.md) |  | 
**search_metadata** | [**SearchExecutionMetadata**](SearchExecutionMetadata.md) |  | 

## Example

```python
from datastorage.models.workflow_discovery_audit_payload import WorkflowDiscoveryAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowDiscoveryAuditPayload from a JSON string
workflow_discovery_audit_payload_instance = WorkflowDiscoveryAuditPayload.from_json(json)
# print the JSON string representation of the object
print WorkflowDiscoveryAuditPayload.to_json()

# convert the object into a dict
workflow_discovery_audit_payload_dict = workflow_discovery_audit_payload_instance.to_dict()
# create an instance of WorkflowDiscoveryAuditPayload from a dict
workflow_discovery_audit_payload_form_dict = workflow_discovery_audit_payload.from_dict(workflow_discovery_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


