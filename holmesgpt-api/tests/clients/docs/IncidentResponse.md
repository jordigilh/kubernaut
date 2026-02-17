# IncidentResponse

Response model for incident analysis endpoint  Business Requirement: BR-HAPI-002 (Incident analysis response schema) Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation) Design Decision: ADR-045 v1.2 (Alternative Workflows for Audit) Design Decision: ADR-055 (LLM-Driven Context Enrichment)  Fields added per AIAnalysis team requests: - warnings: Non-fatal warnings for transparency (Dec 2, 2025) - alternative_workflows: Other workflows considered (Dec 5, 2025) - INFORMATIONAL ONLY - needs_human_review: AI could not produce reliable result (Dec 6, 2025)  ADR-055: target_in_owner_chain removed - replaced by affected_resource in Rego input.

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**incident_id** | **str** | Incident identifier from request | 
**analysis** | **str** | Natural language analysis from LLM | 
**root_cause_analysis** | **Dict[str, object]** | Structured RCA with summary, severity, contributing_factors | 
**selected_workflow** | **Dict[str, object]** |  | [optional] 
**confidence** | **float** | Overall confidence in analysis | 
**timestamp** | **str** | ISO timestamp of analysis completion | 
**needs_human_review** | **bool** | True when AI analysis could not produce a reliable result. Reasons include: workflow validation failures after retries, LLM parsing errors, no suitable workflow found, or other AI reliability issues. When true, AIAnalysis should NOT create WorkflowExecution - requires human intervention. Check &#39;human_review_reason&#39; for structured reason or &#39;warnings&#39; for details. | [optional] [default to False]
**human_review_reason** | [**HumanReviewReason**](HumanReviewReason.md) |  | [optional] 
**warnings** | **List[str]** | Non-fatal warnings (e.g., low confidence) | [optional] 
**alternative_workflows** | [**List[AlternativeWorkflow]**](AlternativeWorkflow.md) | Other workflows considered but not selected. For operator context and audit trail only - NOT for automatic fallback execution. Helps operators understand AI reasoning. | [optional] 
**validation_attempts_history** | [**List[ValidationAttempt]**](ValidationAttempt.md) | History of all validation attempts during LLM self-correction. Each attempt records workflow_id, validation result, and any errors. Empty if validation passed on first attempt or no workflow was selected. | [optional] 

## Example

```python
from holmesgpt_api_client.models.incident_response import IncidentResponse

# TODO update the JSON string below
json = "{}"
# create an instance of IncidentResponse from a JSON string
incident_response_instance = IncidentResponse.from_json(json)
# print the JSON string representation of the object
print(IncidentResponse.to_json())

# convert the object into a dict
incident_response_dict = incident_response_instance.to_dict()
# create an instance of IncidentResponse from a dict
incident_response_from_dict = IncidentResponse.from_dict(incident_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


