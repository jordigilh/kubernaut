# DetectedLabels

Auto-detected cluster characteristics from SignalProcessing.  These labels are used for: 1. LLM context (natural language) - help LLM understand cluster environment 2. MCP workflow filtering - filter workflows to only compatible ones  Design Decision: DD-WORKFLOW-001 v2.2, DD-RECOVERY-003  Changes: - v2.1: Added `failedDetections` field to track which detections failed - v2.2: Removed `podSecurityLevel` (PSP deprecated, PSS is namespace-level)  Consumer logic: if field is in failedDetections, ignore its value

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**failed_detections** | **List[str]** | Field names where detection failed. Consumer should ignore values of these fields. Valid values: gitOpsManaged, pdbProtected, hpaEnabled, stateful, helmManaged, networkIsolated, serviceMesh | [optional] 
**git_ops_managed** | **bool** | Whether namespace is managed by GitOps | [optional] [default to False]
**git_ops_tool** | **str** | GitOps tool: &#39;argocd&#39;, &#39;flux&#39;, or &#39;&#39; | [optional] [default to '']
**pdb_protected** | **bool** | Whether PodDisruptionBudget protects this workload | [optional] [default to False]
**hpa_enabled** | **bool** | Whether HorizontalPodAutoscaler is active | [optional] [default to False]
**stateful** | **bool** | Whether this is a stateful workload (StatefulSet or has PVCs) | [optional] [default to False]
**helm_managed** | **bool** | Whether resource is managed by Helm | [optional] [default to False]
**network_isolated** | **bool** | Whether NetworkPolicy restricts traffic | [optional] [default to False]
**service_mesh** | **str** | Service mesh: &#39;istio&#39;, &#39;linkerd&#39;, &#39;&#39; | [optional] [default to '']

## Example

```python
from holmesgpt_api_client.models.detected_labels import DetectedLabels

# TODO update the JSON string below
json = "{}"
# create an instance of DetectedLabels from a JSON string
detected_labels_instance = DetectedLabels.from_json(json)
# print the JSON string representation of the object
print(DetectedLabels.to_json())

# convert the object into a dict
detected_labels_dict = detected_labels_instance.to_dict()
# create an instance of DetectedLabels from a dict
detected_labels_from_dict = DetectedLabels.from_dict(detected_labels_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


