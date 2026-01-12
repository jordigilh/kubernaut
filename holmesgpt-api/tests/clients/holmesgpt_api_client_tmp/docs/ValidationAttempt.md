# ValidationAttempt

Record of a single validation attempt during LLM self-correction.  Business Requirement: BR-HAPI-197 (needs_human_review field) Design Decision: DD-HAPI-002 v1.2 (Workflow Response Validation)  Used for: 1. Operator notification - natural language description of why validation failed 2. Audit trail - complete history of all validation attempts 3. Debugging - understand LLM behavior when workflows fail

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**attempt** | **int** | Attempt number (1-indexed) | 
**workflow_id** | **str** |  | [optional] 
**is_valid** | **bool** | Whether validation passed | 
**errors** | **List[str]** | Validation errors (empty if valid) | [optional] 
**timestamp** | **str** | ISO timestamp of validation attempt | 

## Example

```python
from holmesgpt_api_client.models.validation_attempt import ValidationAttempt

# TODO update the JSON string below
json = "{}"
# create an instance of ValidationAttempt from a JSON string
validation_attempt_instance = ValidationAttempt.from_json(json)
# print the JSON string representation of the object
print(ValidationAttempt.to_json())

# convert the object into a dict
validation_attempt_dict = validation_attempt_instance.to_dict()
# create an instance of ValidationAttempt from a dict
validation_attempt_from_dict = ValidationAttempt.from_dict(validation_attempt_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


