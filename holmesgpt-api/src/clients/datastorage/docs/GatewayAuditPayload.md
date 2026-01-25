# GatewayAuditPayload

Type-safe audit event payload for Gateway (signal.received, signal.deduplicated, crd.created, crd.failed)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**original_payload** | **Dict[str, object]** | Full signal payload for RR.Spec.OriginalPayload reconstruction | [optional] 
**signal_labels** | **Dict[str, str]** | Signal labels for RR.Spec.SignalLabels reconstruction | [optional] 
**signal_annotations** | **Dict[str, str]** | Signal annotations for RR.Spec.SignalAnnotations reconstruction | [optional] 
**signal_type** | **str** | Signal type identifier for classification and metrics (prometheus-alert&#x3D;Prometheus AlertManager, kubernetes-event&#x3D;Kubernetes events) | 
**alert_name** | **str** | Name of the alert | 
**namespace** | **str** | Kubernetes namespace of the affected resource | 
**fingerprint** | **str** | Unique identifier for the signal (deduplication) | 
**severity** | **str** | Raw severity from signal source (pass-through per DD-SEVERITY-001). Gateway does NOT normalize. Accepts ANY value (e.g., \&quot;warning\&quot;, \&quot;Sev1\&quot;, \&quot;P0\&quot;, \&quot;critical\&quot;, etc.). SignalProcessing performs normalization via Rego. | [optional] 
**resource_kind** | **str** | Kubernetes resource kind | [optional] 
**resource_name** | **str** | Name of the affected Kubernetes resource | [optional] 
**remediation_request** | **str** | Created RemediationRequest reference (namespace/name) | [optional] 
**deduplication_status** | **str** | Whether this is a new or duplicate signal | [optional] 
**occurrence_count** | **int** | Number of times this signal has been seen | [optional] 
**error_details** | [**ErrorDetails**](ErrorDetails.md) |  | [optional] 

## Example

```python
from datastorage.models.gateway_audit_payload import GatewayAuditPayload

# TODO update the JSON string below
json = "{}"
# create an instance of GatewayAuditPayload from a JSON string
gateway_audit_payload_instance = GatewayAuditPayload.from_json(json)
# print the JSON string representation of the object
print GatewayAuditPayload.to_json()

# convert the object into a dict
gateway_audit_payload_dict = gateway_audit_payload_instance.to_dict()
# create an instance of GatewayAuditPayload from a dict
gateway_audit_payload_form_dict = gateway_audit_payload.from_dict(gateway_audit_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


