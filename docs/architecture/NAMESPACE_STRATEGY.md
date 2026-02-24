# Kubernaut Namespace Strategy

**Date**: October 6, 2025
**Status**: ✅ **APPROVED**
**Purpose**: Document dual-namespace architecture for stateless services and CRD controllers

---

## 📋 Overview

Kubernaut uses a **dual-namespace architecture** to separate concerns between HTTP API services and core Kubernetes controllers.

---

## 🏗️ Namespace Allocation

### **1. Stateless Services Namespace: `kubernaut-system`**

**Services**:
1. Gateway Service (Port 8080)
2. Context API (Port 8082)
3. Data Storage (Port 8080)
4. HolmesGPT API (Port 8080)
5. Dynamic Toolset (Port 8083)
6. Effectiveness Monitor (Port 8080)
7. Notification Service (Port 8088)

**Characteristics**:
- HTTP REST APIs
- Stateless microservices
- Inter-service communication via HTTP
- External client access
- Service-to-service authentication via TokenReviewer

**Port Strategy**:
- REST API + Health: Port 8080 (or service-specific port)
- Metrics: Port 9090 (with TokenReviewer authentication)

**Rationale**:
- Service-oriented namespace for HTTP API services
- Clear separation from platform infrastructure
- Easier to manage service networking policies
- Aligns with "alerts-slm" project context

---

### **2. CRD Controllers Namespace: `kubernaut-system`**

**Controllers**:
1. RemediationProcessor Controller
2. AIAnalysis Controller
3. WorkflowExecution Controller
4. ~~KubernetesExecutor Controller~~ (DEPRECATED - ADR-025)
5. RemediationOrchestrator Controller

**Characteristics**:
- Kubernetes controllers
- CRD reconciliation loops
- No external HTTP APIs (except metrics/health)
- Watch Kubernetes resources
- Inter-controller coordination via CRD status updates

**Port Strategy**:
- Health Probes: Port 8080 (follows kube-apiserver pattern)
- Metrics: Port 9090 (with TokenReviewer authentication)

**Rationale**:
- Platform namespace for core Kubernetes controllers
- Follows `kube-system` naming pattern
- Clear separation from user-facing services
- Dedicated namespace for Kubernetes-native infrastructure

---

## 🔄 Cross-Namespace Communication

### **Stateless → CRD Controllers**
- Gateway Service creates `RemediationRequest` CRD in `kubernaut-system`
- Stateless services query CRD status via Kubernetes API

### **CRD Controllers → Stateless**
- Controllers call stateless HTTP APIs:
  - Context API for historical intelligence
  - Data Storage for audit trail persistence
  - HolmesGPT API for AI analysis
  - Notification Service for escalation alerts

### **Authentication**
All cross-namespace communication uses **Kubernetes ServiceAccount tokens** validated via TokenReviewer API.

---

## 📦 Deployment Configuration

### **Namespace Creation**
```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-system
  labels:
    app.kubernetes.io/name: kubernaut
    app.kubernetes.io/component: stateless-services
---
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-system
  labels:
    app.kubernetes.io/name: kubernaut
    app.kubernetes.io/component: crd-controllers
```

### **ServiceAccount Strategy**

#### Stateless Services (in `kubernaut-system`)
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: gateway-service-sa
  namespace: kubernaut-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: context-api-sa
  namespace: kubernaut-system
# ... (one per stateless service)
```

#### CRD Controllers (in `kubernaut-system`)
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: remediation-processor-sa
  namespace: kubernaut-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ai-analysis-sa
  namespace: kubernaut-system
# ... (one per controller)
```

---

## 🔐 RBAC Strategy

### **Stateless Services RBAC**
- Read-only access to CRDs (watch RemediationRequest status)
- Read access to Kubernetes resources (for context enrichment)
- TokenReviewer access for service-to-service authentication

### **CRD Controllers RBAC**
- Full CRUD access to owned CRDs
- Watch access to upstream CRDs
- Update status of downstream CRDs
- TokenReviewer access for metrics authentication

---

## 🌐 NetworkPolicy Strategy

### **Stateless Services**
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: stateless-services-policy
  namespace: kubernaut-system
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          app.kubernetes.io/name: kubernaut
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          app.kubernetes.io/name: kubernaut
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system  # For Kubernetes API access
```

### **CRD Controllers**
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: crd-controllers-policy
  namespace: kubernaut-system
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: kube-system  # For metrics scraping
  egress:
  - to:
    - namespaceSelector:
        matchLabels:
          app.kubernetes.io/name: kubernaut
          app.kubernetes.io/component: stateless-services
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system  # For Kubernetes API access
```

---

## 📊 Service Discovery

