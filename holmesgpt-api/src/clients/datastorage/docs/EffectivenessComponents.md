# EffectivenessComponents

Individual component assessment scores.

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**health_assessed** | **bool** | Whether health component has been assessed. | [optional] 
**health_score** | **float** | Health pass rate score (0.0 to 1.0). | [optional] 
**health_details** | **str** | Human-readable health assessment details. | [optional] 
**alert_assessed** | **bool** | Whether alert component has been assessed. | [optional] 
**alert_score** | **float** | Alert resolution score (0.0 to 1.0). | [optional] 
**alert_details** | **str** | Human-readable alert assessment details. | [optional] 
**metrics_assessed** | **bool** | Whether metrics component has been assessed. | [optional] 
**metrics_score** | **float** | Metric improvement ratio score (0.0 to 1.0). | [optional] 
**metrics_details** | **str** | Human-readable metrics assessment details. | [optional] 

## Example

```python
from datastorage.models.effectiveness_components import EffectivenessComponents

# TODO update the JSON string below
json = "{}"
# create an instance of EffectivenessComponents from a JSON string
effectiveness_components_instance = EffectivenessComponents.from_json(json)
# print the JSON string representation of the object
print EffectivenessComponents.to_json()

# convert the object into a dict
effectiveness_components_dict = effectiveness_components_instance.to_dict()
# create an instance of EffectivenessComponents from a dict
effectiveness_components_form_dict = effectiveness_components.from_dict(effectiveness_components_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


