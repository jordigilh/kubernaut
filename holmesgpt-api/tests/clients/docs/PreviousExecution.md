# PreviousExecution

Complete context about the previous execution attempt that failed.  Business Requirement: BR-HAPI-192 (Recovery Context Consumption) - natural_language_summary is WE-generated LLM-friendly failure description - Provides context for better recovery workflow selection

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_execution_ref** | **str** | Name of failed WorkflowExecution CRD | 
**original_rca** | [**OriginalRCA**](OriginalRCA.md) | RCA from initial AIAnalysis | 
**selected_workflow** | [**SelectedWorkflowSummary**](SelectedWorkflowSummary.md) | Workflow that was executed | 
**failure** | [**ExecutionFailure**](ExecutionFailure.md) | Structured failure details | 
**natural_language_summary** | **str** |  | [optional] 

## Example

```python
from holmesgpt_api_client.models.previous_execution import PreviousExecution

# TODO update the JSON string below
json = "{}"
# create an instance of PreviousExecution from a JSON string
previous_execution_instance = PreviousExecution.from_json(json)
# print the JSON string representation of the object
print(PreviousExecution.to_json())

# convert the object into a dict
previous_execution_dict = previous_execution_instance.to_dict()
# create an instance of PreviousExecution from a dict
previous_execution_from_dict = PreviousExecution.from_dict(previous_execution_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


