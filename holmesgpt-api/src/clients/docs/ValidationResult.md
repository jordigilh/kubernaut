# ValidationResult

Validation result for reconstructed RemediationRequest. Indicates completeness, errors, and warnings. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**is_valid** | **bool** | Whether the RR passes validation (no blocking errors). true &#x3D; ready to apply, false &#x3D; missing required fields.  | 
**completeness** | **int** | Completeness percentage (0-100%) based on field presence. 100% &#x3D; all fields present (Gaps #1-8 complete). 50-99% &#x3D; required fields + some optional fields. 0-49% &#x3D; only required fields or incomplete.  | 
**errors** | **List[str]** | Blocking validation errors (missing required fields). Empty array if is_valid &#x3D; true.  | 
**warnings** | **List[str]** | Non-blocking warnings for missing optional fields. Helps identify gaps in reconstruction.  | 

## Example

```python
from datastorage.models.validation_result import ValidationResult

# TODO update the JSON string below
json = "{}"
# create an instance of ValidationResult from a JSON string
validation_result_instance = ValidationResult.from_json(json)
# print the JSON string representation of the object
print(ValidationResult.to_json())

# convert the object into a dict
validation_result_dict = validation_result_instance.to_dict()
# create an instance of ValidationResult from a dict
validation_result_from_dict = ValidationResult.from_dict(validation_result_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


