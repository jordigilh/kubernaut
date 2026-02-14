# EffectivenessAssessmentAuditPayload

Type-safe audit event payload for Effectiveness Monitor controller. Covers component-level events (health, alert, metrics, hash), the scheduling event (assessment.scheduled), and the lifecycle completion event (assessment.completed). Per ADR-EM-001: Each component assessment emits its own audit event. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**correlation_id** | **str** | Correlation ID (EA spec.correlationID, matches parent RR name) | 
**namespace** | **str** | Kubernetes namespace of the EffectivenessAssessment | 
**ea_name** | **str** | Name of the EffectivenessAssessment CRD | [optional] 
**component** | **str** | Assessment component that produced this event | 
**assessed** | **bool** | Whether the component was successfully assessed | [optional] 
**score** | **float** | Component score (0.0-1.0), null if not assessed | [optional] 
**details** | **str** | Human-readable details about the assessment result | [optional] 
**reason** | **str** | Assessment completion reason (only for assessment.completed events) | [optional] 
**validity_deadline** | **datetime** | Computed validity deadline (only for assessment.scheduled events). EA.creationTimestamp + validityWindow from EM config.  | [optional] 
**prometheus_check_after** | **datetime** | Computed earliest time for Prometheus check (only for assessment.scheduled events). EA.creationTimestamp + stabilizationWindow.  | [optional] 
**alertmanager_check_after** | **datetime** | Computed earliest time for AlertManager check (only for assessment.scheduled events). EA.creationTimestamp + stabilizationWindow.  | [optional] 
**validity_window** | **str** | Validity window duration from EM config (only for assessment.scheduled events). Included for operational observability.  | [optional] 
**stabilization_window** | **str** | Stabilization window duration from EA spec (only for assessment.scheduled events). Included for operational observability.  | [optional] 
**pre_remediation_spec_hash** | **str** | Canonical SHA-256 hash of the target resource&#39;s .spec BEFORE remediation. Retrieved from DataStorage audit trail (remediation.workflow_created event). Format: \&quot;sha256:&lt;hex&gt;\&quot;. Only present for effectiveness.hash.computed events.  | [optional] 
**post_remediation_spec_hash** | **str** | Canonical SHA-256 hash of the target resource&#39;s .spec AFTER remediation. Computed by EM during assessment using DD-EM-002 canonical hash algorithm. Format: \&quot;sha256:&lt;hex&gt;\&quot;. Only present for effectiveness.hash.computed events.  | [optional] 
**hash_match** | **bool** | Whether the pre and post remediation spec hashes match. true &#x3D; no change detected (remediation may have been reverted or had no effect). false &#x3D; spec changed (expected for successful remediations). Only present for effectiveness.hash.computed events.  | [optional] 
**health_checks** | [**EffectivenessAssessmentAuditPayloadHealthChecks**](EffectivenessAssessmentAuditPayloadHealthChecks.md) |  | [optional] 
**metric_deltas** | [**EffectivenessAssessmentAuditPayloadMetricDeltas**](EffectivenessAssessmentAuditPayloadMetricDeltas.md) |  | [optional] 
**alert_resolution** | [**EffectivenessAssessmentAuditPayloadAlertResolution**](EffectivenessAssessmentAuditPayloadAlertResolution.md) |  | [optional] 

## Example

```python
from datastorage.models.effectiveness_assessment_audit_payload import EffectivenessAssessmentAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of EffectivenessAssessmentAuditPayload from a JSON string
effectiveness_assessment_audit_payload_instance = EffectivenessAssessmentAuditPayload.from_json(json)
# print the JSON string representation of the object
print EffectivenessAssessmentAuditPayload.to_json()

# convert the object into a dict
effectiveness_assessment_audit_payload_dict = effectiveness_assessment_audit_payload_instance.to_dict()
# create an instance of EffectivenessAssessmentAuditPayload from a dict
effectiveness_assessment_audit_payload_form_dict = effectiveness_assessment_audit_payload.from_dict(effectiveness_assessment_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


