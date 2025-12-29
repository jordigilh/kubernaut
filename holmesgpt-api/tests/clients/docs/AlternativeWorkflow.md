# AlternativeWorkflow

Alternative workflow recommendation for operator context.  Design Decision: ADR-045 v1.2 (Alternative Workflows for Audit)  IMPORTANT: Alternatives are for CONTEXT, not EXECUTION. Per APPROVAL_REJECTION_BEHAVIOR_DETAILED.md: - ✅ Purpose: Help operator make an informed decision - ✅ Content: Pros/cons of alternative approaches - ❌ NOT: A fallback queue for automatic execution  Only `selected_workflow` is executed. Alternatives provide: - Audit trail of what options were considered - Context for operator approval decisions - Transparency into AI reasoning

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_id** | **str** | Workflow identifier | 
**container_image** | **str** |  | [optional] 
**confidence** | **float** | Confidence score for this alternative | 
**rationale** | **str** | Why this alternative was considered but not selected | 

## Example

```python
from holmesgpt_api_client.models.alternative_workflow import AlternativeWorkflow

# TODO update the JSON string below
json = "{}"
# create an instance of AlternativeWorkflow from a JSON string
alternative_workflow_instance = AlternativeWorkflow.from_json(json)
# print the JSON string representation of the object
print(AlternativeWorkflow.to_json())

# convert the object into a dict
alternative_workflow_dict = alternative_workflow_instance.to_dict()
# create an instance of AlternativeWorkflow from a dict
alternative_workflow_from_dict = AlternativeWorkflow.from_dict(alternative_workflow_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


