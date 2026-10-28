---
name: "k8s-investigate"
description: "Investigate a Kubernetes resource for incidents. Use when given a resource kind, name, and namespace to diagnose."
---

# Kubernetes Investigation Skill

You have access to a Kubernetes cluster via the Python `kubernetes` client. The cluster credentials are already configured (in-cluster ServiceAccount).

## Available Actions

### 1. Get Pod Status
```python
from kubernetes import client, config
config.load_incluster_config()
v1 = client.CoreV1Api()
pods = v1.list_namespaced_pod(namespace=NAMESPACE, label_selector=f"app={APP_LABEL}")
for pod in pods.items:
    print(f"{pod.metadata.name}: {pod.status.phase} - {[cs.state for cs in (pod.status.container_statuses or [])]}")
```

### 2. Describe Resource (Deployment)
```python
apps_v1 = client.AppsV1Api()
dep = apps_v1.read_namespaced_deployment(name=NAME, namespace=NAMESPACE)
print(f"Replicas: desired={dep.spec.replicas} ready={dep.status.ready_replicas} available={dep.status.available_replicas}")
print(f"Generation: {dep.metadata.generation}")
print(f"Image: {dep.spec.template.spec.containers[0].image}")
print(f"Strategy: {dep.spec.strategy.type}")
for cond in (dep.status.conditions or []):
    print(f"  Condition: {cond.type}={cond.status} reason={cond.reason} message={cond.message}")
```

### 3. Get Events for Resource
```python
events = v1.list_namespaced_event(namespace=NAMESPACE, field_selector=f"involvedObject.name={NAME}")
for event in sorted(events.items, key=lambda e: e.last_timestamp or e.event_time or "", reverse=True)[:10]:
    print(f"  {event.reason}: {event.message} (count={event.count}, last={event.last_timestamp})")
```

### 4. Get Pod Logs
```python
logs = v1.read_namespaced_pod_log(name=POD_NAME, namespace=NAMESPACE, tail_lines=50)
print(logs)
```

### 5. Get Rollout History (ReplicaSets)
```python
rs_list = apps_v1.list_namespaced_replica_set(namespace=NAMESPACE, label_selector=f"app={APP_LABEL}")
for rs in sorted(rs_list.items, key=lambda r: r.metadata.annotations.get("deployment.kubernetes.io/revision", "0"), reverse=True):
    rev = rs.metadata.annotations.get("deployment.kubernetes.io/revision", "?")
    image = rs.spec.template.spec.containers[0].image if rs.spec.template.spec.containers else "?"
    print(f"  Revision {rev}: replicas={rs.status.replicas}/{rs.spec.replicas} image={image}")
```

### 6. Get Node Status
```python
node = v1.read_node(name=NODE_NAME)
for cond in node.status.conditions:
    print(f"  {cond.type}: {cond.status} (reason={cond.reason})")
```

### 7. Query Prometheus (if available)
```python
import urllib.request, json
PROM_URL = "http://prometheus.monitoring.svc:9090"
query = f'container_memory_usage_bytes{{namespace="{NAMESPACE}",pod=~"{POD_PREFIX}.*"}}'
resp = urllib.request.urlopen(f"{PROM_URL}/api/v1/query?query={query}", timeout=10)
data = json.loads(resp.read())
for result in data.get("data", {}).get("result", []):
    print(f"  {result['metric'].get('pod')}: {int(result['value'][1])/1024/1024:.0f}Mi")
```

## Investigation Workflow

When asked to investigate `Kind/Name` in `Namespace`:

1. First, get pod status for the target (check for CrashLoopBackOff, ImagePullBackOff, OOMKilled, Pending)
2. Get events for the resource (look for FailedScheduling, FailedCreate, BackOff, Unhealthy)
3. If pods exist but failing, read logs (look for application errors, stack traces, OOM messages)
4. Check the deployment spec (image tag, resource limits, replicas)
5. Check rollout history (ReplicaSets) -- identify if there's a previous healthy revision
6. If OOM suspected, query Prometheus for memory usage trend

## Key Diagnosis Patterns

| Symptom | Likely Root Cause | Check |
|---------|------------------|-------|
| ImagePullBackOff | Bad image tag or missing credentials | Events show "failed to pull" or "unauthorized" |
| CrashLoopBackOff | Application crash or misconfiguration | Pod logs show error; check if recent deploy |
| OOMKilled | Memory limit too low or memory leak | container_statuses shows OOMKilled; check limits vs usage |
| Pending | No schedulable node | Events show FailedScheduling; check node conditions |
| CreateContainerError | Missing ConfigMap/Secret or bad command | Events show specific mount/config error |

## Important Rules

- ALWAYS check ReplicaSet history before concluding "no rollback target exists"
- If metadata.generation > 1 AND multiple ReplicaSets exist, there ARE previous revisions
- A pod in ImagePullBackOff with a running sibling from an older ReplicaSet = rollback IS possible
- Count RUNNING pods from older ReplicaSets as evidence of a valid rollback target
