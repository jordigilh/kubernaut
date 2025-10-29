# Redis Connectivity Options for Integration Tests

**Date**: 2025-10-24
**Context**: Replace unstable `kubectl port-forward` with production-grade connectivity

---

## 🔍 **Current Problem: Port-Forward Instability**

### **Issues Observed**
```
E1024 12:35:10 portforward.go:398] "Unhandled Error" err="error copying from local connection to remote stream: writeto tcp6 [::1]:6379->[::1]:62947: read tcp6 [::1]:6379->[::1]:62947: read: connection reset by peer"
```

**Why Port-Forward Fails**:
1. **Short-lived connections**: Not designed for long-running tests (10-30 minutes)
2. **Connection pool exhaustion**: 33 tests × 10-50 requests each = 330-1650 connections
3. **Network instability**: TCP tunneling through kubectl API server (3 hops)
4. **No reconnection logic**: Single connection failure kills entire test

**Impact**: Tests timeout due to transient connection failures

---

## 💡 **Option A: OpenShift Route (TCP Passthrough)** ⭐ **RECOMMENDED**

### **Architecture**
```
Integration Tests (localhost)
    ↓ TCP
OpenShift Route (redis-gateway-ha-route.apps.cluster.example.com:6379)
    ↓ TCP Passthrough
Service (redis-gateway-ha.kubernaut-system:6379)
    ↓ Load Balanced
Redis Pods (redis-gateway-0, redis-gateway-1, redis-gateway-2)
```

### **Implementation**

#### **1. Create Route with TCP Passthrough**
```yaml
# deploy/redis-ha/redis-gateway-route.yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: redis-gateway-ha-route
  namespace: kubernaut-system
  labels:
    app: redis-gateway
    service: gateway
spec:
  # TCP passthrough (no TLS termination at router)
  port:
    targetPort: redis
  tls:
    termination: passthrough
    insecureEdgeTerminationPolicy: None
  to:
    kind: Service
    name: redis-gateway-ha
    weight: 100
  wildcardPolicy: None
```

#### **2. Update Integration Test Configuration**
```go
// test/integration/gateway/helpers.go
func SetupRedisTestClient(ctx context.Context) *RedisTestClient {
    // Try OpenShift Route first (production-grade connectivity)
    routeHost := os.Getenv("REDIS_ROUTE_HOST") // e.g., redis-gateway-ha-route-kubernaut-system.apps.cluster.example.com
    if routeHost != "" {
        client := goredis.NewClient(&goredis.Options{
            Addr:     routeHost + ":6379",
            Password: os.Getenv("REDIS_PASSWORD"),
            DB:       2,
            // Connection pool for stability
            PoolSize:     20,
            MinIdleConns: 5,
            MaxRetries:   3,
            DialTimeout:  5 * time.Second,
            ReadTimeout:  3 * time.Second,
            WriteTimeout: 3 * time.Second,
        })

        if err := client.Ping(ctx).Err(); err == nil {
            return &RedisTestClient{Client: client}
        }
    }

    // Fallback to port-forward (development)
    // ... existing code ...
}
```

#### **3. Update Test Script**
```bash
# test/integration/gateway/run-tests.sh
#!/bin/bash

# Get Redis Route hostname
REDIS_ROUTE=$(kubectl get route redis-gateway-ha-route -n kubernaut-system -o jsonpath='{.spec.host}' 2>/dev/null)

if [[ -n "${REDIS_ROUTE}" ]]; then
    echo "✅ Using OpenShift Route: ${REDIS_ROUTE}:6379"
    export REDIS_ROUTE_HOST="${REDIS_ROUTE}"

    # No port-forward needed!
    go test -v ./test/integration/gateway -run "TestGatewayIntegration" -timeout 30m
else
    echo "⚠️  Route not found, falling back to port-forward"
    # ... existing port-forward logic ...
fi
```

