# RemediationHistoryContext

Structured remediation history context for LLM prompt enrichment. Contains two tiers of remediation chain data for a target resource. Authority: DD-HAPI-016. 

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**target_resource** | **str** | Target resource identifier in format \&quot;{namespace}/{kind}/{name}\&quot;. Matches the format used by RO and EM audit events.  | 
**current_spec_hash** | **str** | SHA-256 hash of the current target resource spec (echoed from request).  | 
**regression_detected** | **bool** | True if any remediation entry&#39;s preRemediationSpecHash matches currentSpecHash, indicating configuration regression.  | 
**tier1** | [**RemediationHistoryTier1**](RemediationHistoryTier1.md) |  | 
**tier2** | [**RemediationHistoryTier2**](RemediationHistoryTier2.md) |  | 

## Example

```python
from datastorage.models.remediation_history_context import RemediationHistoryContext

# TODO update the JSON string below
json = "{}"
# create an instance of RemediationHistoryContext from a JSON string
remediation_history_context_instance = RemediationHistoryContext.from_json(json)
# print the JSON string representation of the object
print RemediationHistoryContext.to_json()

# convert the object into a dict
remediation_history_context_dict = remediation_history_context_instance.to_dict()
# create an instance of RemediationHistoryContext from a dict
remediation_history_context_form_dict = remediation_history_context.from_dict(remediation_history_context_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


