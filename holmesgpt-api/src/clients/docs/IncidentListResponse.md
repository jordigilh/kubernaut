# IncidentListResponse


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**data** | [**List[Incident]**](Incident.md) | Array of incidents matching the filter criteria | 
**pagination** | [**Pagination**](Pagination.md) |  | 

## Example

```python
from datastorage.models.incident_list_response import IncidentListResponse

# TODO update the JSON string below
json = "{}"
# create an instance of IncidentListResponse from a JSON string
incident_list_response_instance = IncidentListResponse.from_json(json)
# print the JSON string representation of the object
print(IncidentListResponse.to_json())

# convert the object into a dict
incident_list_response_dict = incident_list_response_instance.to_dict()
# create an instance of IncidentListResponse from a dict
incident_list_response_from_dict = IncidentListResponse.from_dict(incident_list_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


