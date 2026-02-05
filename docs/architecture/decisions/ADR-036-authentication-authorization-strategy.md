# ADR-036: Authentication and Authorization Strategy for All Services

## Status
**✅ APPROVED** (with Gateway exception - see below)
**Version**: 1.1
**Decision Date**: November 9, 2025
**Last Reviewed**: January 29, 2026
**Confidence**: 95%
**Authority Level**: **ARCHITECTURAL** - Applies to all services

---

## ⚠️ **IMPORTANT UPDATE (January 29, 2026)**

**Gateway Service Exception**: Gateway now **requires SAR authentication** per [DD-AUTH-014 V2.0](./DD-AUTH-014-middleware-based-sar-authentication.md).

**Rationale**:
- Gateway is **external-facing** (AlertManager, K8s Event forwarders) - different threat model
- **Zero-trust architecture**: Network Policies alone insufficient for external-facing services
- **SOC2 compliance**: Operator attribution required (CC8.1)
- **Proven pattern**: DataStorage/HAPI SAR implementation successful

**Updated Service Status**:
- **Gateway**: ✅ **SAR Auth Required** (DD-AUTH-014 V2.0) - Exception to ADR-036
- **DataStorage**: ✅ **SAR Auth Required** (DD-AUTH-014 V1.0)
- **HolmesGPT API**: ✅ **SAR Auth Required** (DD-AUTH-014 V1.0)
- **Others**: Network Policies + TLS (per this ADR)

**See**: BR-GATEWAY-182, BR-GATEWAY-183 for Gateway auth requirements

---

---

## Context

Kubernaut consists of multiple services (Gateway, Context API, Data Storage, Dynamic Toolset, HolmesGPT API, Notification Service, etc.) that communicate within a Kubernetes cluster. Initially, some services implemented OAuth2-based authentication (TokenReview) and authorization (SubjectAccessReview) middleware. However, this approach created several challenges:

1. **Testing Complexity**: Setting up ServiceAccounts, tokens, and RBAC for integration tests added significant complexity
2. **K8s API Throttling**: TokenReview and SubjectAccessReview calls on every request created K8s API load
3. **In-Cluster Use Case**: Services are designed for in-cluster communication, not external access
4. **Inconsistency**: Different services had different authentication approaches
5. **Maintenance Burden**: Authentication logic duplicated across multiple services
6. **Performance Impact**: K8s API calls on every request added latency

### Related Decisions
- **DD-GATEWAY-006**: Gateway Authentication Strategy (to be superseded by this ADR)
- **DD-HOLMESGPT-011**: HolmesGPT Authentication Strategy (to be superseded by this ADR)
- **ADR-032**: Data Access Layer Isolation (references network policy approach)
- **ADR-014**: Notification Service External Auth (specific to external integrations)

---

## Decision

**Remove OAuth2 authentication and authorization middleware from all Kubernaut services** and adopt a **layered security approach** that leverages Kubernetes-native mechanisms and deployment-time flexibility.

### Core Principles

1. **Network-Level Security First**: Use Kubernetes Network Policies and TLS for traffic encryption and isolation
2. **Deployment-Time Flexibility**: Support sidecar containers for custom protocol-based authentication and authorization
3. **Separation of Concerns**: Keep services focused on business logic, not authentication
4. **Defense-in-Depth**: Combine multiple security layers for comprehensive protection
5. **Consistency**: Apply the same security model across all services

### Security Architecture

#### Layer 1: Network Isolation (MANDATORY for all services)
- **Kubernetes Network Policies**: Restrict traffic to authorized sources only
- **Namespace Isolation**: Deploy services in dedicated namespaces with strict ingress/egress rules
- **Service-Level TLS**: Encrypt traffic between services using Kubernetes Service TLS

#### Layer 2: Transport Security (MANDATORY for all services)
- **TLS Encryption**: All inter-service traffic MUST use TLS
- **Certificate Management**: Use cert-manager or similar for automated certificate rotation
- **Cipher Suite Configuration**: Enforce strong TLS 1.3 cipher suites

#### Layer 3: Application-Level Authentication (OPTIONAL, Deployment-Specific)
- **Sidecar Pattern**: Deploy authentication/authorization as a sidecar container when needed
- **Protocol Flexibility**: Support mTLS, OAuth2, API keys, or custom protocols via sidecar
- **Service Mesh Integration**: Leverage Istio, Linkerd, or similar for advanced auth scenarios

