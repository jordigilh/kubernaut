# IncidentResponseDataSelectedWorkflow

Selected workflow with workflow_id, containerImage, confidence, parameters (optional)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**workflow_id** | **str** |  | [optional] 
**container_image** | **str** |  | [optional] 
**confidence** | **float** |  | [optional] 
**parameters** | **Dict[str, object]** |  | [optional] 

## Example

```python
from datastorage.models.incident_response_data_selected_workflow import IncidentResponseDataSelectedWorkflow

# TODO update the JSON string below
json = "{}"
# create an instance of IncidentResponseDataSelectedWorkflow from a JSON string
incident_response_data_selected_workflow_instance = IncidentResponseDataSelectedWorkflow.from_json(json)
# print the JSON string representation of the object
print(IncidentResponseDataSelectedWorkflow.to_json())

# convert the object into a dict
incident_response_data_selected_workflow_dict = incident_response_data_selected_workflow_instance.to_dict()
# create an instance of IncidentResponseDataSelectedWorkflow from a dict
incident_response_data_selected_workflow_from_dict = IncidentResponseDataSelectedWorkflow.from_dict(incident_response_data_selected_workflow_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


