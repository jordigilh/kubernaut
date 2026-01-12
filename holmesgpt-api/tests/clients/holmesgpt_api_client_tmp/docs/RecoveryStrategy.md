# RecoveryStrategy

Individual recovery strategy option

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**action_type** | **str** | Type of recovery action | 
**confidence** | **float** | Confidence in strategy | 
**rationale** | **str** | Why this strategy is recommended | 
**estimated_risk** | **str** | Risk level | 
**prerequisites** | **List[str]** | Prerequisites for execution | [optional] 

## Example

```python
from holmesgpt_api_client.models.recovery_strategy import RecoveryStrategy

# TODO update the JSON string below
json = "{}"
# create an instance of RecoveryStrategy from a JSON string
recovery_strategy_instance = RecoveryStrategy.from_json(json)
# print the JSON string representation of the object
print(RecoveryStrategy.to_json())

# convert the object into a dict
recovery_strategy_dict = recovery_strategy_instance.to_dict()
# create an instance of RecoveryStrategy from a dict
recovery_strategy_from_dict = RecoveryStrategy.from_dict(recovery_strategy_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


