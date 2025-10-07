## RBAC Configuration

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: alertremediation-controller
rules:
# RemediationRequest CRD permissions
- apiGroups: ["kubernaut.io"]
  resources: ["alertremediations", "alertremediations/status", "alertremediations/finalizers"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Service CRD creation permissions
- apiGroups: ["remediationprocessing.kubernaut.io"]
  resources: ["alertprocessings"]
  verbs: ["create", "get", "list", "watch"]

- apiGroups: ["aianalysis.kubernaut.io"]
  resources: ["aianalyses"]
  verbs: ["create", "get", "list", "watch"]

- apiGroups: ["workflowexecution.kubernaut.io"]
  resources: ["workflowexecutions"]
  verbs: ["create", "get", "list", "watch"]

- apiGroups: ["kubernetesexecution.kubernaut.io"]
  resources: ["kubernetesexecutions"]
  verbs: ["create", "get", "list", "watch"]

# Event emission
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

**Note**: RemediationRequest controller creates all service CRDs but only needs read permissions on their status (watches handle updates).

---

