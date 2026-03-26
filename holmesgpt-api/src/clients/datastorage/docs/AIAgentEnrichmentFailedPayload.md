# AIAgentEnrichmentFailedPayload

AI Agent Phase 2 enrichment failure event payload (aiagent.enrichment.failed) - SOC2 CC8.1, Issue

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**event_id** | **str** | Unique event identifier | 
**incident_id** | **str** | Incident correlation ID from request | 
**reason** | **str** | Failure reason from EnrichmentFailure (e.g., rca_incomplete) | 
**detail** | **str** | Detailed failure context including retry information | 
**affected_resource_kind** | **str** | Kind of the resource that was being enriched when failure occurred | 
**affected_resource_name** | **str** | Name of the resource that was being enriched when failure occurred | 
**affected_resource_namespace** | **str** | Namespace of the resource (empty for cluster-scoped) | [optional] 

## Example

```python
from datastorage.models.ai_agent_enrichment_failed_payload import AIAgentEnrichmentFailedPayload

# TODO update the JSON string below
json = "{}"
# create an instance of AIAgentEnrichmentFailedPayload from a JSON string
ai_agent_enrichment_failed_payload_instance = AIAgentEnrichmentFailedPayload.from_json(json)
# print the JSON string representation of the object
print AIAgentEnrichmentFailedPayload.to_json()

# convert the object into a dict
ai_agent_enrichment_failed_payload_dict = ai_agent_enrichment_failed_payload_instance.to_dict()
# create an instance of AIAgentEnrichmentFailedPayload from a dict
ai_agent_enrichment_failed_payload_form_dict = ai_agent_enrichment_failed_payload.from_dict(ai_agent_enrichment_failed_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


