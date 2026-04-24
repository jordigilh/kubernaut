# RemediationHealthChecks

Post-remediation health check results from EM assessment. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**pod_running** | **bool** | Whether the target pod is running | [optional] 
**readiness_pass** | **bool** | Whether readiness probes are passing | [optional] 
**restart_delta** | **int** | Change in restart count since remediation | [optional] 
**crash_loops** | **bool** | Whether crash loops were detected | [optional] 
**oom_killed** | **bool** | Whether OOM kills were detected | [optional] 
**pending_count** | **int** | Number of pods still in Pending phase after the stabilization window. Non-zero indicates scheduling failures, image pull issues, or resource exhaustion.  | [optional] 

## Example

```python
from datastorage.models.remediation_health_checks import RemediationHealthChecks

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationHealthChecks from a JSON string
remediation_health_checks_instance = RemediationHealthChecks.from_json(json)
# print the JSON string representation of the object
print RemediationHealthChecks.to_json()

# convert the object into a dict
remediation_health_checks_dict = remediation_health_checks_instance.to_dict()
# create an instance of RemediationHealthChecks from a dict
remediation_health_checks_form_dict = remediation_health_checks.from_dict(remediation_health_checks_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


