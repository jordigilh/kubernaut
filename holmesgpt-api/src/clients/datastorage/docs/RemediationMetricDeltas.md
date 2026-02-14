# RemediationMetricDeltas

Metric deltas between pre-remediation and post-remediation measurements. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**cpu_before** | **float** | CPU utilization before remediation (0.0-1.0) | [optional] 
**cpu_after** | **float** | CPU utilization after remediation (0.0-1.0) | [optional] 
**memory_before** | **float** | Memory utilization before remediation (0.0-1.0) | [optional] 
**memory_after** | **float** | Memory utilization after remediation (0.0-1.0) | [optional] 
**latency_p95_before_ms** | **float** | P95 latency before remediation (milliseconds) | [optional] 
**latency_p95_after_ms** | **float** | P95 latency after remediation (milliseconds) | [optional] 
**error_rate_before** | **float** | Error rate before remediation (0.0-1.0) | [optional] 
**error_rate_after** | **float** | Error rate after remediation (0.0-1.0) | [optional] 

## Example

```python
from datastorage.models.remediation_metric_deltas import RemediationMetricDeltas

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationMetricDeltas from a JSON string
remediation_metric_deltas_instance = RemediationMetricDeltas.from_json(json)
# print the JSON string representation of the object
print RemediationMetricDeltas.to_json()

# convert the object into a dict
remediation_metric_deltas_dict = remediation_metric_deltas_instance.to_dict()
# create an instance of RemediationMetricDeltas from a dict
remediation_metric_deltas_form_dict = remediation_metric_deltas.from_dict(remediation_metric_deltas_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


