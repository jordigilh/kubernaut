# WorkflowValidationPayload

Workflow validation attempt event payload (workflow_validation_attempt)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**event_type** | **str** | Event type for discriminator (matches parent event_type) | 
**event_id** | **str** | Unique event identifier | 
**incident_id** | **str** | Incident correlation ID (remediation_id) | 
**attempt** | **int** | Current validation attempt number | 
**max_attempts** | **int** | Maximum validation attempts allowed | 
**is_valid** | **bool** | Whether validation succeeded | 
**errors** | **List[str]** | List of validation error messages | [optional] 
**validation_errors** | **str** | Combined validation error messages (for backward compatibility) | [optional] 
**workflow_id** | **str** | Workflow ID being validated | [optional] 
**workflow_name** | **str** | Name of workflow being validated | [optional] 
**human_review_reason** | **str** | Reason code if needs_human_review (final attempt) | [optional] 
**is_final_attempt** | **bool** | Whether this is the final validation attempt | [optional] [default to False]

## Example

```python
from datastorage.models.workflow_validation_payload import WorkflowValidationPayload

# TODO update the JSON string below
json = "{}"
# create an instance of WorkflowValidationPayload from a JSON string
workflow_validation_payload_instance = WorkflowValidationPayload.from_json(json)
# print the JSON string representation of the object
print WorkflowValidationPayload.to_json()

# convert the object into a dict
workflow_validation_payload_dict = workflow_validation_payload_instance.to_dict()
# create an instance of WorkflowValidationPayload from a dict
workflow_validation_payload_form_dict = workflow_validation_payload.from_dict(workflow_validation_payload_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


