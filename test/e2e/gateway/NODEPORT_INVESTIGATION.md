# NodePort Investigation for Kind Cluster

**Date**: November 24, 2025
**Objective**: Eliminate port-forward instability by using NodePort with Kind's extraPortMappings
**Status**: 🔍 **INVESTIGATION**

## Problem Statement

### Current Architecture (Port-Forward)
```
Test Process → kubectl port-forward → Gateway Pod
     ↓              ↓ (unstable)          ↓
  8081-8084      crashes mid-test      :8080
```

**Issues**:
- kubectl port-forward crashes under concurrent load
- 12.5% failure rate with 8 processes
- 17% failure rate with 12 processes
- Unreliable for high-concurrency testing

### Proposed Architecture (NodePort)
```
Test Process → Kind Node → NodePort Service → Gateway Pod
     ↓            ↓            ↓                  ↓
  localhost    30080      NodePort 30080       :8080
```

**Benefits**:
- No kubectl port-forward dependency
- Direct network access
- More stable under load
- Production-like configuration

## Kind extraPortMappings Investigation

### How It Works

Kind supports exposing NodePort services to the host machine via `extraPortMappings` in the cluster configuration.

**Mechanism**:
1. Map host port → Kind node port
2. NodePort service exposes pod on node
3. Traffic flows: Host → Node → Service → Pod

### Configuration Example

```yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  extraPortMappings:
  - containerPort: 30080  # NodePort in cluster
    hostPort: 8080        # Port on host machine
    protocol: TCP
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
      extraArgs:
        max-requests-inflight: "800"
        max-mutating-requests-inflight: "400"
- role: worker
```

### Key Concepts

1. **extraPortMappings**: Maps host ports to container ports
   - `hostPort`: Port on your local machine (e.g., 8080)
   - `containerPort`: Port inside Kind container (e.g., 30080)
   - Must be on control-plane or worker node

2. **NodePort Service**: Exposes service on static port
   - Port range: 30000-32767 (default)
   - Same port on all nodes
   - Accessible via any node IP

3. **Port Mapping Flow**:
   ```
   localhost:8080 → Kind container:30080 → NodePort Service:30080 → Pod:8080
   ```

## Current Gateway Service Configuration

Let me check the current Gateway service:

<function_calls>
<invoke name="read_file">
<parameter name="target_file">/Users/jgil/go/src/github.com/jordigilh/kubernaut/test/e2e/gateway/gateway-deployment.yaml
