# RecoveryResponse

Response model for recovery analysis endpoint  Business Requirement: BR-HAPI-002 (Recovery response schema) Business Requirement: BR-AI-080 (Recovery attempt support) Business Requirement: BR-AI-081 (Previous execution context handling)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**incident_id** | **str** | Incident identifier from request | 
**can_recover** | **bool** | Whether recovery is possible | 
**strategies** | [**List[RecoveryStrategy]**](RecoveryStrategy.md) | Recommended recovery strategies | [optional] 
**primary_recommendation** | **str** |  | [optional] 
**analysis_confidence** | **float** | Overall confidence | 
**warnings** | **List[str]** | Warnings about recovery | [optional] 
**metadata** | **Dict[str, object]** | Additional metadata | [optional] 
**selected_workflow** | **Dict[str, object]** |  | [optional] 
**recovery_analysis** | **Dict[str, object]** |  | [optional] 
**needs_human_review** | **bool** | Whether human review is needed (BR-HAPI-197) | [optional] [default to False]
**human_review_reason** | **str** |  | [optional] 

## Example

```python
from holmesgpt_api_client.models.recovery_response import RecoveryResponse

# TODO update the JSON string below
json = "{}"
# create an instance of RecoveryResponse from a JSON string
recovery_response_instance = RecoveryResponse.from_json(json)
# print the JSON string representation of the object
print(RecoveryResponse.to_json())

# convert the object into a dict
recovery_response_dict = recovery_response_instance.to_dict()
# create an instance of RecoveryResponse from a dict
recovery_response_from_dict = RecoveryResponse.from_dict(recovery_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


