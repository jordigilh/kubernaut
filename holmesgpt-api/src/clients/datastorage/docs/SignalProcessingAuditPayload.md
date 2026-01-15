# SignalProcessingAuditPayload

Type-safe audit event payload for SignalProcessing (signal.processed, phase.transition, classification.decision, business.classified, enrichment.completed, error.occurred)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**phase** | **str** | Current phase of the SignalProcessing | 
**signal** | **str** | Name of the signal being processed | 
**severity** | **str** | Severity level of the signal | [optional] 
**external_severity** | **str** | Original severity from external monitoring system (e.g., Sev1, P0, critical) | [optional] 
**normalized_severity** | **str** | Normalized severity determined by Rego policy | [optional] 
**determination_source** | **str** | Source of severity determination for audit trail | [optional] 
**policy_hash** | **str** | SHA256 hash of Rego policy used for severity determination (for audit trail and policy version tracking) | [optional] 
**environment** | **str** | Classified environment | [optional] 
**environment_source** | **str** | Source of the environment classification | [optional] 
**priority** | **str** | Assigned priority | [optional] 
**priority_source** | **str** | Source of the priority assignment | [optional] 
**criticality** | **str** | Business criticality classification | [optional] 
**sla_requirement** | **str** | SLA requirement for remediation | [optional] 
**has_owner_chain** | **bool** | Whether the resource has an owner chain | [optional] 
**owner_chain_length** | **int** | Length of the owner chain | [optional] 
**degraded_mode** | **bool** | Whether context enrichment was degraded | [optional] 
**has_pdb** | **bool** | Whether the resource has a PodDisruptionBudget | [optional] 
**has_hpa** | **bool** | Whether the resource has a HorizontalPodAutoscaler | [optional] 
**duration_ms** | **int** | Enrichment duration in milliseconds | [optional] 
**has_namespace** | **bool** | Whether namespace context was enriched | [optional] 
**has_pod** | **bool** | Whether pod context was enriched | [optional] 
**has_deployment** | **bool** | Whether deployment context was enriched | [optional] 
**business_unit** | **str** | Owning business unit | [optional] 
**from_phase** | **str** | Phase being transitioned from | [optional] 
**to_phase** | **str** | Phase being transitioned to | [optional] 
**error** | **str** | Error message if processing failed | [optional] 

## Example

```python
from datastorage.models.signal_processing_audit_payload import SignalProcessingAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of SignalProcessingAuditPayload from a JSON string
signal_processing_audit_payload_instance = SignalProcessingAuditPayload.from_json(json)
# print the JSON string representation of the object
print SignalProcessingAuditPayload.to_json()

# convert the object into a dict
signal_processing_audit_payload_dict = signal_processing_audit_payload_instance.to_dict()
# create an instance of SignalProcessingAuditPayload from a dict
signal_processing_audit_payload_form_dict = signal_processing_audit_payload.from_dict(signal_processing_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