### **Pros** ✅
- ✅ **Production-grade**: Uses OpenShift Router (HAProxy) - same as production traffic
- ✅ **Stable**: No connection resets, automatic reconnection
- ✅ **Load balanced**: Router distributes connections across Redis replicas
- ✅ **Scalable**: Handles 1000+ concurrent connections
- ✅ **No local setup**: No port-forward process to manage
- ✅ **CI/CD friendly**: Works in automated pipelines without `kubectl` access

### **Cons** ⚠️
- ⚠️ **Requires cluster admin**: Creating Routes needs permissions
- ⚠️ **External exposure**: Redis accessible from outside cluster (mitigated by password + network policies)
- ⚠️ **OpenShift specific**: Not portable to vanilla Kubernetes (would need NodePort or LoadBalancer)

### **Security Mitigation**
```yaml
# deploy/redis-ha/redis-gateway-networkpolicy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: redis-gateway-external-access
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: redis-gateway
  policyTypes:
  - Ingress
  ingress:
  # Allow from OpenShift Router only
  - from:
    - namespaceSelector:
        matchLabels:
          network.openshift.io/policy-group: ingress
    ports:
    - protocol: TCP
      port: 6379
  # Allow from within namespace (Gateway service)
  - from:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 6379
```

### **Confidence**: **95%** - OpenShift Route is production-grade, stable, and scalable

---

## 💡 **Option B: NodePort Service** (Kubernetes-native)

### **Architecture**
```
Integration Tests (localhost)
    ↓ TCP
Node IP:30379 (NodePort)
    ↓ kube-proxy
Service (redis-gateway-ha.kubernaut-system:6379)
    ↓ Load Balanced
Redis Pods
```

### **Implementation**
```yaml
# deploy/redis-ha/redis-gateway-nodeport.yaml
apiVersion: v1
kind: Service
metadata:
  name: redis-gateway-ha-nodeport
  namespace: kubernaut-system
spec:
  type: NodePort
  selector:
    app: redis-gateway
  ports:
  - name: redis
    port: 6379
    targetPort: 6379
    nodePort: 30379  # Static port (30000-32767 range)
```

### **Pros** ✅
- ✅ **Kubernetes-native**: Works on any K8s cluster (not just OpenShift)
- ✅ **Stable**: No port-forward connection resets
- ✅ **Simple**: Single Service manifest

### **Cons** ⚠️
- ⚠️ **Port range limited**: 30000-32767 (may conflict with other services)
- ⚠️ **Node IP required**: Tests need to know node IP (not always stable)
- ⚠️ **Less secure**: Exposes Redis on all cluster nodes
- ⚠️ **Not load balanced**: Connections go to single node, then kube-proxy distributes

### **Confidence**: **80%** - Works, but less elegant than Route

---

## 💡 **Option C: LoadBalancer Service** (Cloud-native)

### **Architecture**
```
Integration Tests (localhost)
    ↓ TCP
Cloud Load Balancer (external IP)
    ↓ TCP
Service (redis-gateway-ha.kubernaut-system:6379)
    ↓ Load Balanced
Redis Pods
```

### **Implementation**
```yaml
# deploy/redis-ha/redis-gateway-loadbalancer.yaml
apiVersion: v1
kind: Service
metadata:
  name: redis-gateway-ha-lb
  namespace: kubernaut-system
spec:
  type: LoadBalancer
  selector:
    app: redis-gateway
  ports:
  - name: redis
    port: 6379
    targetPort: 6379
```

### **Pros** ✅
- ✅ **Cloud-native**: Best option for AWS/GCP/Azure
- ✅ **Stable external IP**: Persistent across cluster changes
- ✅ **Production-grade**: Cloud provider's load balancer

