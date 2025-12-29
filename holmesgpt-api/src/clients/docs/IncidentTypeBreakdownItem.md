# IncidentTypeBreakdownItem


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**incident_type** | **str** | Incident type identifier | 
**executions** | **int** | Number of times this workflow was used for this incident type | 
**success_rate** | **float** | Success rate for this specific incident type | 

## Example

```python
from datastorage.models.incident_type_breakdown_item import IncidentTypeBreakdownItem

# TODO update the JSON string below
json = "{}"
# create an instance of IncidentTypeBreakdownItem from a JSON string
incident_type_breakdown_item_instance = IncidentTypeBreakdownItem.from_json(json)
# print the JSON string representation of the object
print(IncidentTypeBreakdownItem.to_json())

# convert the object into a dict
incident_type_breakdown_item_dict = incident_type_breakdown_item_instance.to_dict()
# create an instance of IncidentTypeBreakdownItem from a dict
incident_type_breakdown_item_from_dict = IncidentTypeBreakdownItem.from_dict(incident_type_breakdown_item_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


