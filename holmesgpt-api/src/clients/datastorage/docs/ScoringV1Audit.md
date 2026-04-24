# ScoringV1Audit

V1.0 scoring (confidence only per DD-WORKFLOW-004 v2.0)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**confidence** | **float** | Overall confidence score (0.0-1.0) | 

## Example

```python
from datastorage.models.scoring_v1_audit import ScoringV1Audit

# TODO update the JSON string below
json = "{}"
# create an instance of ScoringV1Audit from a JSON string
scoring_v1_audit_instance = ScoringV1Audit.from_json(json)
# print the JSON string representation of the object
print ScoringV1Audit.to_json()

# convert the object into a dict
scoring_v1_audit_dict = scoring_v1_audit_instance.to_dict()
# create an instance of ScoringV1Audit from a dict
scoring_v1_audit_form_dict = scoring_v1_audit.from_dict(scoring_v1_audit_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