### **Cons** ⚠️
- ⚠️ **Cloud-only**: Requires cloud provider (doesn't work on bare-metal OpenShift)
- ⚠️ **Cost**: Cloud load balancers cost $10-30/month
- ⚠️ **Slow provisioning**: Takes 1-3 minutes to provision

### **Confidence**: **70%** - Great for cloud, not applicable for on-prem OpenShift

---

## 💡 **Option D: Keep Port-Forward + Add Reconnection Logic**

### **Implementation**
```go
// test/integration/gateway/helpers.go
type ResilientRedisClient struct {
    *goredis.Client
    portForwardCmd *exec.Cmd
    mu             sync.Mutex
}

func (r *ResilientRedisClient) Ping(ctx context.Context) error {
    err := r.Client.Ping(ctx).Err()
    if err != nil {
        // Reconnect on failure
        r.mu.Lock()
        defer r.mu.Unlock()

        // Kill old port-forward
        if r.portForwardCmd != nil {
            r.portForwardCmd.Process.Kill()
        }

        // Start new port-forward
        r.portForwardCmd = exec.Command("kubectl", "port-forward", "-n", "kubernaut-system", "redis-gateway-0", "6379:6379")
        r.portForwardCmd.Start()
        time.Sleep(2 * time.Second)

        // Reconnect Redis client
        r.Client = goredis.NewClient(&goredis.Options{Addr: "localhost:6379", DB: 2})
        return r.Client.Ping(ctx).Err()
    }
    return nil
}
```

### **Pros** ✅
- ✅ **No cluster changes**: Works with existing setup
- ✅ **Automatic recovery**: Reconnects on failure

### **Cons** ⚠️
- ⚠️ **Complex**: Adds significant complexity to test code
- ⚠️ **Still unstable**: Port-forward can still fail mid-test
- ⚠️ **Race conditions**: Multiple tests reconnecting simultaneously

### **Confidence**: **60%** - Band-aid solution, not addressing root cause

---

## 🎯 **Recommendation: Option A (OpenShift Route)** ⭐

### **Why Route is Best**
1. **Production-grade**: Uses same infrastructure as production traffic
2. **Stable**: No connection resets, automatic reconnection
3. **Simple**: Single Route manifest + environment variable
4. **Secure**: NetworkPolicy restricts access to Router + Gateway pods
5. **CI/CD friendly**: No local port-forward process to manage

### **Implementation Steps**
1. ✅ Create `deploy/redis-ha/redis-gateway-route.yaml`
2. ✅ Create `deploy/redis-ha/redis-gateway-networkpolicy.yaml`
3. ✅ Update `test/integration/gateway/helpers.go` to use Route
4. ✅ Update `test/integration/gateway/run-tests.sh` to detect Route
5. ✅ Document in `DD-INFRASTRUCTURE-001`

### **Estimated Time**: **30 minutes**

### **Confidence**: **95%** - This will eliminate port-forward instability

---

## 📊 **Comparison Matrix**

| Feature | Port-Forward | Route (A) | NodePort (B) | LoadBalancer (C) |
|---|---|---|---|---|
| **Stability** | ❌ Poor | ✅ Excellent | ✅ Good | ✅ Excellent |
| **Performance** | ⚠️ Medium | ✅ High | ✅ High | ✅ High |
| **Setup Complexity** | ✅ Simple | ✅ Simple | ✅ Simple | ⚠️ Medium |
| **Security** | ✅ Secure | ✅ Secure (with NetworkPolicy) | ⚠️ Exposed | ⚠️ Exposed |
| **OpenShift** | ✅ Yes | ✅ Yes | ✅ Yes | ❌ Cloud only |
| **CI/CD Friendly** | ❌ No | ✅ Yes | ✅ Yes | ✅ Yes |
| **Cost** | ✅ Free | ✅ Free | ✅ Free | ❌ $10-30/month |

**Winner**: **Route (Option A)** ⭐

---

## ✅ **Next Steps**

1. **Create Route manifest** (5 minutes)
2. **Create NetworkPolicy** (5 minutes)
3. **Update test helpers** (10 minutes)
4. **Update test script** (5 minutes)
5. **Test connectivity** (5 minutes)
6. **Re-run integration tests** (20-30 minutes)

**Total Time**: **50-60 minutes**

**Expected Outcome**: ✅ Stable Redis connectivity, no more connection resets, tests complete successfully


