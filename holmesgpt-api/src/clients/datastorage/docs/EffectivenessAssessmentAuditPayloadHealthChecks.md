# EffectivenessAssessmentAuditPayloadHealthChecks

Structured health check results from the K8s API assessment. Only present for effectiveness.health.assessed events. Enables downstream consumers (DS, HAPI) to extract typed fields without parsing the human-readable details string. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**pod_running** | **bool** | Whether at least one pod exists for the target resource | [optional] 
**readiness_pass** | **bool** | Whether all desired replicas are ready (readyReplicas &#x3D;&#x3D; totalReplicas) | [optional] 
**total_replicas** | **int** | Total number of desired replicas | [optional] 
**ready_replicas** | **int** | Number of ready replicas | [optional] 
**restart_delta** | **int** | Total container restart count since remediation | [optional] 
**crash_loops** | **bool** | Whether any container is in CrashLoopBackOff waiting state | [optional] 
**oom_killed** | **bool** | Whether any container was terminated with OOMKilled reason since remediation | [optional] 
**pending_count** | **int** | Number of pods in Pending phase after the stabilization window. Pods still Pending after stabilization indicates scheduling failures, image pull issues, or resource exhaustion. 0 &#x3D; all pods running or terminated.  | [optional] 

## Example

```python
from datastorage.models.effectiveness_assessment_audit_payload_health_checks import EffectivenessAssessmentAuditPayloadHealthChecks

# TODO update the JSON string below
json = "{}"
# create an instance of EffectivenessAssessmentAuditPayloadHealthChecks from a JSON string
effectiveness_assessment_audit_payload_health_checks_instance = EffectivenessAssessmentAuditPayloadHealthChecks.from_json(json)
# print the JSON string representation of the object
print EffectivenessAssessmentAuditPayloadHealthChecks.to_json()

# convert the object into a dict
effectiveness_assessment_audit_payload_health_checks_dict = effectiveness_assessment_audit_payload_health_checks_instance.to_dict()
# create an instance of EffectivenessAssessmentAuditPayloadHealthChecks from a dict
effectiveness_assessment_audit_payload_health_checks_form_dict = effectiveness_assessment_audit_payload_health_checks.from_dict(effectiveness_assessment_audit_payload_health_checks_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


