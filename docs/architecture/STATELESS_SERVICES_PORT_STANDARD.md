# Stateless Services - Port Configuration Standard

**Version**: v1.0
**Last Updated**: October 6, 2025
**Status**: ✅ **STANDARDIZED**
**Scope**: All 6 Stateless HTTP Services

---

## 📋 **Port Standard**

### **Standardized Configuration**

All Kubernaut stateless HTTP services **MUST** use the following port configuration:

| Purpose | Port | Endpoints | Authentication |
|---------|------|-----------|----------------|
| **REST API + Health** | **8080** | `/api/v1/*`, `/healthz`, `/readyz` | TokenReviewer (API), None (health) |
| **Metrics** | **9090** | `/metrics` | TokenReviewer |

---

## ✅ **Compliance Matrix**

### **All Services Compliant**

| Service | REST/Health Port | Metrics Port | Status |
|---------|-----------------|--------------|--------|
| **Gateway** | 8080 | 9090 | ✅ Compliant |
| **Context API** | 8080 | 9090 | ✅ Compliant |
| **Data Storage** | 8080 | 9090 | ✅ Compliant |
| **HolmesGPT API** | 8080 | 9090 | ✅ Compliant |
| **Notification** | 8080 | 9090 | ✅ Compliant |
| **Dynamic Toolset** | 8080 | 9090 | ✅ Compliant |

**Compliance Rate**: 6/6 (100%)

---

## 🎯 **Rationale**

### **1. Single Port for REST API + Health (8080)**

**Pattern**: Follows **kube-apiserver pattern**

**Rationale**:
- ✅ **Simplicity**: Single port for all HTTP traffic
- ✅ **Standard**: Kubernetes API server uses 6443 for both API and health
- ✅ **Common**: Widely used pattern in Kubernetes ecosystem
- ✅ **Authentication**: Health endpoints (`/healthz`, `/readyz`) don't require auth
- ✅ **API Endpoints**: All API endpoints (`/api/v1/*`) require TokenReviewer auth

**Example**:
```yaml
# Service configuration
ports:
- port: 8080
  targetPort: 8080
  name: http
  # Serves both:
  # - /api/v1/* (with auth)
  # - /healthz, /readyz (no auth)
```

---

### **2. Separate Port for Metrics (9090)**

**Pattern**: Follows **Prometheus ecosystem standard**

**Rationale**:
- ✅ **Isolation**: Metrics on separate port from API traffic
- ✅ **Standard**: Prometheus itself uses 9090
- ✅ **Security**: Metrics require authentication (TokenReviewer)
- ✅ **Monitoring**: Prometheus ServiceMonitor targets port 9090
- ✅ **No Conflict**: No overlap with API port

**Example**:
```yaml
ports:
- port: 9090
  targetPort: 9090
  name: metrics
  # Serves /metrics with TokenReviewer auth
```

---

## 📊 **Service Configuration Examples**

### **Gateway Service**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: gateway-service
spec:
  selector:
    app: gateway
  ports:
  - port: 8080
    targetPort: 8080
    name: http
    protocol: TCP
  - port: 9090
    targetPort: 9090
    name: metrics
    protocol: TCP
  type: ClusterIP
```

---

### **Context API Service**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: context-api
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: context-api
spec:
  selector:
    app: context-api
  ports:
  - port: 8080
    targetPort: 8080
    name: http
    protocol: TCP
  - port: 9090
    targetPort: 9090
    name: metrics
    protocol: TCP
  type: ClusterIP
```

---

### **Data Storage Service**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: data-storage
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: data-storage
spec:
  selector:
    app: data-storage
  ports:
  - port: 8080
    targetPort: 8080
    name: http
    protocol: TCP
  - port: 9090
    targetPort: 9090
    name: metrics
    protocol: TCP
  type: ClusterIP
```

---

### **HolmesGPT API Service**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: holmesgpt-api
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: holmesgpt-api
spec:
  selector:
    app: holmesgpt-api
  ports:
  - port: 8080
    targetPort: 8080
    name: http
    protocol: TCP
  - port: 9090
    targetPort: 9090
    name: metrics
    protocol: TCP
  type: ClusterIP
```

---

