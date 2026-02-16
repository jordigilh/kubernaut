# RemediationHistoryTier1

Tier 1: Detailed remediation chain within the recent window (default 24h). Contains full effectiveness data, health checks, and metric deltas. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**window** | **str** | Lookback window used for this tier (e.g. \&quot;24h\&quot;) | 
**chain** | [**List[RemediationHistoryEntry]**](RemediationHistoryEntry.md) | Ordered list of detailed remediation entries (ascending by completedAt) | 

## Example

```python
from datastorage.models.remediation_history_tier1 import RemediationHistoryTier1

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationHistoryTier1 from a JSON string
remediation_history_tier1_instance = RemediationHistoryTier1.from_json(json)
# print the JSON string representation of the object
print RemediationHistoryTier1.to_json()

# convert the object into a dict
remediation_history_tier1_dict = remediation_history_tier1_instance.to_dict()
# create an instance of RemediationHistoryTier1 from a dict
remediation_history_tier1_form_dict = remediation_history_tier1.from_dict(remediation_history_tier1_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


