# HashComparisonData

Pre/post remediation spec hash comparison data per DD-EM-002. Supplementary signal (not part of scoring formula). 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**pre_remediation_spec_hash** | **str** | Canonical SHA-256 hash of target resource spec before remediation. | [optional] 
**post_remediation_spec_hash** | **str** | Canonical SHA-256 hash of target resource spec after remediation. | [optional] 
**hash_match** | **bool** | Whether pre and post hashes match (true &#x3D; no spec change detected). | [optional] 

## Example

```python
from datastorage.models.hash_comparison_data import HashComparisonData

# TODO update the JSON string below
json = "{}"
# create an instance of HashComparisonData from a JSON string
hash_comparison_data_instance = HashComparisonData.from_json(json)
# print the JSON string representation of the object
print HashComparisonData.to_json()

# convert the object into a dict
hash_comparison_data_dict = hash_comparison_data_instance.to_dict()
# create an instance of HashComparisonData from a dict
hash_comparison_data_form_dict = hash_comparison_data.from_dict(hash_comparison_data_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