### **Notification Service**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: notification-service
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: notification-service
spec:
  selector:
    app: notification-service
  ports:
  - port: 8080
    targetPort: 8080
    name: http
    protocol: TCP
  - port: 9090
    targetPort: 9090
    name: metrics
    protocol: TCP
  type: ClusterIP
```

---

### **Dynamic Toolset Service**

```yaml
apiVersion: v1
kind: Service
metadata:
  name: dynamic-toolset
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: dynamic-toolset
spec:
  selector:
    app: dynamic-toolset
  ports:
  - port: 8080
    targetPort: 8080
    name: http
    protocol: TCP
  - port: 9090
    targetPort: 9090
    name: metrics
    protocol: TCP
  type: ClusterIP
```

---

## 🔒 **Authentication Strategy**

### **Port 8080 (Mixed Authentication)**

**No Authentication** (Health Probes):
- `/healthz` - Liveness probe
- `/readyz` - Readiness probe

**TokenReviewer Authentication** (API Endpoints):
- `/api/v1/*` - All REST API endpoints

**Implementation Pattern**:
```go
// Middleware applies auth selectively
func (m *AuthMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
    // Skip auth for health endpoints
    if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
        next(w, r)
        return
    }

    // Require TokenReviewer for all other endpoints
    if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
        http.Error(w, "Not found", http.StatusNotFound)
        return
    }

    // Validate token
    if err := m.validateToken(r); err != nil {
        http.Error(w, "Unauthorized", http.StatusUnauthorized)
        return
    }

    next(w, r)
}
```

---

### **Port 9090 (Full Authentication)**

**TokenReviewer Authentication** (Required):
- `/metrics` - Prometheus scrape endpoint

**Implementation**: See [KUBERNETES_TOKENREVIEWER_AUTH.md](./KUBERNETES_TOKENREVIEWER_AUTH.md)

---

## 📊 **Prometheus ServiceMonitor Pattern**

All stateless services use identical ServiceMonitor configuration:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: {service}-monitor
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: {service}
    app.kubernetes.io/component: observability
    app.kubernetes.io/part-of: kubernaut
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: {service}
  endpoints:
  - port: metrics  # Port 9090
    path: /metrics
    interval: 30s
    scrapeTimeout: 10s
    scheme: http
    bearerTokenSecret:
      name: prometheus-{service}-token
      key: token
  namespaceSelector:
    matchNames:
    - kubernaut-system
```

**See**: [PROMETHEUS_SERVICEMONITOR_PATTERN.md](./PROMETHEUS_SERVICEMONITOR_PATTERN.md)

---

## ✅ **Deployment Checklist**

### **For Each Service**:

1. ✅ **Service YAML**: Ports 8080 (http) and 9090 (metrics)
2. ✅ **Deployment YAML**: Container ports 8080 and 9090
3. ✅ **Health Probes**: Liveness/Readiness on port 8080
4. ✅ **ServiceMonitor**: Scrape port 9090 with TokenReviewer
5. ✅ **ServiceAccount**: For TokenReviewer authentication
6. ✅ **Documentation**: Port configuration in service overview

---

## 🎯 **Benefits of Standardization**

### **1. Operational Simplicity**
- ✅ Predictable port configuration across all services
- ✅ Single pattern for Prometheus scraping
- ✅ Consistent health probe configuration

### **2. Security Consistency**
- ✅ All metrics require authentication
- ✅ Health probes don't require authentication (standard)
- ✅ API endpoints require TokenReviewer

### **3. Service Discovery**
- ✅ Services discoverable by port name (`http`, `metrics`)
- ✅ ServiceMonitor can use label selectors
- ✅ No port conflicts

### **4. Documentation Clarity**
- ✅ Single port standard to document
- ✅ No exceptions or special cases
- ✅ Clear for new developers

---

## 📚 **Related Documentation**

- [KUBERNETES_TOKENREVIEWER_AUTH.md](./KUBERNETES_TOKENREVIEWER_AUTH.md) - Authentication implementation
- [PROMETHEUS_SERVICEMONITOR_PATTERN.md](./PROMETHEUS_SERVICEMONITOR_PATTERN.md) - ServiceMonitor configuration
- [SERVICE_DEPENDENCY_MAP.md](./SERVICE_DEPENDENCY_MAP.md) - Service interactions

---

**Document Status**: ✅ Complete
**Compliance**: 6/6 services (100%)
**Last Updated**: October 6, 2025
**Version**: 1.0
