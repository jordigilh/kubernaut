# AIAgentEnrichmentCompletedPayload

AI Agent Phase 2 enrichment completed event payload (aiagent.enrichment.completed) - SOC2 CC8.1, Issue

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**event_id** | **str** | Unique event identifier | 
**incident_id** | **str** | Incident correlation ID from request | 
**root_owner_kind** | **str** | Resolved root owner resource kind (e.g., Deployment, StatefulSet) | 
**root_owner_name** | **str** | Resolved root owner resource name | 
**root_owner_namespace** | **str** | Resolved root owner namespace (empty for cluster-scoped resources) | [optional] 
**owner_chain_length** | **int** | Number of resources in the K8s owner chain (1 &#x3D; no parent) | 
**detected_labels_summary** | **object** | Infrastructure labels detected by LabelDetector (null when detector unavailable) | [optional] 
**failed_detections** | **List[str]** | Labels that could not be detected (null when all succeeded or detector unavailable) | [optional] 
**remediation_history_fetched** | **bool** | Whether remediation history was successfully fetched from DataStorage | 

## Example

```python
from datastorage.models.ai_agent_enrichment_completed_payload import AIAgentEnrichmentCompletedPayload

# TODO update the JSON string below
json = "{}"
# create an instance of AIAgentEnrichmentCompletedPayload from a JSON string
ai_agent_enrichment_completed_payload_instance = AIAgentEnrichmentCompletedPayload.from_json(json)
# print the JSON string representation of the object
print AIAgentEnrichmentCompletedPayload.to_json()

# convert the object into a dict
ai_agent_enrichment_completed_payload_dict = ai_agent_enrichment_completed_payload_instance.to_dict()
# create an instance of AIAgentEnrichmentCompletedPayload from a dict
ai_agent_enrichment_completed_payload_form_dict = ai_agent_enrichment_completed_payload.from_dict(ai_agent_enrichment_completed_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


