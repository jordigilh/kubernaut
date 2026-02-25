# IncidentRequest

Request model for initial incident analysis endpoint  Business Requirements: - BR-HAPI-002: Incident analysis request schema - BR-AUDIT-001: Unified audit trail (remediation_id)  Design Decision: DD-WORKFLOW-002 v2.2 - remediation_id is MANDATORY for audit trail correlation - remediation_id is for CORRELATION ONLY - do NOT use for RCA or workflow matching  Design Decision: DD-HAPI-001 - enrichment_results contains DetectedLabels for workflow filtering

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**incident_id** | **str** | Unique incident identifier | 
**remediation_id** | **str** | Remediation request ID for audit correlation (e.g., &#39;req-2025-11-27-abc123&#39;). MANDATORY per DD-WORKFLOW-002 v2.2. This ID is for CORRELATION/AUDIT ONLY - do NOT use for RCA analysis or workflow matching. | 
**signal_type** | **str** | Canonical signal type | 
**severity** | **str** | Signal severity | 
**signal_source** | **str** | Monitoring system | 
**resource_namespace** | **str** | Kubernetes namespace | 
**resource_kind** | **str** | Kubernetes resource kind | 
**resource_name** | **str** | Resource name | 
**error_message** | **str** | Error message | 
**description** | **str** |  | [optional] 
**environment** | **str** | Deployment environment | 
**priority** | **str** | Business priority | 
**risk_tolerance** | **str** | Risk tolerance | 
**business_category** | **str** | Business category | 
**cluster_name** | **str** | Kubernetes cluster name | 
**is_duplicate** | **bool** |  | [optional] 
**occurrence_count** | **int** |  | [optional] 
**deduplication_window_minutes** | **int** |  | [optional] 
**is_storm** | **bool** |  | [optional] 
**storm_signal_count** | **int** |  | [optional] 
**storm_window_minutes** | **int** |  | [optional] 
**storm_type** | **str** |  | [optional] 
**affected_resources** | **List[str]** |  | [optional] 
**firing_time** | **str** |  | [optional] 
**received_time** | **str** |  | [optional] 
**first_seen** | **str** |  | [optional] 
**last_seen** | **str** |  | [optional] 
**signal_labels** | **Dict[str, str]** |  | [optional] 
**enrichment_results** | [**EnrichmentResults**](EnrichmentResults.md) |  | [optional] 

## Example

```python
from holmesgpt_api_client.models.incident_request import IncidentRequest

# TODO update the JSON string below
json = "{}"
# create an instance of IncidentRequest from a JSON string
incident_request_instance = IncidentRequest.from_json(json)
# print the JSON string representation of the object
print(IncidentRequest.to_json())

# convert the object into a dict
incident_request_dict = incident_request_instance.to_dict()
# create an instance of IncidentRequest from a dict
incident_request_from_dict = IncidentRequest.from_dict(incident_request_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