---

## Implementation Guidelines

### For All Services

#### 1. Remove Authentication Middleware
- Delete `TokenReviewAuth` middleware (if exists)
- Delete `SubjectAccessReviewAuthz` middleware (if exists)
- Remove `DisableAuth` configuration options
- Remove `k8sClientset` parameter from server constructors (if only used for auth)
- Remove authentication-related metrics

#### 2. Keep Security Middleware
Services should retain:
- **Rate Limiting**: Protect against DoS attacks
- **Payload Size Limits**: Prevent large request attacks
- **Input Validation**: RFC 7807 error responses for invalid inputs
- **Log Sanitization**: Redact sensitive data from logs
- **Security Headers**: Prevent common web vulnerabilities

#### 3. Document Security Requirements
Each service must document:
- Required Network Policies
- TLS configuration
- Optional sidecar authentication patterns

---

## Network Policy Examples

### Example 1: Gateway Service
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-ingress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Ingress
  ingress:
  # Allow Prometheus
  - from:
    - namespaceSelector:
        matchLabels:
          name: monitoring
      podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8080
  # Deny all other traffic (implicit)
```

### Example 2: Data Storage Service
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: data-storage-allow-internal
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: data-storage
  policyTypes:
  - Ingress
  ingress:
  # Allow Context API
  - from:
    - namespaceSelector:
        matchLabels:
          name: kubernaut-system
      podSelector:
        matchLabels:
          app: context-api
    ports:
    - protocol: TCP
      port: 8080
  # Allow CRD Controllers
  - from:
    - namespaceSelector:
        matchLabels:
          name: kubernaut-system
      podSelector:
        matchLabels:
          component: crd-controller
    ports:
    - protocol: TCP
      port: 8080
```

### Example 3: Dynamic Toolset Service
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: dynamic-toolset-ingress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: dynamic-toolset
  policyTypes:
  - Ingress
  ingress:
  # Allow HolmesGPT API
  - from:
    - namespaceSelector:
        matchLabels:
          name: kubernaut-system
      podSelector:
        matchLabels:
          app: holmesgpt-api
    ports:
    - protocol: TCP
      port: 8080
```

---

## Sidecar Authentication Pattern (Optional)

### Example: Envoy Sidecar with OAuth2

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      # Main application container
      - name: gateway
        image: kubernaut/gateway:v1.0
        ports:
        - containerPort: 8080
          name: http
      
      # Envoy sidecar for authentication
      - name: envoy-auth
        image: envoyproxy/envoy:v1.28
        ports:
        - containerPort: 9000
          name: envoy-admin
        volumeMounts:
        - name: envoy-config
          mountPath: /etc/envoy
      
      volumes:
      - name: envoy-config
        configMap:
          name: envoy-auth-config
---
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  ports:
  - port: 80
    targetPort: 8080  # Routes to Envoy, which proxies to gateway:8080
  selector:
    app: gateway
```

---

## Consequences

### Positive
1. **Simplified Service Code**: Removes authentication complexity from all services
2. **Reduced K8s API Load**: No TokenReview/SubjectAccessReview calls
3. **Deployment Flexibility**: Different deployments can choose appropriate authentication mechanisms
4. **Easier Testing**: Integration tests don't require complex ServiceAccount setup
5. **Better Separation of Concerns**: Authentication is a deployment concern, not a service concern
6. **Performance**: Lower latency without K8s API calls for every request
7. **Consistency**: All services follow the same security model
8. **Maintainability**: Authentication logic centralized in deployment configuration

### Negative
1. **Deployment Responsibility**: Operators must configure Network Policies and TLS correctly
2. **Documentation Burden**: Need clear documentation for different deployment scenarios
3. **No Built-in Auth**: Services themselves don't enforce authentication (relies on network layer)
4. **Migration Effort**: Existing services with auth middleware need updates

### Neutral
1. **Security Model Shift**: From application-level to network-level security
2. **Sidecar Complexity**: Optional sidecar authentication adds deployment complexity (but only when needed)

---

## Migration Plan

### Phase 1: Service Updates (Immediate)
For each service with authentication middleware:

