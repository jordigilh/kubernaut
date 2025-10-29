# HAProxy Configuration for Redis NodePort Access

**Date**: 2025-10-24
**Purpose**: Expose Redis NodePort (30379) through HAProxy for integration test access

---

## 🎯 **Problem Statement**

Integration tests running on Mac (`jgil-mac`) cannot access Redis NodePort directly because:
1. **Internal cluster IP** (`192.168.122.217:30379`) is not routable from outside `helios08`
2. **NodePort service** exists but is only accessible within the cluster network
3. **HAProxy** on `helios08` currently only exposes K8s API (6443), HTTPS (443), and HTTP (80)

**Solution**: Add Redis NodePort frontend/backend to HAProxy configuration

---

## 📋 **Proposed HAProxy Configuration Changes**

### **Current Configuration**
```haproxy
# /etc/haproxy/haproxy.cfg (existing)
global
    daemon

defaults
    mode tcp
    timeout connect 5000ms
    timeout client 50000ms
    timeout server 50000ms

frontend api_frontend
    bind 10.46.108.23:6443
    bind 192.168.122.100:6443
    default_backend api_backend

backend api_backend
    balance roundrobin
    option tcp-check
    server cluster-api-vip 192.168.122.251:6443 check

frontend https_frontend
    bind 10.46.108.23:443
    bind 192.168.122.101:443
    default_backend https_backend

backend https_backend
    balance roundrobin
    option tcp-check
    server cluster-https-vip 192.168.122.251:443 check

frontend http_frontend
    bind 10.46.108.23:80
    bind 192.168.122.101:80
    default_backend http_backend

backend http_backend
    balance roundrobin
    option tcp-check
    server cluster-http-vip 192.168.122.251:80 check
```

---

### **Proposed Addition** ⭐

Add this section to `/etc/haproxy/haproxy.cfg`:

```haproxy
# Redis Gateway NodePort (for integration tests)
# Exposes Redis HA cluster NodePort service to external clients
frontend redis_gateway_frontend
    bind 10.46.108.23:30379
    bind 192.168.122.100:30379
    default_backend redis_gateway_backend

backend redis_gateway_backend
    balance roundrobin
    option tcp-check
    # NodePort service routes to any node, then kube-proxy distributes
    # Using cluster VIP as entry point (kube-proxy will handle distribution)
    server redis-gateway-nodeport 192.168.122.217:30379 check
```

**Explanation**:
- **Frontend binds**:
  - `10.46.108.23:30379` - External IP (accessible from Mac)
  - `192.168.122.100:30379` - Internal IP (for consistency)
- **Backend**:
  - Points to any cluster node's NodePort (30379)
  - `kube-proxy` on the node will distribute to Redis pods
  - Health check ensures node is reachable

---

## 🔧 **Implementation Steps**

### **Step 1: Backup Current Configuration**
```bash
ssh helios08 'cp /etc/haproxy/haproxy.cfg /etc/haproxy/haproxy.cfg.backup-$(date +%Y%m%d-%H%M%S)'
```

### **Step 2: Add Redis Frontend/Backend**
```bash
ssh helios08 'cat >> /etc/haproxy/haproxy.cfg << EOF

# Redis Gateway NodePort (for integration tests)
# Exposes Redis HA cluster NodePort service to external clients
frontend redis_gateway_frontend
    bind 10.46.108.23:30379
    bind 192.168.122.100:30379
    default_backend redis_gateway_backend

backend redis_gateway_backend
    balance roundrobin
    option tcp-check
    # NodePort service routes to any node, then kube-proxy distributes
    server redis-gateway-nodeport 192.168.122.217:30379 check
EOF
'
```

### **Step 3: Validate Configuration**
```bash
ssh helios08 'haproxy -c -f /etc/haproxy/haproxy.cfg'
```

**Expected Output**: `Configuration file is valid`

### **Step 4: Restart HAProxy**
```bash
ssh helios08 'systemctl restart haproxy'
```