### **DNS Resolution**

#### Stateless Services (in `kubernaut-system`)
```
gateway-service.kubernaut-system.svc.cluster.local:8080
context-api.kubernaut-system.svc.cluster.local:8082
data-storage.kubernaut-system.svc.cluster.local:8080
holmesgpt-api.kubernaut-system.svc.cluster.local:8080
dynamic-toolset.kubernaut-system.svc.cluster.local:8083
effectiveness-monitor.kubernaut-system.svc.cluster.local:8080
notification-service.kubernaut-system.svc.cluster.local:8088
```

#### CRD Controllers (in `kubernaut-system`)
Controllers don't expose HTTP services, only metrics endpoints:
```
remediation-processor-controller.kubernaut-system.svc.cluster.local:9090/metrics
ai-analysis-controller.kubernaut-system.svc.cluster.local:9090/metrics
workflow-execution-controller.kubernaut-system.svc.cluster.local:9090/metrics
kubernetes-executor-controller.kubernaut-system.svc.cluster.local:9090/metrics  # DEPRECATED - ADR-025
remediation-orchestrator-controller.kubernaut-system.svc.cluster.local:9090/metrics
```

---

## 🎯 Benefits of Dual-Namespace Architecture

### **1. Clear Separation of Concerns**
- Stateless services handle HTTP requests
- CRD controllers handle Kubernetes reconciliation
- Easy to understand system boundaries

### **2. Security Isolation**
- Different RBAC policies per namespace
- Fine-grained NetworkPolicy control
- Blast radius containment

### **3. Operational Clarity**
- `kubectl get pods -n kubernaut-system` → See all stateless services
- `kubectl get pods -n kubernaut-system` → See all controllers
- Easy to monitor, upgrade, or debug specific components

### **4. Resource Management**
- Independent ResourceQuotas per namespace
- Separate LimitRanges for services vs controllers
- Easier capacity planning

### **5. Deployment Flexibility**
- Can deploy stateless services independently
- Can upgrade controllers without affecting stateless services
- Supports staged rollouts and blue-green deployments

---

## 🚀 Migration Strategy (If Unified Namespace Desired)

**⚠️ Not Recommended**: Dual-namespace architecture is preferred.

If unification is required:
1. Choose target namespace: `kubernaut-system` (platform-centric) or `kubernaut` (service-centric)
2. Update all ServiceAccount references
3. Update all Service DNS references
4. Update all NetworkPolicy rules
5. Update deployment manifests
6. Test cross-namespace communication still works
7. Update all documentation references

**Estimated Effort**: 2-3 days
**Risk**: HIGH (breaking changes to service discovery)

---

## 📋 Deployment Checklist

### **Prerequisites**
- [ ] Create `kubernaut-system` namespace
- [ ] Create `kubernaut-system` namespace
- [ ] Create ServiceAccounts for all 12 services
- [ ] Create RBAC roles and bindings
- [ ] Create NetworkPolicies (if required)

### **Stateless Services Deployment**
- [ ] Deploy Gateway Service in `kubernaut-system`
- [ ] Deploy Context API in `kubernaut-system`
- [ ] Deploy Data Storage in `kubernaut-system`
- [ ] Deploy HolmesGPT API in `kubernaut-system`
- [ ] Deploy Dynamic Toolset in `kubernaut-system`
- [ ] Deploy Effectiveness Monitor in `kubernaut-system`
- [ ] Deploy Notification Service in `kubernaut-system`

### **CRD Controllers Deployment**
- [ ] Deploy RemediationProcessor in `kubernaut-system`
- [ ] Deploy AIAnalysis in `kubernaut-system`
- [ ] Deploy WorkflowExecution in `kubernaut-system`
- [ ] ~~Deploy KubernetesExecutor~~ (DEPRECATED - ADR-025) in `kubernaut-system`
- [ ] Deploy RemediationOrchestrator in `kubernaut-system`

### **Validation**
- [ ] All pods running in correct namespaces
- [ ] Cross-namespace service discovery working
- [ ] TokenReviewer authentication working
- [ ] CRD creation/reconciliation working
- [ ] Metrics endpoints accessible with authentication

---

## 🔗 References

- [Kubernetes Namespace Best Practices](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/)
- [Service-to-Service Authentication](https://kubernetes.io/docs/reference/access-authn-authz/authentication/)
- [NetworkPolicy Documentation](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [RBAC Authorization](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)

---

**Document Maintainer**: Kubernaut Documentation Team
**Last Updated**: October 6, 2025
**Status**: ✅ **APPROVED - DUAL-NAMESPACE ARCHITECTURE**
**Confidence**: 95% (Well-established pattern, clear benefits)
