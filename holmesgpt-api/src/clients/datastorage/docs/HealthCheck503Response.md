# HealthCheck503Response


## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**status** | **str** |  | [optional] 
**error** | **str** |  | [optional] 

## Example

```python
from datastorage.models.health_check503_response import HealthCheck503Response

# TODO update the JSON string below
json = "{}"
# create an instance of HealthCheck503Response from a JSON string
health_check503_response_instance = HealthCheck503Response.from_json(json)
# print the JSON string representation of the object
print HealthCheck503Response.to_json()

# convert the object into a dict
health_check503_response_dict = health_check503_response_instance.to_dict()
# create an instance of HealthCheck503Response from a dict
health_check503_response_form_dict = health_check503_response.from_dict(health_check503_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


