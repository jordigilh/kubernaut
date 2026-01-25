# DetectedLabels

Auto-detected labels from Kubernetes resources (DD-WORKFLOW-001 v2.3) - V1.0 structured types

## Properties

Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**failed_detections** | **List[str]** | Fields where detection failed (RBAC, timeout, etc.) - consumer should ignore these fields | [optional] 
**git_ops_managed** | **bool** | Resource is managed by GitOps (ArgoCD/Flux) | [optional] 
**git_ops_tool** | **str** | GitOps tool: argocd, flux, or * (wildcard &#x3D; any tool) | [optional] 
**pdb_protected** | **bool** | PodDisruptionBudget protects this workload | [optional] 
**hpa_enabled** | **bool** | HorizontalPodAutoscaler is configured | [optional] 
**stateful** | **bool** | Workload uses persistent storage or is StatefulSet | [optional] 
**helm_managed** | **bool** | Resource is managed by Helm | [optional] 
**network_isolated** | **bool** | NetworkPolicy restricts traffic | [optional] 
**service_mesh** | **str** | Service mesh type: istio, linkerd, or * (wildcard &#x3D; any mesh) | [optional] 

## Example

```python
from datastorage.models.detected_labels import DetectedLabels

# TODO update the JSON string below
json = "{}"
# create an instance of DetectedLabels from a JSON string
detected_labels_instance = DetectedLabels.from_json(json)
# print the JSON string representation of the object
print DetectedLabels.to_json()

# convert the object into a dict
detected_labels_dict = detected_labels_instance.to_dict()
# create an instance of DetectedLabels from a dict
detected_labels_form_dict = detected_labels.from_dict(detected_labels_dict)
```
[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


