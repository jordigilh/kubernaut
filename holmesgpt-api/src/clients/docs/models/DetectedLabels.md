# datastorage.model.detected_labels.DetectedLabels

Auto-detected labels from Kubernetes resources (DD-WORKFLOW-001 v2.3) - V1.0 structured types

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
dict, frozendict.frozendict,  | frozendict.frozendict,  | Auto-detected labels from Kubernetes resources (DD-WORKFLOW-001 v2.3) - V1.0 structured types | 

### Dictionary Keys
Key | Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | ------------- | -------------
**[failed_detections](#failed_detections)** | list, tuple,  | tuple,  | Fields where detection failed (RBAC, timeout, etc.) - consumer should ignore these fields | [optional] 
**gitOpsManaged** | bool,  | BoolClass,  | Resource is managed by GitOps (ArgoCD/Flux) | [optional] 
**gitOpsTool** | str,  | str,  | GitOps tool: argocd, flux, or * (wildcard &#x3D; any tool) | [optional] must be one of ["argocd", "flux", "*", ] 
**pdbProtected** | bool,  | BoolClass,  | PodDisruptionBudget protects this workload | [optional] 
**hpaEnabled** | bool,  | BoolClass,  | HorizontalPodAutoscaler is configured | [optional] 
**stateful** | bool,  | BoolClass,  | Workload uses persistent storage or is StatefulSet | [optional] 
**helmManaged** | bool,  | BoolClass,  | Resource is managed by Helm | [optional] 
**networkIsolated** | bool,  | BoolClass,  | NetworkPolicy restricts traffic | [optional] 
**serviceMesh** | str,  | str,  | Service mesh type: istio, linkerd, or * (wildcard &#x3D; any mesh) | [optional] must be one of ["istio", "linkerd", "*", ] 
**any_string_name** | dict, frozendict.frozendict, str, date, datetime, int, float, bool, decimal.Decimal, None, list, tuple, bytes, io.FileIO, io.BufferedReader | frozendict.frozendict, str, BoolClass, decimal.Decimal, NoneClass, tuple, bytes, FileIO | any string name can be used but the value must be the correct type | [optional]

# failed_detections

Fields where detection failed (RBAC, timeout, etc.) - consumer should ignore these fields

## Model Type Info
Input Type | Accessed Type | Description | Notes
------------ | ------------- | ------------- | -------------
list, tuple,  | tuple,  | Fields where detection failed (RBAC, timeout, etc.) - consumer should ignore these fields | 

### Tuple Items
Class Name | Input Type | Accessed Type | Description | Notes
------------- | ------------- | ------------- | ------------- | -------------
items | str,  | str,  |  | must be one of ["gitOpsManaged", "pdbProtected", "hpaEnabled", "stateful", "helmManaged", "networkIsolated", "serviceMesh", ] 

[[Back to Model list]](../../README.md#documentation-for-models) [[Back to API list]](../../README.md#documentation-for-api-endpoints) [[Back to README]](../../README.md)

