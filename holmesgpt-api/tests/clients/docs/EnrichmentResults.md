# EnrichmentResults

Enrichment results from SignalProcessing.  Contains Kubernetes context, auto-detected labels, and custom labels that are used for workflow filtering and LLM context.  Design Decision: DD-RECOVERY-003, DD-HAPI-001  Custom Labels (DD-HAPI-001): - Format: map[string][]string (subdomain → list of values) - Keys are subdomains (e.g., \"constraint\", \"team\") - Values are lists of strings (boolean keys or \"key=value\" pairs) - Example: {\"constraint\": [\"cost-constrained\"], \"team\": [\"name=payments\"]} - Auto-appended to MCP workflow search (invisible to LLM)

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**kubernetes_context** | **Dict[str, object]** |  | [optional] 
**detected_labels** | [**DetectedLabels**](DetectedLabels.md) |  | [optional] 
**custom_labels** | **Dict[str, List[str]]** |  | [optional] 
**enrichment_quality** | **float** | Quality score of enrichment (0-1) | [optional] [default to 0.0]

## Example

```python
from holmesgpt_api_client.models.enrichment_results import EnrichmentResults

# TODO update the JSON string below
json = "{}"
# create an instance of EnrichmentResults from a JSON string
enrichment_results_instance = EnrichmentResults.from_json(json)
# print the JSON string representation of the object
print(EnrichmentResults.to_json())

# convert the object into a dict
enrichment_results_dict = enrichment_results_instance.to_dict()
# create an instance of EnrichmentResults from a dict
enrichment_results_from_dict = EnrichmentResults.from_dict(enrichment_results_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


