# RemediationHistoryTier2

Tier 2: Summary remediation chain for historical lookback (default 90d). Activated when currentSpecHash matches a historical preRemediationSpecHash beyond the Tier 1 window. Contains compact entries without health/metric details. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**window** | **str** | Lookback window used for this tier (e.g. \&quot;2160h\&quot;) | 
**chain** | [**List[RemediationHistorySummary]**](RemediationHistorySummary.md) | Ordered list of summary remediation entries (ascending by completedAt) | 

## Example

```python
from datastorage.models.remediation_history_tier2 import RemediationHistoryTier2

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationHistoryTier2 from a JSON string
remediation_history_tier2_instance = RemediationHistoryTier2.from_json(json)
# print the JSON string representation of the object
print RemediationHistoryTier2.to_json()

# convert the object into a dict
remediation_history_tier2_dict = remediation_history_tier2_instance.to_dict()
# create an instance of RemediationHistoryTier2 from a dict
remediation_history_tier2_form_dict = remediation_history_tier2.from_dict(remediation_history_tier2_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


