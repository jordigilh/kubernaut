# ActionTypeWorkflowCountResponse

Response containing the count of active workflows referencing an action type

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**count** | **int** | Number of active RemediationWorkflows referencing this action type | 

## Example

```python
from datastorage.models.action_type_workflow_count_response import ActionTypeWorkflowCountResponse

# TODO update the JSON string below
json = "{}"
# create an instance of ActionTypeWorkflowCountResponse from a JSON string
action_type_workflow_count_response_instance = ActionTypeWorkflowCountResponse.from_json(json)
# print the JSON string representation of the object
print ActionTypeWorkflowCountResponse.to_json()

# convert the object into a dict
action_type_workflow_count_response_dict = action_type_workflow_count_response_instance.to_dict()
# create an instance of ActionTypeWorkflowCountResponse from a dict
action_type_workflow_count_response_form_dict = action_type_workflow_count_response.from_dict(action_type_workflow_count_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


