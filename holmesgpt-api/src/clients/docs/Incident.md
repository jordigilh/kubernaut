# Incident


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**id** | **int** | Unique incident identifier (from resource_action_traces table) | 
**alert_name** | **str** | Name of the Prometheus alert or Kubernetes event | 
**alert_fingerprint** | **str** | Unique alert fingerprint from Prometheus | [optional] 
**alert_severity** | **str** | Severity level of the alert | 
**action_type** | **str** | Type of remediation action taken (e.g., scale, restart, check) | 
**action_timestamp** | **datetime** | Timestamp when the action was executed (ISO 8601) | 
**namespace** | **str** | Kubernetes namespace where the action was taken | [optional] 
**cluster_name** | **str** | Kubernetes cluster identifier | [optional] 
**environment** | **str** | Environment (e.g., production, staging, development) | [optional] 
**target_resource** | **str** | Target Kubernetes resource (e.g., deployment/my-app) | [optional] 
**remediation_request_id** | **str** | Unique remediation request identifier | [optional] 
**model_used** | **str** | AI model used for analysis (e.g., gpt-4, claude-3) | 
**model_confidence** | **float** | Confidence score from the AI model (0.0 to 1.0) | 
**execution_status** | **str** | Current status of the remediation action | 
**start_time** | **datetime** | When the remediation started (ISO 8601) | [optional] 
**end_time** | **datetime** | When the remediation completed (ISO 8601) | [optional] 
**duration** | **int** | Duration of the remediation in milliseconds | [optional] 
**error_message** | **str** | Error message if the remediation failed | [optional] 
**metadata** | **str** | Additional metadata as JSON string | [optional] 

## Example

```python
from datastorage.models.incident import Incident

# TODO update the JSON string below
json = "{}"
# create an instance of Incident from a JSON string
incident_instance = Incident.from_json(json)
# print the JSON string representation of the object
print(Incident.to_json())

# convert the object into a dict
incident_dict = incident_instance.to_dict()
# create an instance of Incident from a dict
incident_from_dict = Incident.from_dict(incident_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


