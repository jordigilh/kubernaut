# TimeoutConfig

Timeout configuration for RemediationRequest (BR-ORCH-027/028, Gap

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**var_global** | **str** | Global timeout (Go duration string, e.g., \&quot;30m\&quot;, \&quot;1h\&quot;) | [optional] 
**processing** | **str** | Processing phase timeout (Go duration string) | [optional] 
**analyzing** | **str** | Analyzing phase timeout (Go duration string) | [optional] 
**executing** | **str** | Executing phase timeout (Go duration string) | [optional] 

## Example

```python
from datastorage.models.timeout_config import TimeoutConfig

# TODO update the JSON string below
json = "{}"
# create an instance of TimeoutConfig from a JSON string
timeout_config_instance = TimeoutConfig.from_json(json)
# print the JSON string representation of the object
print(TimeoutConfig.to_json())

# convert the object into a dict
timeout_config_dict = timeout_config_instance.to_dict()
# create an instance of TimeoutConfig from a dict
timeout_config_from_dict = TimeoutConfig.from_dict(timeout_config_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