### **Step 5: Verify HAProxy Status**
```bash
ssh helios08 'systemctl status haproxy'
```

### **Step 6: Test Redis Connectivity from Mac**
```bash
# From your Mac
redis-cli -h helios08.lab.eng.tlv2.redhat.com -p 30379 -n 2 PING
# Or using IP
redis-cli -h 10.46.108.23 -p 30379 -n 2 PING
```

**Expected Output**: `PONG`

---

## 🔄 **Rollback Plan**

If anything goes wrong:

```bash
# Restore backup
ssh helios08 'cp /etc/haproxy/haproxy.cfg.backup-* /etc/haproxy/haproxy.cfg'

# Restart HAProxy
ssh helios08 'systemctl restart haproxy'
```

---

## 📊 **Update Integration Test Configuration**

After HAProxy is configured, update test helpers:

```go
// test/integration/gateway/helpers.go
func SetupRedisTestClient(ctx context.Context) *RedisTestClient {
    // ... existing code ...
    
    // Try NodePort via HAProxy first (production-grade, stable connectivity)
    nodeHost := os.Getenv("REDIS_NODE_HOST")
    if nodeHost == "" {
        nodeHost = "helios08.lab.eng.tlv2.redhat.com" // HAProxy host
    }
    
    client := goredis.NewClient(&goredis.Options{
        Addr:         nodeHost + ":30379", // NodePort via HAProxy
        Password:     "",                   // No password for test Redis
        DB:           2,                    // Use DB 2 for integration tests
        PoolSize:     20,                   // Connection pool for stability
        MinIdleConns: 5,                    // Keep connections alive
        MaxRetries:   3,                    // Retry on transient failures
        DialTimeout:  5 * time.Second,
        ReadTimeout:  3 * time.Second,
        WriteTimeout: 3 * time.Second,
    })
    
    // ... rest of code ...
}
```

---

## ✅ **Verification Checklist**

- [ ] HAProxy configuration backed up
- [ ] Redis frontend/backend added to HAProxy config
- [ ] HAProxy configuration validated (`haproxy -c`)
- [ ] HAProxy restarted successfully
- [ ] Redis connectivity tested from Mac (`redis-cli PING`)
- [ ] Integration test helpers updated to use `helios08.lab.eng.tlv2.redhat.com:30379`
- [ ] Integration tests run successfully

---

## 🎯 **Expected Benefits**

1. ✅ **Stable connectivity**: No more port-forward connection resets
2. ✅ **Production-grade**: HAProxy is battle-tested for high availability
3. ✅ **Consistent access**: Same endpoint for all developers and CI/CD
4. ✅ **No local setup**: No need to manage `kubectl port-forward` processes
5. ✅ **Scalable**: HAProxy can handle 1000+ concurrent connections

---

## ⚠️ **Security Considerations**

**Current Risk**: Redis exposed on external IP without authentication

**Mitigation Options**:

### **Option A: Firewall Rule (Recommended)**
```bash
# Allow only your Mac's IP
ssh helios08 'firewall-cmd --permanent --add-rich-rule="rule family=ipv4 source address=<YOUR_MAC_IP> port port=30379 protocol=tcp accept"'
ssh helios08 'firewall-cmd --reload'
```

### **Option B: Redis Password (Alternative)**
Update Redis deployment to require password, then update test helpers with password.

### **Option C: VPN/Bastion (Long-term)**
Route all test traffic through VPN or bastion host.

**Recommendation**: Start with **Option A** (firewall rule) for immediate security, plan **Option C** for production.

---

## 📝 **Summary**

**Changes Required**:
1. Add Redis frontend/backend to HAProxy config on `helios08`
2. Update test helpers to use `helios08.lab.eng.tlv2.redhat.com:30379`
3. Add firewall rule to restrict access (security)

**Estimated Time**: 15 minutes

**Confidence**: **95%** - HAProxy is proven technology, this is a standard TCP proxy configuration


