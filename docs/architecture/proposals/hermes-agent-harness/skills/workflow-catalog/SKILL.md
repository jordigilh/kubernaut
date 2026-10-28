---
name: "workflow-catalog"
description: "Search and select remediation workflows from the Kubernaut catalog. Use after RCA is complete to find a matching fix."
---

# Workflow Catalog Skill

You have access to the Kubernaut workflow catalog via the DataStorage API. Each workflow is a pre-tested, versioned remediation recipe.

## Available Actions

### 1. List Available Action Types
```python
import urllib.request, json
DS_URL = "https://data-storage-service.kubernaut-system.svc.cluster.local:8443"
CA_FILE = "/etc/tls-ca/ca.crt"

import ssl
ctx = ssl.create_default_context(cafile=CA_FILE)

req = urllib.request.Request(f"{DS_URL}/api/v1/action-types")
req.add_header("Authorization", f"Bearer {open('/var/run/secrets/kubernetes.io/serviceaccount/token').read()}")
resp = urllib.request.urlopen(req, context=ctx, timeout=10)
action_types = json.loads(resp.read())
for at in action_types:
    print(f"  {at['name']}: {at['action_type']} (workflows: {at.get('workflow_count', '?')})")
```

### 2. Search Workflows by Action Type and Component
```python
# Search for workflows matching the target resource kind and action type
params = f"action_type=RollbackDeployment&component=apps/v1/Deployment"
req = urllib.request.Request(f"{DS_URL}/api/v1/workflows?{params}")
req.add_header("Authorization", f"Bearer {open('/var/run/secrets/kubernetes.io/serviceaccount/token').read()}")
resp = urllib.request.urlopen(req, context=ctx, timeout=10)
workflows = json.loads(resp.read())
for wf in workflows:
    print(f"  {wf['name']} v{wf['version']}: {wf['description']['what']}")
    print(f"    whenToUse: {wf['description']['when_to_use']}")
    print(f"    whenNotToUse: {wf['description']['when_not_to_use']}")
    print(f"    engine: {wf['execution']['engine']}")
    print(f"    bundle: {wf['execution']['bundle']}")
```

### 3. Get Workflow Details (parameters schema)
```python
req = urllib.request.Request(f"{DS_URL}/api/v1/workflows/{WORKFLOW_ID}")
req.add_header("Authorization", f"Bearer {open('/var/run/secrets/kubernetes.io/serviceaccount/token').read()}")
resp = urllib.request.urlopen(req, context=ctx, timeout=10)
wf = json.loads(resp.read())
print(f"Workflow: {wf['name']} v{wf['version']}")
print(f"Action: {wf['action_type']}")
print(f"Parameters:")
for param in wf.get('parameters', []):
    print(f"  {param['name']} ({param['type']}): required={param.get('required', False)} default={param.get('default', 'none')}")
```

## Workflow Selection Logic

1. Determine the action type from RCA:
   - ImagePullBackOff + previous revision → `RollbackDeployment`
   - OOMKilled + scaling viable → `ScaleReplicas` or `IncreaseMemoryLimits`
   - CrashLoopBackOff + recent deploy → `RollbackDeployment`
   - CrashLoopBackOff + no recent deploy → `RestartPod` (if transient) or `manual`
   - Node NotReady → `CordonDrainNode`
   - Certificate expired → `FixCertificate`

2. Search the catalog for that action type + target component kind

3. Check workflow preconditions against what you observed:
   - `RollbackDeployment` requires: previous healthy revision exists (verified via ReplicaSet history)
   - `IncreaseMemoryLimits` requires: deployment has resource limits set
   - `ScaleReplicas` requires: deployment supports horizontal scaling (not a StatefulSet with PVC-per-replica issues)
   - `CordonDrainNode` requires: node is accessible

4. Fill in parameters:
   - `TARGET_RESOURCE_NAME`: the deployment/pod/node name
   - `TARGET_NAMESPACE`: the namespace
   - `REVISION`: target revision number (for rollback)
   - Any workflow-specific params from the schema

## Known Workflows on This Cluster

| Action Type | Workflow | Target |
|-------------|----------|--------|
| RollbackDeployment | `rollback-deployment-v1`, `crashloop-rollback-v1` | Deployment |
| IncreaseMemoryLimits | `increase-memory-limits-v1` | Deployment |
| ScaleReplicas | `scale-replicas-v1` | Deployment |
| GracefulRestart | `graceful-restart-v1` | Pod |
| CordonDrainNode | `cordon-drain-v1` | Node |
| FixCertificate | `fix-certificate-v1` | Pod (cert-manager) |
| FixAuthorizationPolicy | `fix-authz-policy-v1` | Deployment (Istio) |
| HelmRollback | `helm-rollback-v1` | Deployment (Helm) |
| PatchConfiguration | `hotfix-config-v1` | Deployment/ConfigMap |
| ExpandPersistentVolumeClaim | `expand-pvc-v1` | PVC |

## Important Rules

- NEVER recommend a workflow if its preconditions are not satisfied by your RCA observations
- ALWAYS fill in ALL required parameters from the workflow schema
- If multiple workflows match, prefer the one whose `whenToUse` best matches the observed situation
- If the workflow's `whenNotToUse` matches the situation, skip it and look for alternatives
