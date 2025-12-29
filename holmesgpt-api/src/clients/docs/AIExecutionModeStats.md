# AIExecutionModeStats


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**catalog_selected** | **int** | Number of times AI selected a single workflow from the catalog (90-95% expected per ADR-033 Hybrid Model)  | 
**chained** | **int** | Number of times AI chained multiple workflows together (4-9% expected per ADR-033 Hybrid Model)  | 
**manual_escalation** | **int** | Number of times AI escalated to human operator (&lt;1% expected per ADR-033 Hybrid Model)  | 

## Example

```python
from datastorage.models.ai_execution_mode_stats import AIExecutionModeStats

# TODO update the JSON string below
json = "{}"
# create an instance of AIExecutionModeStats from a JSON string
ai_execution_mode_stats_instance = AIExecutionModeStats.from_json(json)
# print the JSON string representation of the object
print(AIExecutionModeStats.to_json())

# convert the object into a dict
ai_execution_mode_stats_dict = ai_execution_mode_stats_instance.to_dict()
# create an instance of AIExecutionModeStats from a dict
ai_execution_mode_stats_from_dict = AIExecutionModeStats.from_dict(ai_execution_mode_stats_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


