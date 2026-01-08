# MandatoryLabels

5 mandatory workflow labels (DD-WORKFLOW-001 v2.3)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**signal_type** | **str** | Signal type this workflow handles (e.g., OOMKilled, CrashLoopBackOff) | 
**severity** | **str** | Severity level this workflow is designed for | 
**component** | **str** | Kubernetes resource type this workflow targets (e.g., pod, deployment, node) | 
**environment** | **str** | Target environment (production, staging, development, test, * for any) | 
**priority** | **str** | Business priority level (P0, P1, P2, P3, * for any) | 

## Example

```python
from datastorage.models.mandatory_labels import MandatoryLabels

# TODO update the JSON string below
json = "{}"
# create an instance of MandatoryLabels from a JSON string
mandatory_labels_instance = MandatoryLabels.from_json(json)
# print the JSON string representation of the object
print MandatoryLabels.to_json()

# convert the object into a dict
mandatory_labels_dict = mandatory_labels_instance.to_dict()
# create an instance of MandatoryLabels from a dict
mandatory_labels_form_dict = mandatory_labels.from_dict(mandatory_labels_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