1. **Remove Middleware**:
   - Delete auth/authz middleware files
   - Remove `k8sClientset` from server constructors (if only used for auth)
   - Remove `DisableAuth` configuration options
   - Remove authentication metrics

2. **Update Tests**:
   - Remove ServiceAccount setup from integration tests
   - Remove token generation helpers
   - Simplify test infrastructure

3. **Update Documentation**:
   - Document required Network Policies
   - Add deployment examples
   - Update security documentation

### Phase 2: Deployment Configuration (Immediate)
1. Create Network Policies for all services
2. Configure TLS for inter-service communication
3. Document optional sidecar patterns

### Phase 3: Validation (Immediate)
1. Integration tests verify functionality without auth middleware
2. Security review confirms Network Policies are correct
3. Performance testing confirms latency improvements

---

## Services Affected

| Service | Status | Action Required | Notes |
|---------|--------|-----------------|-------|
| **Gateway** | ⚠️ **Exception** | **SAR Auth Required** (DD-AUTH-014 V2.0) | External-facing - requires app-level auth |
| **Data Storage** | ⚠️ **Exception** | **SAR Auth Complete** (DD-AUTH-014 V1.0) | Internal REST API - SAR for audit compliance |
| **HolmesGPT API** | ⚠️ **Exception** | **SAR Auth Complete** (DD-AUTH-014 V1.0) | Internal REST API - SAR for audit compliance |
| **Context API** | ✅ Follows ADR | Network Policies + TLS | - |
| **Notification Service** | ✅ Follows ADR | CRD controller, K8s RBAC only | - |
| **Dynamic Toolset** | ✅ Follows ADR | Network Policies + TLS | - |
| **Future Services** | 📋 Planned | Evaluate threat model per service | Internal: ADR-036, External: DD-AUTH-014 |

**Note**: ADR-036 applies to **internal-only services**. External-facing services require SAR authentication per DD-AUTH-014.

---

## Validation

### Security Validation
- [ ] Network Policies deployed for all services
- [ ] TLS configured for inter-service communication
- [ ] No authentication middleware in service code
- [ ] Security review passed

### Testing Validation
- [ ] Integration tests pass without auth setup
- [ ] Performance tests show latency improvements
- [ ] E2E tests validate end-to-end security

### Documentation Validation
- [ ] Network Policy examples documented
- [ ] Deployment guides updated
- [ ] Sidecar patterns documented

---

## References

### Internal Documents
- **DD-GATEWAY-006**: Gateway Authentication Strategy (superseded by this ADR)
- **DD-HOLMESGPT-011**: HolmesGPT Authentication Strategy (superseded by this ADR)
- **ADR-032**: Data Access Layer Isolation (Section 4c: Authentication & Security)
- **ADR-014**: Notification Service External Auth (specific to Slack/external integrations)

### External References
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Kubernetes Service TLS](https://kubernetes.io/docs/concepts/services-networking/service/#ssl-support)
- [Istio Security](https://istio.io/latest/docs/concepts/security/)
- [Envoy External Authorization](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_authz_filter)

---

## Appendix: Migration Checklist

For each service being migrated:

### Code Changes
- [ ] Remove `pkg/<service>/middleware/auth.go`
- [ ] Remove `pkg/<service>/middleware/authz.go`
- [ ] Remove `k8sClientset` from `server.NewServer()` (if only used for auth)
- [ ] Remove `DisableAuth` from `server.Config`
- [ ] Remove authentication metrics
- [ ] Update server constructor calls in `cmd/` and tests

### Test Changes
- [ ] Remove `SetupSecurityTokens()` from test helpers
- [ ] Remove ServiceAccount creation from integration tests
- [ ] Remove RBAC setup from integration tests
- [ ] Simplify test infrastructure setup

### Documentation Changes
- [ ] Add Network Policy examples to service docs
- [ ] Document TLS requirements
- [ ] Add deployment security guide
- [ ] Update README with security model

### Deployment Changes
- [ ] Create Network Policy manifests
- [ ] Configure TLS certificates
- [ ] Update deployment manifests
- [ ] Add security validation to CI/CD

---

**Approved by**: @jordigilh  
**Implementation Status**: In Progress (Dynamic Toolset)  
**Next Review**: After all services migrated (Q1 2026)

