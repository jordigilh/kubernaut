# OriginalRCA

Summary of the original root cause analysis from initial AIAnalysis

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**summary** | **str** | Brief RCA summary from initial investigation | 
**signal_type** | **str** | Signal type determined by original RCA (e.g., &#39;OOMKilled&#39;) | 
**severity** | **str** | Severity determined by original RCA | 
**contributing_factors** | **List[str]** | Factors that contributed to the issue | [optional] 

## Example

```python
from holmesgpt_api_client.models.original_rca import OriginalRCA

# TODO update the JSON string below
json = "{}"
# create an instance of OriginalRCA from a JSON string
original_rca_instance = OriginalRCA.from_json(json)
# print the JSON string representation of the object
print(OriginalRCA.to_json())

# convert the object into a dict
original_rca_dict = original_rca_instance.to_dict()
# create an instance of OriginalRCA from a dict
original_rca_from_dict = OriginalRCA.from_dict(original_rca_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


