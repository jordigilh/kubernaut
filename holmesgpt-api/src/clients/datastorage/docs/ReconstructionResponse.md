# ReconstructionResponse

Response for RemediationRequest reconstruction from audit trail. Contains the reconstructed RR in YAML format and validation results. Implements BR-AUDIT-006 (SOC2 compliance). 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**remediation_request_yaml** | **str** | Complete RemediationRequest CRD in YAML format. Can be applied directly to Kubernetes cluster with kubectl. Includes TypeMeta, ObjectMeta, Spec, and Status.  | 
**validation** | [**ValidationResult**](ValidationResult.md) |  | 
**reconstructed_at** | **datetime** | Timestamp when reconstruction was performed.  | [optional] 
**correlation_id** | **str** | Correlation ID used for reconstruction.  | [optional] 

## Example

```python
from datastorage.models.reconstruction_response import ReconstructionResponse

# TODO update the JSON string below
json = "{}"
# create an instance of ReconstructionResponse from a JSON string
reconstruction_response_instance = ReconstructionResponse.from_json(json)
# print the JSON string representation of the object
print ReconstructionResponse.to_json()

# convert the object into a dict
reconstruction_response_dict = reconstruction_response_instance.to_dict()
# create an instance of ReconstructionResponse from a dict
reconstruction_response_form_dict = reconstruction_response.from_dict(reconstruction_response_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


