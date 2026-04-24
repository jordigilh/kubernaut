# EffectivenessAssessmentAuditPayloadMetricDeltas

Structured pre/post remediation metric comparison results from Prometheus. Only present for effectiveness.metrics.assessed events. Phase A (V1.0): cpu_before/cpu_after populated. Other fields nullable pending Phase B metrics assessor expansion (additional PromQL queries). 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cpu_before** | **float** | CPU utilization before remediation (earliest sample in query range) | [optional] 
**cpu_after** | **float** | CPU utilization after remediation (latest sample in query range) | [optional] 
**memory_before** | **float** | Memory utilization before remediation (Phase B) | [optional] 
**memory_after** | **float** | Memory utilization after remediation (Phase B) | [optional] 
**latency_p95_before_ms** | **float** | Request latency p95 in milliseconds before remediation (Phase B) | [optional] 
**latency_p95_after_ms** | **float** | Request latency p95 in milliseconds after remediation (Phase B) | [optional] 
**error_rate_before** | **float** | Error rate (5xx/total) before remediation (Phase B) | [optional] 
**error_rate_after** | **float** | Error rate (5xx/total) after remediation (Phase B) | [optional] 
**throughput_before_rps** | **float** | Request throughput (requests/second) before remediation | [optional] 
**throughput_after_rps** | **float** | Request throughput (requests/second) after remediation | [optional] 

## Example

```python
from datastorage.models.effectiveness_assessment_audit_payload_metric_deltas import EffectivenessAssessmentAuditPayloadMetricDeltas

# TODO update the JSON string below
json = "{}"
# create an instance of EffectivenessAssessmentAuditPayloadMetricDeltas from a JSON string
effectiveness_assessment_audit_payload_metric_deltas_instance = EffectivenessAssessmentAuditPayloadMetricDeltas.from_json(json)
# print the JSON string representation of the object
print EffectivenessAssessmentAuditPayloadMetricDeltas.to_json()

# convert the object into a dict
effectiveness_assessment_audit_payload_metric_deltas_dict = effectiveness_assessment_audit_payload_metric_deltas_instance.to_dict()
# create an instance of EffectivenessAssessmentAuditPayloadMetricDeltas from a dict
effectiveness_assessment_audit_payload_metric_deltas_form_dict = effectiveness_assessment_audit_payload_metric_deltas.from_dict(effectiveness_assessment_audit_payload_metric_deltas_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


