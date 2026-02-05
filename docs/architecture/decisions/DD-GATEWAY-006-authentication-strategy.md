# DD-GATEWAY-006: Gateway Authentication and Authorization Strategy

**Status**: ⛔ **SUPERSEDED** by DD-AUTH-014 V2.0 (January 29, 2026)
**Original Date**: 2025-10-27
**Superseded Date**: 2026-01-29
**Deciders**: @jordigilh
**Related**: BR-GATEWAY-066 (Authentication), BR-GATEWAY-069 (Authorization), VULN-GATEWAY-001, VULN-GATEWAY-002

---

## ⚠️ **SUPERSEDED NOTICE**

**This design decision has been superseded by [DD-AUTH-014 V2.0](./DD-AUTH-014-middleware-based-sar-authentication.md).**

**Reason for Change**:
- Gateway is **external-facing** entry point (Prometheus AlertManager, K8s Event forwarders)
- **Network Policies alone** insufficient for defense-in-depth security
- **SOC2 compliance** requires operator attribution for signal injection (ActorID)
- **Zero-trust architecture** requires authentication at application layer
- **Webhook compatibility**: AlertManager + K8s Events natively support Bearer tokens

**New Approach** (DD-AUTH-014 V2.0):
- ✅ **Kubernetes TokenReview**: Validate ServiceAccount tokens (BR-GATEWAY-182)
- ✅ **SubjectAccessReview (SAR)**: Authorize CRD creation permissions (BR-GATEWAY-183)
- ✅ **Audit Logging**: Capture authenticated user identity for SOC2 compliance
- ✅ **No caching**: Low throughput (<100 signals/min) + NetworkPolicies reduce risk

**Migration Path**:
- Deploy Gateway with SAR middleware (same pattern as DataStorage/HAPI)
- Configure webhooks with Bearer tokens (AlertManager `http_config.authorization`)
- Create RBAC: ClusterRole `gateway-signal-sender` with `create remediationrequests` permission

**See**: [DD-AUTH-014 V2.0](./DD-AUTH-014-middleware-based-sar-authentication.md) for complete implementation details

---

## 📜 **Original Decision (OBSOLETE - For Historical Reference)**

## Context

The Gateway service initially implemented OAuth2-based authentication (TokenReview) and authorization (SubjectAccessReview) middleware to secure the `/webhook/prometheus` endpoint. However, during integration testing, several challenges emerged:

1. **Testing Complexity**: Setting up ServiceAccounts, tokens, and RBAC for integration tests added significant complexity
2. **K8s API Throttling**: TokenReview and SubjectAccessReview calls on every request created K8s API load
3. **In-Cluster Use Case**: The Gateway is designed for in-cluster communication, not external access
4. **Network-Level Security**: Kubernetes provides native network-level security mechanisms (Network Policies, TLS)
5. **Flexibility**: Different deployments may require different authentication mechanisms (mTLS, API keys, OAuth2)

## Decision

**Remove OAuth2 authentication and authorization middleware from the Gateway service** and adopt a **layered security approach** that leverages Kubernetes-native mechanisms and deployment-time flexibility.

### Core Principles

1. **Network-Level Security First**: Use Kubernetes Network Policies and TLS for traffic encryption
2. **Deployment-Time Flexibility**: Support sidecar containers for custom protocol-based authentication and authorization
3. **Simplicity**: Keep the Gateway service focused on signal processing, not authentication
4. **Defense-in-Depth**: Combine multiple security layers for comprehensive protection

### Security Architecture

#### Layer 1: Network Isolation (MANDATORY)
- **Kubernetes Network Policies**: Restrict traffic to authorized sources (e.g., Prometheus, monitoring namespace)
- **Namespace Isolation**: Deploy Gateway in a dedicated namespace with strict ingress rules
- **Service-Level TLS**: Encrypt traffic between client and Gateway using Kubernetes Service TLS

#### Layer 2: Transport Security (MANDATORY)
- **TLS Encryption**: All traffic MUST use TLS (either via Service TLS or reverse proxy)
- **Certificate Management**: Use cert-manager or similar for automated certificate rotation
- **Cipher Suite Configuration**: Enforce strong TLS 1.3 cipher suites

#### Layer 3: Application-Level Authentication (OPTIONAL, Deployment-Specific)
- **Sidecar Pattern**: Deploy authentication/authorization as a sidecar container
- **Protocol Flexibility**: Support mTLS, OAuth2, API keys, or custom protocols via sidecar
- **Examples**:
  - **Envoy + Authorino**: For OAuth2/OIDC authentication
  - **Istio**: For mTLS and service mesh integration
  - **Custom Sidecar**: For proprietary authentication mechanisms

### Implementation Details

#### Gateway Service Changes
1. **Remove Middleware**:
   - Delete `TokenReviewAuth` middleware
   - Delete `SubjectAccessReviewAuthz` middleware
   - Delete `DisableAuth` configuration option
   - Delete `ValidateAuthConfig` function

2. **Keep Security Middleware**:
   - **Rate Limiting**: Protect against DoS attacks (BR-GATEWAY-071)
   - **Payload Size Limit**: Prevent large request attacks (BR-GATEWAY-073)
   - **Log Sanitization**: Redact sensitive data from logs (BR-GATEWAY-074)
   - **Security Headers**: Prevent common web vulnerabilities (BR-GATEWAY-075)
   - **Timestamp Validation**: Prevent replay attacks (BR-GATEWAY-076)

3. **Simplify Server Constructor**:
   - Remove `k8sClientset` parameter (no longer needed for auth)
   - Remove `DisableAuth` from `Config` struct
   - Remove authentication-related metrics (`TokenReviewTimeouts`, `SubjectAccessReviewTimeouts`)

#### Deployment Configurations

##### Configuration 1: Network Policies + Service TLS (Recommended for In-Cluster)
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
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
```

##### Configuration 2: Sidecar Authentication (For Custom Requirements)
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
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8080
          name: http
      - name: auth-proxy
        image: envoy:v1.28
        ports:
        - containerPort: 8443
          name: https
        volumeMounts:
        - name: envoy-config
          mountPath: /etc/envoy
        - name: tls-certs
          mountPath: /etc/tls
      volumes:
      - name: envoy-config
        configMap:
          name: envoy-auth-config
      - name: tls-certs
        secret:
          secretName: gateway-tls
```

##### Configuration 3: Reverse Proxy (For External Access)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway-external
  namespace: kubernaut-system
spec:
  type: LoadBalancer
  ports:
  - name: https
    port: 443
    targetPort: 8443
    protocol: TCP
  selector:
    app: nginx-gateway-proxy
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-gateway-proxy
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.25
        volumeMounts:
        - name: nginx-config
          mountPath: /etc/nginx/nginx.conf
          subPath: nginx.conf
        - name: tls-certs
          mountPath: /etc/nginx/tls
      volumes:
      - name: nginx-config
        configMap:
          name: nginx-gateway-config
      - name: tls-certs
        secret:
          secretName: gateway-external-tls
```

## Consequences

### Positive
1. **Simplified Gateway Service**: Removes authentication complexity from core service logic
2. **Reduced K8s API Load**: No TokenReview/SubjectAccessReview calls on every request
3. **Deployment Flexibility**: Different deployments can choose appropriate authentication mechanisms
4. **Easier Testing**: Integration tests don't require complex ServiceAccount setup
5. **Better Separation of Concerns**: Authentication is a deployment concern, not a service concern
6. **Performance**: Lower latency without K8s API calls for every request

### Negative
1. **Deployment Responsibility**: Operators must configure Network Policies and TLS correctly
2. **Documentation Burden**: Need clear documentation for different deployment scenarios
3. **No Built-in Auth**: Gateway service itself doesn't enforce authentication (relies on network layer)

### Neutral
1. **Security Model Shift**: From application-level to network-level security
2. **Sidecar Complexity**: Optional sidecar authentication adds deployment complexity (but only when needed)

## Implementation Plan

### Phase 1: Remove Authentication Middleware (Immediate - v1.0)
1. Delete `pkg/gateway/middleware/auth.go`
2. Delete `pkg/gateway/middleware/authz.go`
3. Remove `k8sClientset` from `server.NewServer()`
4. Remove `DisableAuth` from `server.Config`
5. Delete `pkg/gateway/server/config_validation.go`
6. Remove authentication metrics from `pkg/gateway/metrics/factory.go`

### Phase 2: Update Tests (Immediate - v1.0)
1. Remove `SetupSecurityTokens()` from `test/integration/gateway/helpers.go`
2. Delete `test/integration/gateway/security_integration_test.go`
3. Remove `KUBERNAUT_ENV` and `KUBERNAUT_DISABLE_AUTH_CONFIRM` environment variable usage
4. Simplify `StartTestGateway()` helper

### Phase 3: Documentation (Immediate - v1.0)
1. Create `docs/deployment/gateway-security.md` with Network Policies + TLS configuration
2. Update `docs/services/stateless/gateway-service/README.md` with security architecture
3. Document deployment prerequisites and security requirements
4. Update `IMPLEMENTATION_PLAN_V2.12.md` to reflect authentication removal

### Phase 4: Deployment Tooling (DEFERRED to v2.0)
**Decision**: Defer Helm/Kustomize support until pilot deployments validate security direction.

**Rationale**:
- **Pilot Focus**: v1.0 targets pilot deployments with manual Network Policy + TLS configuration
- **Security Evolution**: Sidecar authentication patterns need real-world validation before standardization
- **Flexibility**: Manual configuration allows experimentation with different security approaches
- **Complexity**: Helm/Kustomize add maintenance burden without proven deployment patterns

**Deferred Items**:
1. Kustomize overlays for different deployment scenarios
2. Helm chart with configurable security options
3. Automated cert-manager integration
4. Sidecar authentication examples (Envoy, Istio)

**v2.0 Criteria for Re-evaluation**:
- At least 3 pilot deployments successfully running with Network Policies + TLS
- Clear understanding of common deployment patterns and requirements
- Validated need for sidecar authentication in production use cases
- Operator feedback on manual vs. automated configuration preferences

## Alternatives Considered

### Alternative 1: Keep TokenReview with Caching
**Rejected**: Still adds K8s API dependency and complexity. Caching mitigates but doesn't eliminate the issue.

### Alternative 2: API Key Authentication
**Rejected**: Requires key management, rotation, and distribution. Adds complexity without significant benefit over network-level security.

### Alternative 3: mTLS in Gateway Service
**Rejected**: Inflexible for different deployment scenarios. Better implemented as a sidecar or reverse proxy for deployments that need it.

## References

- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Kubernetes Service TLS](https://kubernetes.io/docs/concepts/services-networking/service/#ssl-support-on-aws)
- [Envoy External Authorization](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_authz_filter)
- [Istio Security](https://istio.io/latest/docs/concepts/security/)
- [cert-manager](https://cert-manager.io/)
- [DD-GATEWAY-002: Service Mandatory Dependencies](./DD-GATEWAY-002-service-mandatory-dependencies.md)
- [DD-GATEWAY-003: Redis Outage Risk Tracking Metrics](./DD-GATEWAY-003-redis-outage-metrics.md)

## Confidence Assessment

**Confidence Level**: 95%

**Justification**:
- **Industry Best Practice**: Network-level security is standard for in-cluster services (Kubernetes Network Policies, Service Mesh)
- **Proven Patterns**: Sidecar authentication is a well-established pattern (Istio, Envoy, Linkerd)
- **Reduced Complexity**: Removing authentication middleware simplifies the Gateway service significantly
- **Flexibility**: Deployment-time authentication choice allows for different security requirements
- **Performance**: Eliminates K8s API calls for every request, improving latency

**Risks**:
- **Misconfiguration**: Operators must correctly configure Network Policies and TLS (mitigated by documentation and examples)
- **Defense-in-Depth**: Relies on network layer; no application-level authentication fallback (acceptable for in-cluster use case)

**Validation**:
- Integration tests will verify Gateway functionality without authentication middleware
- Deployment examples will demonstrate secure configurations
- Performance testing will confirm latency improvements


**Status**: Approved
**Date**: 2025-10-27
**Deciders**: @jordigilh
**Related**: BR-GATEWAY-066 (Authentication), BR-GATEWAY-069 (Authorization), VULN-GATEWAY-001, VULN-GATEWAY-002

## Context

The Gateway service initially implemented OAuth2-based authentication (TokenReview) and authorization (SubjectAccessReview) middleware to secure the `/webhook/prometheus` endpoint. However, during integration testing, several challenges emerged:

1. **Testing Complexity**: Setting up ServiceAccounts, tokens, and RBAC for integration tests added significant complexity
2. **K8s API Throttling**: TokenReview and SubjectAccessReview calls on every request created K8s API load
3. **In-Cluster Use Case**: The Gateway is designed for in-cluster communication, not external access
4. **Network-Level Security**: Kubernetes provides native network-level security mechanisms (Network Policies, TLS)
5. **Flexibility**: Different deployments may require different authentication mechanisms (mTLS, API keys, OAuth2)

## Decision

**Remove OAuth2 authentication and authorization middleware from the Gateway service** and adopt a **layered security approach** that leverages Kubernetes-native mechanisms and deployment-time flexibility.

### Core Principles

1. **Network-Level Security First**: Use Kubernetes Network Policies and TLS for traffic encryption
2. **Deployment-Time Flexibility**: Support sidecar containers for custom protocol-based authentication and authorization
3. **Simplicity**: Keep the Gateway service focused on signal processing, not authentication
4. **Defense-in-Depth**: Combine multiple security layers for comprehensive protection

### Security Architecture

#### Layer 1: Network Isolation (MANDATORY)
- **Kubernetes Network Policies**: Restrict traffic to authorized sources (e.g., Prometheus, monitoring namespace)
- **Namespace Isolation**: Deploy Gateway in a dedicated namespace with strict ingress rules
- **Service-Level TLS**: Encrypt traffic between client and Gateway using Kubernetes Service TLS

#### Layer 2: Transport Security (MANDATORY)
- **TLS Encryption**: All traffic MUST use TLS (either via Service TLS or reverse proxy)
- **Certificate Management**: Use cert-manager or similar for automated certificate rotation
- **Cipher Suite Configuration**: Enforce strong TLS 1.3 cipher suites

#### Layer 3: Application-Level Authentication (OPTIONAL, Deployment-Specific)
- **Sidecar Pattern**: Deploy authentication/authorization as a sidecar container
- **Protocol Flexibility**: Support mTLS, OAuth2, API keys, or custom protocols via sidecar
- **Examples**:
  - **Envoy + Authorino**: For OAuth2/OIDC authentication
  - **Istio**: For mTLS and service mesh integration
  - **Custom Sidecar**: For proprietary authentication mechanisms

### Implementation Details

#### Gateway Service Changes
1. **Remove Middleware**:
   - Delete `TokenReviewAuth` middleware
   - Delete `SubjectAccessReviewAuthz` middleware
   - Delete `DisableAuth` configuration option
   - Delete `ValidateAuthConfig` function

2. **Keep Security Middleware**:
   - **Rate Limiting**: Protect against DoS attacks (BR-GATEWAY-071)
   - **Payload Size Limit**: Prevent large request attacks (BR-GATEWAY-073)
   - **Log Sanitization**: Redact sensitive data from logs (BR-GATEWAY-074)
   - **Security Headers**: Prevent common web vulnerabilities (BR-GATEWAY-075)
   - **Timestamp Validation**: Prevent replay attacks (BR-GATEWAY-076)

3. **Simplify Server Constructor**:
   - Remove `k8sClientset` parameter (no longer needed for auth)
   - Remove `DisableAuth` from `Config` struct
   - Remove authentication-related metrics (`TokenReviewTimeouts`, `SubjectAccessReviewTimeouts`)

#### Deployment Configurations

##### Configuration 1: Network Policies + Service TLS (Recommended for In-Cluster)
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
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
```

##### Configuration 2: Sidecar Authentication (For Custom Requirements)
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
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8080
          name: http
      - name: auth-proxy
        image: envoy:v1.28
        ports:
        - containerPort: 8443
          name: https
        volumeMounts:
        - name: envoy-config
          mountPath: /etc/envoy
        - name: tls-certs
          mountPath: /etc/tls
      volumes:
      - name: envoy-config
        configMap:
          name: envoy-auth-config
      - name: tls-certs
        secret:
          secretName: gateway-tls
```

##### Configuration 3: Reverse Proxy (For External Access)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway-external
  namespace: kubernaut-system
spec:
  type: LoadBalancer
  ports:
  - name: https
    port: 443
    targetPort: 8443
    protocol: TCP
  selector:
    app: nginx-gateway-proxy
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-gateway-proxy
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.25
        volumeMounts:
        - name: nginx-config
          mountPath: /etc/nginx/nginx.conf
          subPath: nginx.conf
        - name: tls-certs
          mountPath: /etc/nginx/tls
      volumes:
      - name: nginx-config
        configMap:
          name: nginx-gateway-config
      - name: tls-certs
        secret:
          secretName: gateway-external-tls
```

## Consequences

### Positive
1. **Simplified Gateway Service**: Removes authentication complexity from core service logic
2. **Reduced K8s API Load**: No TokenReview/SubjectAccessReview calls on every request
3. **Deployment Flexibility**: Different deployments can choose appropriate authentication mechanisms
4. **Easier Testing**: Integration tests don't require complex ServiceAccount setup
5. **Better Separation of Concerns**: Authentication is a deployment concern, not a service concern
6. **Performance**: Lower latency without K8s API calls for every request

### Negative
1. **Deployment Responsibility**: Operators must configure Network Policies and TLS correctly
2. **Documentation Burden**: Need clear documentation for different deployment scenarios
3. **No Built-in Auth**: Gateway service itself doesn't enforce authentication (relies on network layer)

### Neutral
1. **Security Model Shift**: From application-level to network-level security
2. **Sidecar Complexity**: Optional sidecar authentication adds deployment complexity (but only when needed)

## Implementation Plan

### Phase 1: Remove Authentication Middleware (Immediate - v1.0)
1. Delete `pkg/gateway/middleware/auth.go`
2. Delete `pkg/gateway/middleware/authz.go`
3. Remove `k8sClientset` from `server.NewServer()`
4. Remove `DisableAuth` from `server.Config`
5. Delete `pkg/gateway/server/config_validation.go`
6. Remove authentication metrics from `pkg/gateway/metrics/factory.go`

### Phase 2: Update Tests (Immediate - v1.0)
1. Remove `SetupSecurityTokens()` from `test/integration/gateway/helpers.go`
2. Delete `test/integration/gateway/security_integration_test.go`
3. Remove `KUBERNAUT_ENV` and `KUBERNAUT_DISABLE_AUTH_CONFIRM` environment variable usage
4. Simplify `StartTestGateway()` helper

### Phase 3: Documentation (Immediate - v1.0)
1. Create `docs/deployment/gateway-security.md` with Network Policies + TLS configuration
2. Update `docs/services/stateless/gateway-service/README.md` with security architecture
3. Document deployment prerequisites and security requirements
4. Update `IMPLEMENTATION_PLAN_V2.12.md` to reflect authentication removal

### Phase 4: Deployment Tooling (DEFERRED to v2.0)
**Decision**: Defer Helm/Kustomize support until pilot deployments validate security direction.

**Rationale**:
- **Pilot Focus**: v1.0 targets pilot deployments with manual Network Policy + TLS configuration
- **Security Evolution**: Sidecar authentication patterns need real-world validation before standardization
- **Flexibility**: Manual configuration allows experimentation with different security approaches
- **Complexity**: Helm/Kustomize add maintenance burden without proven deployment patterns

**Deferred Items**:
1. Kustomize overlays for different deployment scenarios
2. Helm chart with configurable security options
3. Automated cert-manager integration
4. Sidecar authentication examples (Envoy, Istio)

**v2.0 Criteria for Re-evaluation**:
- At least 3 pilot deployments successfully running with Network Policies + TLS
- Clear understanding of common deployment patterns and requirements
- Validated need for sidecar authentication in production use cases
- Operator feedback on manual vs. automated configuration preferences

## Alternatives Considered

### Alternative 1: Keep TokenReview with Caching
**Rejected**: Still adds K8s API dependency and complexity. Caching mitigates but doesn't eliminate the issue.

### Alternative 2: API Key Authentication
**Rejected**: Requires key management, rotation, and distribution. Adds complexity without significant benefit over network-level security.

### Alternative 3: mTLS in Gateway Service
**Rejected**: Inflexible for different deployment scenarios. Better implemented as a sidecar or reverse proxy for deployments that need it.

## References

- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Kubernetes Service TLS](https://kubernetes.io/docs/concepts/services-networking/service/#ssl-support-on-aws)
- [Envoy External Authorization](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_authz_filter)
- [Istio Security](https://istio.io/latest/docs/concepts/security/)
- [cert-manager](https://cert-manager.io/)
- [DD-GATEWAY-002: Service Mandatory Dependencies](./DD-GATEWAY-002-service-mandatory-dependencies.md)
- [DD-GATEWAY-003: Redis Outage Risk Tracking Metrics](./DD-GATEWAY-003-redis-outage-metrics.md)

## Confidence Assessment

**Confidence Level**: 95%

**Justification**:
- **Industry Best Practice**: Network-level security is standard for in-cluster services (Kubernetes Network Policies, Service Mesh)
- **Proven Patterns**: Sidecar authentication is a well-established pattern (Istio, Envoy, Linkerd)
- **Reduced Complexity**: Removing authentication middleware simplifies the Gateway service significantly
- **Flexibility**: Deployment-time authentication choice allows for different security requirements
- **Performance**: Eliminates K8s API calls for every request, improving latency

**Risks**:
- **Misconfiguration**: Operators must correctly configure Network Policies and TLS (mitigated by documentation and examples)
- **Defense-in-Depth**: Relies on network layer; no application-level authentication fallback (acceptable for in-cluster use case)

**Validation**:
- Integration tests will verify Gateway functionality without authentication middleware
- Deployment examples will demonstrate secure configurations
- Performance testing will confirm latency improvements


**Status**: Approved
**Date**: 2025-10-27
**Deciders**: @jordigilh
**Related**: BR-GATEWAY-066 (Authentication), BR-GATEWAY-069 (Authorization), VULN-GATEWAY-001, VULN-GATEWAY-002

## Context

The Gateway service initially implemented OAuth2-based authentication (TokenReview) and authorization (SubjectAccessReview) middleware to secure the `/webhook/prometheus` endpoint. However, during integration testing, several challenges emerged:

1. **Testing Complexity**: Setting up ServiceAccounts, tokens, and RBAC for integration tests added significant complexity
2. **K8s API Throttling**: TokenReview and SubjectAccessReview calls on every request created K8s API load
3. **In-Cluster Use Case**: The Gateway is designed for in-cluster communication, not external access
4. **Network-Level Security**: Kubernetes provides native network-level security mechanisms (Network Policies, TLS)
5. **Flexibility**: Different deployments may require different authentication mechanisms (mTLS, API keys, OAuth2)

## Decision

**Remove OAuth2 authentication and authorization middleware from the Gateway service** and adopt a **layered security approach** that leverages Kubernetes-native mechanisms and deployment-time flexibility.

### Core Principles

1. **Network-Level Security First**: Use Kubernetes Network Policies and TLS for traffic encryption
2. **Deployment-Time Flexibility**: Support sidecar containers for custom protocol-based authentication and authorization
3. **Simplicity**: Keep the Gateway service focused on signal processing, not authentication
4. **Defense-in-Depth**: Combine multiple security layers for comprehensive protection

### Security Architecture

#### Layer 1: Network Isolation (MANDATORY)
- **Kubernetes Network Policies**: Restrict traffic to authorized sources (e.g., Prometheus, monitoring namespace)
- **Namespace Isolation**: Deploy Gateway in a dedicated namespace with strict ingress rules
- **Service-Level TLS**: Encrypt traffic between client and Gateway using Kubernetes Service TLS

#### Layer 2: Transport Security (MANDATORY)
- **TLS Encryption**: All traffic MUST use TLS (either via Service TLS or reverse proxy)
- **Certificate Management**: Use cert-manager or similar for automated certificate rotation
- **Cipher Suite Configuration**: Enforce strong TLS 1.3 cipher suites

#### Layer 3: Application-Level Authentication (OPTIONAL, Deployment-Specific)
- **Sidecar Pattern**: Deploy authentication/authorization as a sidecar container
- **Protocol Flexibility**: Support mTLS, OAuth2, API keys, or custom protocols via sidecar
- **Examples**:
  - **Envoy + Authorino**: For OAuth2/OIDC authentication
  - **Istio**: For mTLS and service mesh integration
  - **Custom Sidecar**: For proprietary authentication mechanisms

### Implementation Details

#### Gateway Service Changes
1. **Remove Middleware**:
   - Delete `TokenReviewAuth` middleware
   - Delete `SubjectAccessReviewAuthz` middleware
   - Delete `DisableAuth` configuration option
   - Delete `ValidateAuthConfig` function

2. **Keep Security Middleware**:
   - **Rate Limiting**: Protect against DoS attacks (BR-GATEWAY-071)
   - **Payload Size Limit**: Prevent large request attacks (BR-GATEWAY-073)
   - **Log Sanitization**: Redact sensitive data from logs (BR-GATEWAY-074)
   - **Security Headers**: Prevent common web vulnerabilities (BR-GATEWAY-075)
   - **Timestamp Validation**: Prevent replay attacks (BR-GATEWAY-076)

3. **Simplify Server Constructor**:
   - Remove `k8sClientset` parameter (no longer needed for auth)
   - Remove `DisableAuth` from `Config` struct
   - Remove authentication-related metrics (`TokenReviewTimeouts`, `SubjectAccessReviewTimeouts`)

#### Deployment Configurations

##### Configuration 1: Network Policies + Service TLS (Recommended for In-Cluster)
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
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
```

##### Configuration 2: Sidecar Authentication (For Custom Requirements)
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
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8080
          name: http
      - name: auth-proxy
        image: envoy:v1.28
        ports:
        - containerPort: 8443
          name: https
        volumeMounts:
        - name: envoy-config
          mountPath: /etc/envoy
        - name: tls-certs
          mountPath: /etc/tls
      volumes:
      - name: envoy-config
        configMap:
          name: envoy-auth-config
      - name: tls-certs
        secret:
          secretName: gateway-tls
```

##### Configuration 3: Reverse Proxy (For External Access)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway-external
  namespace: kubernaut-system
spec:
  type: LoadBalancer
  ports:
  - name: https
    port: 443
    targetPort: 8443
    protocol: TCP
  selector:
    app: nginx-gateway-proxy
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-gateway-proxy
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.25
        volumeMounts:
        - name: nginx-config
          mountPath: /etc/nginx/nginx.conf
          subPath: nginx.conf
        - name: tls-certs
          mountPath: /etc/nginx/tls
      volumes:
      - name: nginx-config
        configMap:
          name: nginx-gateway-config
      - name: tls-certs
        secret:
          secretName: gateway-external-tls
```

## Consequences

### Positive
1. **Simplified Gateway Service**: Removes authentication complexity from core service logic
2. **Reduced K8s API Load**: No TokenReview/SubjectAccessReview calls on every request
3. **Deployment Flexibility**: Different deployments can choose appropriate authentication mechanisms
4. **Easier Testing**: Integration tests don't require complex ServiceAccount setup
5. **Better Separation of Concerns**: Authentication is a deployment concern, not a service concern
6. **Performance**: Lower latency without K8s API calls for every request

### Negative
1. **Deployment Responsibility**: Operators must configure Network Policies and TLS correctly
2. **Documentation Burden**: Need clear documentation for different deployment scenarios
3. **No Built-in Auth**: Gateway service itself doesn't enforce authentication (relies on network layer)

### Neutral
1. **Security Model Shift**: From application-level to network-level security
2. **Sidecar Complexity**: Optional sidecar authentication adds deployment complexity (but only when needed)

## Implementation Plan

### Phase 1: Remove Authentication Middleware (Immediate - v1.0)
1. Delete `pkg/gateway/middleware/auth.go`
2. Delete `pkg/gateway/middleware/authz.go`
3. Remove `k8sClientset` from `server.NewServer()`
4. Remove `DisableAuth` from `server.Config`
5. Delete `pkg/gateway/server/config_validation.go`
6. Remove authentication metrics from `pkg/gateway/metrics/factory.go`

### Phase 2: Update Tests (Immediate - v1.0)
1. Remove `SetupSecurityTokens()` from `test/integration/gateway/helpers.go`
2. Delete `test/integration/gateway/security_integration_test.go`
3. Remove `KUBERNAUT_ENV` and `KUBERNAUT_DISABLE_AUTH_CONFIRM` environment variable usage
4. Simplify `StartTestGateway()` helper

### Phase 3: Documentation (Immediate - v1.0)
1. Create `docs/deployment/gateway-security.md` with Network Policies + TLS configuration
2. Update `docs/services/stateless/gateway-service/README.md` with security architecture
3. Document deployment prerequisites and security requirements
4. Update `IMPLEMENTATION_PLAN_V2.12.md` to reflect authentication removal

### Phase 4: Deployment Tooling (DEFERRED to v2.0)
**Decision**: Defer Helm/Kustomize support until pilot deployments validate security direction.

**Rationale**:
- **Pilot Focus**: v1.0 targets pilot deployments with manual Network Policy + TLS configuration
- **Security Evolution**: Sidecar authentication patterns need real-world validation before standardization
- **Flexibility**: Manual configuration allows experimentation with different security approaches
- **Complexity**: Helm/Kustomize add maintenance burden without proven deployment patterns

**Deferred Items**:
1. Kustomize overlays for different deployment scenarios
2. Helm chart with configurable security options
3. Automated cert-manager integration
4. Sidecar authentication examples (Envoy, Istio)

**v2.0 Criteria for Re-evaluation**:
- At least 3 pilot deployments successfully running with Network Policies + TLS
- Clear understanding of common deployment patterns and requirements
- Validated need for sidecar authentication in production use cases
- Operator feedback on manual vs. automated configuration preferences

## Alternatives Considered

### Alternative 1: Keep TokenReview with Caching
**Rejected**: Still adds K8s API dependency and complexity. Caching mitigates but doesn't eliminate the issue.

### Alternative 2: API Key Authentication
**Rejected**: Requires key management, rotation, and distribution. Adds complexity without significant benefit over network-level security.

### Alternative 3: mTLS in Gateway Service
**Rejected**: Inflexible for different deployment scenarios. Better implemented as a sidecar or reverse proxy for deployments that need it.

## References

- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Kubernetes Service TLS](https://kubernetes.io/docs/concepts/services-networking/service/#ssl-support-on-aws)
- [Envoy External Authorization](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_authz_filter)
- [Istio Security](https://istio.io/latest/docs/concepts/security/)
- [cert-manager](https://cert-manager.io/)
- [DD-GATEWAY-002: Service Mandatory Dependencies](./DD-GATEWAY-002-service-mandatory-dependencies.md)
- [DD-GATEWAY-003: Redis Outage Risk Tracking Metrics](./DD-GATEWAY-003-redis-outage-metrics.md)

## Confidence Assessment

**Confidence Level**: 95%

**Justification**:
- **Industry Best Practice**: Network-level security is standard for in-cluster services (Kubernetes Network Policies, Service Mesh)
- **Proven Patterns**: Sidecar authentication is a well-established pattern (Istio, Envoy, Linkerd)
- **Reduced Complexity**: Removing authentication middleware simplifies the Gateway service significantly
- **Flexibility**: Deployment-time authentication choice allows for different security requirements
- **Performance**: Eliminates K8s API calls for every request, improving latency

**Risks**:
- **Misconfiguration**: Operators must correctly configure Network Policies and TLS (mitigated by documentation and examples)
- **Defense-in-Depth**: Relies on network layer; no application-level authentication fallback (acceptable for in-cluster use case)

**Validation**:
- Integration tests will verify Gateway functionality without authentication middleware
- Deployment examples will demonstrate secure configurations
- Performance testing will confirm latency improvements




**Status**: Approved
**Date**: 2025-10-27
**Deciders**: @jordigilh
**Related**: BR-GATEWAY-066 (Authentication), BR-GATEWAY-069 (Authorization), VULN-GATEWAY-001, VULN-GATEWAY-002

## Context

The Gateway service initially implemented OAuth2-based authentication (TokenReview) and authorization (SubjectAccessReview) middleware to secure the `/webhook/prometheus` endpoint. However, during integration testing, several challenges emerged:

1. **Testing Complexity**: Setting up ServiceAccounts, tokens, and RBAC for integration tests added significant complexity
2. **K8s API Throttling**: TokenReview and SubjectAccessReview calls on every request created K8s API load
3. **In-Cluster Use Case**: The Gateway is designed for in-cluster communication, not external access
4. **Network-Level Security**: Kubernetes provides native network-level security mechanisms (Network Policies, TLS)
5. **Flexibility**: Different deployments may require different authentication mechanisms (mTLS, API keys, OAuth2)

## Decision

**Remove OAuth2 authentication and authorization middleware from the Gateway service** and adopt a **layered security approach** that leverages Kubernetes-native mechanisms and deployment-time flexibility.

### Core Principles

1. **Network-Level Security First**: Use Kubernetes Network Policies and TLS for traffic encryption
2. **Deployment-Time Flexibility**: Support sidecar containers for custom protocol-based authentication and authorization
3. **Simplicity**: Keep the Gateway service focused on signal processing, not authentication
4. **Defense-in-Depth**: Combine multiple security layers for comprehensive protection

### Security Architecture

#### Layer 1: Network Isolation (MANDATORY)
- **Kubernetes Network Policies**: Restrict traffic to authorized sources (e.g., Prometheus, monitoring namespace)
- **Namespace Isolation**: Deploy Gateway in a dedicated namespace with strict ingress rules
- **Service-Level TLS**: Encrypt traffic between client and Gateway using Kubernetes Service TLS

#### Layer 2: Transport Security (MANDATORY)
- **TLS Encryption**: All traffic MUST use TLS (either via Service TLS or reverse proxy)
- **Certificate Management**: Use cert-manager or similar for automated certificate rotation
- **Cipher Suite Configuration**: Enforce strong TLS 1.3 cipher suites

#### Layer 3: Application-Level Authentication (OPTIONAL, Deployment-Specific)
- **Sidecar Pattern**: Deploy authentication/authorization as a sidecar container
- **Protocol Flexibility**: Support mTLS, OAuth2, API keys, or custom protocols via sidecar
- **Examples**:
  - **Envoy + Authorino**: For OAuth2/OIDC authentication
  - **Istio**: For mTLS and service mesh integration
  - **Custom Sidecar**: For proprietary authentication mechanisms

### Implementation Details

#### Gateway Service Changes
1. **Remove Middleware**:
   - Delete `TokenReviewAuth` middleware
   - Delete `SubjectAccessReviewAuthz` middleware
   - Delete `DisableAuth` configuration option
   - Delete `ValidateAuthConfig` function

2. **Keep Security Middleware**:
   - **Rate Limiting**: Protect against DoS attacks (BR-GATEWAY-071)
   - **Payload Size Limit**: Prevent large request attacks (BR-GATEWAY-073)
   - **Log Sanitization**: Redact sensitive data from logs (BR-GATEWAY-074)
   - **Security Headers**: Prevent common web vulnerabilities (BR-GATEWAY-075)
   - **Timestamp Validation**: Prevent replay attacks (BR-GATEWAY-076)

3. **Simplify Server Constructor**:
   - Remove `k8sClientset` parameter (no longer needed for auth)
   - Remove `DisableAuth` from `Config` struct
   - Remove authentication-related metrics (`TokenReviewTimeouts`, `SubjectAccessReviewTimeouts`)

#### Deployment Configurations

##### Configuration 1: Network Policies + Service TLS (Recommended for In-Cluster)
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
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
```

##### Configuration 2: Sidecar Authentication (For Custom Requirements)
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
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8080
          name: http
      - name: auth-proxy
        image: envoy:v1.28
        ports:
        - containerPort: 8443
          name: https
        volumeMounts:
        - name: envoy-config
          mountPath: /etc/envoy
        - name: tls-certs
          mountPath: /etc/tls
      volumes:
      - name: envoy-config
        configMap:
          name: envoy-auth-config
      - name: tls-certs
        secret:
          secretName: gateway-tls
```

##### Configuration 3: Reverse Proxy (For External Access)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway-external
  namespace: kubernaut-system
spec:
  type: LoadBalancer
  ports:
  - name: https
    port: 443
    targetPort: 8443
    protocol: TCP
  selector:
    app: nginx-gateway-proxy
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-gateway-proxy
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.25
        volumeMounts:
        - name: nginx-config
          mountPath: /etc/nginx/nginx.conf
          subPath: nginx.conf
        - name: tls-certs
          mountPath: /etc/nginx/tls
      volumes:
      - name: nginx-config
        configMap:
          name: nginx-gateway-config
      - name: tls-certs
        secret:
          secretName: gateway-external-tls
```

## Consequences

### Positive
1. **Simplified Gateway Service**: Removes authentication complexity from core service logic
2. **Reduced K8s API Load**: No TokenReview/SubjectAccessReview calls on every request
3. **Deployment Flexibility**: Different deployments can choose appropriate authentication mechanisms
4. **Easier Testing**: Integration tests don't require complex ServiceAccount setup
5. **Better Separation of Concerns**: Authentication is a deployment concern, not a service concern
6. **Performance**: Lower latency without K8s API calls for every request

### Negative
1. **Deployment Responsibility**: Operators must configure Network Policies and TLS correctly
2. **Documentation Burden**: Need clear documentation for different deployment scenarios
3. **No Built-in Auth**: Gateway service itself doesn't enforce authentication (relies on network layer)

### Neutral
1. **Security Model Shift**: From application-level to network-level security
2. **Sidecar Complexity**: Optional sidecar authentication adds deployment complexity (but only when needed)

## Implementation Plan

### Phase 1: Remove Authentication Middleware (Immediate - v1.0)
1. Delete `pkg/gateway/middleware/auth.go`
2. Delete `pkg/gateway/middleware/authz.go`
3. Remove `k8sClientset` from `server.NewServer()`
4. Remove `DisableAuth` from `server.Config`
5. Delete `pkg/gateway/server/config_validation.go`
6. Remove authentication metrics from `pkg/gateway/metrics/factory.go`

### Phase 2: Update Tests (Immediate - v1.0)
1. Remove `SetupSecurityTokens()` from `test/integration/gateway/helpers.go`
2. Delete `test/integration/gateway/security_integration_test.go`
3. Remove `KUBERNAUT_ENV` and `KUBERNAUT_DISABLE_AUTH_CONFIRM` environment variable usage
4. Simplify `StartTestGateway()` helper

### Phase 3: Documentation (Immediate - v1.0)
1. Create `docs/deployment/gateway-security.md` with Network Policies + TLS configuration
2. Update `docs/services/stateless/gateway-service/README.md` with security architecture
3. Document deployment prerequisites and security requirements
4. Update `IMPLEMENTATION_PLAN_V2.12.md` to reflect authentication removal

### Phase 4: Deployment Tooling (DEFERRED to v2.0)
**Decision**: Defer Helm/Kustomize support until pilot deployments validate security direction.

**Rationale**:
- **Pilot Focus**: v1.0 targets pilot deployments with manual Network Policy + TLS configuration
- **Security Evolution**: Sidecar authentication patterns need real-world validation before standardization
- **Flexibility**: Manual configuration allows experimentation with different security approaches
- **Complexity**: Helm/Kustomize add maintenance burden without proven deployment patterns

**Deferred Items**:
1. Kustomize overlays for different deployment scenarios
2. Helm chart with configurable security options
3. Automated cert-manager integration
4. Sidecar authentication examples (Envoy, Istio)

**v2.0 Criteria for Re-evaluation**:
- At least 3 pilot deployments successfully running with Network Policies + TLS
- Clear understanding of common deployment patterns and requirements
- Validated need for sidecar authentication in production use cases
- Operator feedback on manual vs. automated configuration preferences

## Alternatives Considered

### Alternative 1: Keep TokenReview with Caching
**Rejected**: Still adds K8s API dependency and complexity. Caching mitigates but doesn't eliminate the issue.

### Alternative 2: API Key Authentication
**Rejected**: Requires key management, rotation, and distribution. Adds complexity without significant benefit over network-level security.

### Alternative 3: mTLS in Gateway Service
**Rejected**: Inflexible for different deployment scenarios. Better implemented as a sidecar or reverse proxy for deployments that need it.

## References

- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Kubernetes Service TLS](https://kubernetes.io/docs/concepts/services-networking/service/#ssl-support-on-aws)
- [Envoy External Authorization](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_authz_filter)
- [Istio Security](https://istio.io/latest/docs/concepts/security/)
- [cert-manager](https://cert-manager.io/)
- [DD-GATEWAY-002: Service Mandatory Dependencies](./DD-GATEWAY-002-service-mandatory-dependencies.md)
- [DD-GATEWAY-003: Redis Outage Risk Tracking Metrics](./DD-GATEWAY-003-redis-outage-metrics.md)

## Confidence Assessment

**Confidence Level**: 95%

**Justification**:
- **Industry Best Practice**: Network-level security is standard for in-cluster services (Kubernetes Network Policies, Service Mesh)
- **Proven Patterns**: Sidecar authentication is a well-established pattern (Istio, Envoy, Linkerd)
- **Reduced Complexity**: Removing authentication middleware simplifies the Gateway service significantly
- **Flexibility**: Deployment-time authentication choice allows for different security requirements
- **Performance**: Eliminates K8s API calls for every request, improving latency

**Risks**:
- **Misconfiguration**: Operators must correctly configure Network Policies and TLS (mitigated by documentation and examples)
- **Defense-in-Depth**: Relies on network layer; no application-level authentication fallback (acceptable for in-cluster use case)

**Validation**:
- Integration tests will verify Gateway functionality without authentication middleware
- Deployment examples will demonstrate secure configurations
- Performance testing will confirm latency improvements


**Status**: Approved
**Date**: 2025-10-27
**Deciders**: @jordigilh
**Related**: BR-GATEWAY-066 (Authentication), BR-GATEWAY-069 (Authorization), VULN-GATEWAY-001, VULN-GATEWAY-002

## Context

The Gateway service initially implemented OAuth2-based authentication (TokenReview) and authorization (SubjectAccessReview) middleware to secure the `/webhook/prometheus` endpoint. However, during integration testing, several challenges emerged:

1. **Testing Complexity**: Setting up ServiceAccounts, tokens, and RBAC for integration tests added significant complexity
2. **K8s API Throttling**: TokenReview and SubjectAccessReview calls on every request created K8s API load
3. **In-Cluster Use Case**: The Gateway is designed for in-cluster communication, not external access
4. **Network-Level Security**: Kubernetes provides native network-level security mechanisms (Network Policies, TLS)
5. **Flexibility**: Different deployments may require different authentication mechanisms (mTLS, API keys, OAuth2)

## Decision

**Remove OAuth2 authentication and authorization middleware from the Gateway service** and adopt a **layered security approach** that leverages Kubernetes-native mechanisms and deployment-time flexibility.

### Core Principles

1. **Network-Level Security First**: Use Kubernetes Network Policies and TLS for traffic encryption
2. **Deployment-Time Flexibility**: Support sidecar containers for custom protocol-based authentication and authorization
3. **Simplicity**: Keep the Gateway service focused on signal processing, not authentication
4. **Defense-in-Depth**: Combine multiple security layers for comprehensive protection

### Security Architecture

#### Layer 1: Network Isolation (MANDATORY)
- **Kubernetes Network Policies**: Restrict traffic to authorized sources (e.g., Prometheus, monitoring namespace)
- **Namespace Isolation**: Deploy Gateway in a dedicated namespace with strict ingress rules
- **Service-Level TLS**: Encrypt traffic between client and Gateway using Kubernetes Service TLS

#### Layer 2: Transport Security (MANDATORY)
- **TLS Encryption**: All traffic MUST use TLS (either via Service TLS or reverse proxy)
- **Certificate Management**: Use cert-manager or similar for automated certificate rotation
- **Cipher Suite Configuration**: Enforce strong TLS 1.3 cipher suites

#### Layer 3: Application-Level Authentication (OPTIONAL, Deployment-Specific)
- **Sidecar Pattern**: Deploy authentication/authorization as a sidecar container
- **Protocol Flexibility**: Support mTLS, OAuth2, API keys, or custom protocols via sidecar
- **Examples**:
  - **Envoy + Authorino**: For OAuth2/OIDC authentication
  - **Istio**: For mTLS and service mesh integration
  - **Custom Sidecar**: For proprietary authentication mechanisms

### Implementation Details

#### Gateway Service Changes
1. **Remove Middleware**:
   - Delete `TokenReviewAuth` middleware
   - Delete `SubjectAccessReviewAuthz` middleware
   - Delete `DisableAuth` configuration option
   - Delete `ValidateAuthConfig` function

2. **Keep Security Middleware**:
   - **Rate Limiting**: Protect against DoS attacks (BR-GATEWAY-071)
   - **Payload Size Limit**: Prevent large request attacks (BR-GATEWAY-073)
   - **Log Sanitization**: Redact sensitive data from logs (BR-GATEWAY-074)
   - **Security Headers**: Prevent common web vulnerabilities (BR-GATEWAY-075)
   - **Timestamp Validation**: Prevent replay attacks (BR-GATEWAY-076)

3. **Simplify Server Constructor**:
   - Remove `k8sClientset` parameter (no longer needed for auth)
   - Remove `DisableAuth` from `Config` struct
   - Remove authentication-related metrics (`TokenReviewTimeouts`, `SubjectAccessReviewTimeouts`)

#### Deployment Configurations

##### Configuration 1: Network Policies + Service TLS (Recommended for In-Cluster)
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
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
```

##### Configuration 2: Sidecar Authentication (For Custom Requirements)
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
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8080
          name: http
      - name: auth-proxy
        image: envoy:v1.28
        ports:
        - containerPort: 8443
          name: https
        volumeMounts:
        - name: envoy-config
          mountPath: /etc/envoy
        - name: tls-certs
          mountPath: /etc/tls
      volumes:
      - name: envoy-config
        configMap:
          name: envoy-auth-config
      - name: tls-certs
        secret:
          secretName: gateway-tls
```

##### Configuration 3: Reverse Proxy (For External Access)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway-external
  namespace: kubernaut-system
spec:
  type: LoadBalancer
  ports:
  - name: https
    port: 443
    targetPort: 8443
    protocol: TCP
  selector:
    app: nginx-gateway-proxy
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-gateway-proxy
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.25
        volumeMounts:
        - name: nginx-config
          mountPath: /etc/nginx/nginx.conf
          subPath: nginx.conf
        - name: tls-certs
          mountPath: /etc/nginx/tls
      volumes:
      - name: nginx-config
        configMap:
          name: nginx-gateway-config
      - name: tls-certs
        secret:
          secretName: gateway-external-tls
```

## Consequences

### Positive
1. **Simplified Gateway Service**: Removes authentication complexity from core service logic
2. **Reduced K8s API Load**: No TokenReview/SubjectAccessReview calls on every request
3. **Deployment Flexibility**: Different deployments can choose appropriate authentication mechanisms
4. **Easier Testing**: Integration tests don't require complex ServiceAccount setup
5. **Better Separation of Concerns**: Authentication is a deployment concern, not a service concern
6. **Performance**: Lower latency without K8s API calls for every request

### Negative
1. **Deployment Responsibility**: Operators must configure Network Policies and TLS correctly
2. **Documentation Burden**: Need clear documentation for different deployment scenarios
3. **No Built-in Auth**: Gateway service itself doesn't enforce authentication (relies on network layer)

### Neutral
1. **Security Model Shift**: From application-level to network-level security
2. **Sidecar Complexity**: Optional sidecar authentication adds deployment complexity (but only when needed)

## Implementation Plan

### Phase 1: Remove Authentication Middleware (Immediate - v1.0)
1. Delete `pkg/gateway/middleware/auth.go`
2. Delete `pkg/gateway/middleware/authz.go`
3. Remove `k8sClientset` from `server.NewServer()`
4. Remove `DisableAuth` from `server.Config`
5. Delete `pkg/gateway/server/config_validation.go`
6. Remove authentication metrics from `pkg/gateway/metrics/factory.go`

### Phase 2: Update Tests (Immediate - v1.0)
1. Remove `SetupSecurityTokens()` from `test/integration/gateway/helpers.go`
2. Delete `test/integration/gateway/security_integration_test.go`
3. Remove `KUBERNAUT_ENV` and `KUBERNAUT_DISABLE_AUTH_CONFIRM` environment variable usage
4. Simplify `StartTestGateway()` helper

### Phase 3: Documentation (Immediate - v1.0)
1. Create `docs/deployment/gateway-security.md` with Network Policies + TLS configuration
2. Update `docs/services/stateless/gateway-service/README.md` with security architecture
3. Document deployment prerequisites and security requirements
4. Update `IMPLEMENTATION_PLAN_V2.12.md` to reflect authentication removal

### Phase 4: Deployment Tooling (DEFERRED to v2.0)
**Decision**: Defer Helm/Kustomize support until pilot deployments validate security direction.

**Rationale**:
- **Pilot Focus**: v1.0 targets pilot deployments with manual Network Policy + TLS configuration
- **Security Evolution**: Sidecar authentication patterns need real-world validation before standardization
- **Flexibility**: Manual configuration allows experimentation with different security approaches
- **Complexity**: Helm/Kustomize add maintenance burden without proven deployment patterns

**Deferred Items**:
1. Kustomize overlays for different deployment scenarios
2. Helm chart with configurable security options
3. Automated cert-manager integration
4. Sidecar authentication examples (Envoy, Istio)

**v2.0 Criteria for Re-evaluation**:
- At least 3 pilot deployments successfully running with Network Policies + TLS
- Clear understanding of common deployment patterns and requirements
- Validated need for sidecar authentication in production use cases
- Operator feedback on manual vs. automated configuration preferences

## Alternatives Considered

### Alternative 1: Keep TokenReview with Caching
**Rejected**: Still adds K8s API dependency and complexity. Caching mitigates but doesn't eliminate the issue.

### Alternative 2: API Key Authentication
**Rejected**: Requires key management, rotation, and distribution. Adds complexity without significant benefit over network-level security.

### Alternative 3: mTLS in Gateway Service
**Rejected**: Inflexible for different deployment scenarios. Better implemented as a sidecar or reverse proxy for deployments that need it.

## References

- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Kubernetes Service TLS](https://kubernetes.io/docs/concepts/services-networking/service/#ssl-support-on-aws)
- [Envoy External Authorization](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_authz_filter)
- [Istio Security](https://istio.io/latest/docs/concepts/security/)
- [cert-manager](https://cert-manager.io/)
- [DD-GATEWAY-002: Service Mandatory Dependencies](./DD-GATEWAY-002-service-mandatory-dependencies.md)
- [DD-GATEWAY-003: Redis Outage Risk Tracking Metrics](./DD-GATEWAY-003-redis-outage-metrics.md)

## Confidence Assessment

**Confidence Level**: 95%

**Justification**:
- **Industry Best Practice**: Network-level security is standard for in-cluster services (Kubernetes Network Policies, Service Mesh)
- **Proven Patterns**: Sidecar authentication is a well-established pattern (Istio, Envoy, Linkerd)
- **Reduced Complexity**: Removing authentication middleware simplifies the Gateway service significantly
- **Flexibility**: Deployment-time authentication choice allows for different security requirements
- **Performance**: Eliminates K8s API calls for every request, improving latency

**Risks**:
- **Misconfiguration**: Operators must correctly configure Network Policies and TLS (mitigated by documentation and examples)
- **Defense-in-Depth**: Relies on network layer; no application-level authentication fallback (acceptable for in-cluster use case)

**Validation**:
- Integration tests will verify Gateway functionality without authentication middleware
- Deployment examples will demonstrate secure configurations
- Performance testing will confirm latency improvements


**Status**: Approved
**Date**: 2025-10-27
**Deciders**: @jordigilh
**Related**: BR-GATEWAY-066 (Authentication), BR-GATEWAY-069 (Authorization), VULN-GATEWAY-001, VULN-GATEWAY-002

## Context

The Gateway service initially implemented OAuth2-based authentication (TokenReview) and authorization (SubjectAccessReview) middleware to secure the `/webhook/prometheus` endpoint. However, during integration testing, several challenges emerged:

1. **Testing Complexity**: Setting up ServiceAccounts, tokens, and RBAC for integration tests added significant complexity
2. **K8s API Throttling**: TokenReview and SubjectAccessReview calls on every request created K8s API load
3. **In-Cluster Use Case**: The Gateway is designed for in-cluster communication, not external access
4. **Network-Level Security**: Kubernetes provides native network-level security mechanisms (Network Policies, TLS)
5. **Flexibility**: Different deployments may require different authentication mechanisms (mTLS, API keys, OAuth2)

## Decision

**Remove OAuth2 authentication and authorization middleware from the Gateway service** and adopt a **layered security approach** that leverages Kubernetes-native mechanisms and deployment-time flexibility.

### Core Principles

1. **Network-Level Security First**: Use Kubernetes Network Policies and TLS for traffic encryption
2. **Deployment-Time Flexibility**: Support sidecar containers for custom protocol-based authentication and authorization
3. **Simplicity**: Keep the Gateway service focused on signal processing, not authentication
4. **Defense-in-Depth**: Combine multiple security layers for comprehensive protection

### Security Architecture

#### Layer 1: Network Isolation (MANDATORY)
- **Kubernetes Network Policies**: Restrict traffic to authorized sources (e.g., Prometheus, monitoring namespace)
- **Namespace Isolation**: Deploy Gateway in a dedicated namespace with strict ingress rules
- **Service-Level TLS**: Encrypt traffic between client and Gateway using Kubernetes Service TLS

#### Layer 2: Transport Security (MANDATORY)
- **TLS Encryption**: All traffic MUST use TLS (either via Service TLS or reverse proxy)
- **Certificate Management**: Use cert-manager or similar for automated certificate rotation
- **Cipher Suite Configuration**: Enforce strong TLS 1.3 cipher suites

#### Layer 3: Application-Level Authentication (OPTIONAL, Deployment-Specific)
- **Sidecar Pattern**: Deploy authentication/authorization as a sidecar container
- **Protocol Flexibility**: Support mTLS, OAuth2, API keys, or custom protocols via sidecar
- **Examples**:
  - **Envoy + Authorino**: For OAuth2/OIDC authentication
  - **Istio**: For mTLS and service mesh integration
  - **Custom Sidecar**: For proprietary authentication mechanisms

### Implementation Details

#### Gateway Service Changes
1. **Remove Middleware**:
   - Delete `TokenReviewAuth` middleware
   - Delete `SubjectAccessReviewAuthz` middleware
   - Delete `DisableAuth` configuration option
   - Delete `ValidateAuthConfig` function

2. **Keep Security Middleware**:
   - **Rate Limiting**: Protect against DoS attacks (BR-GATEWAY-071)
   - **Payload Size Limit**: Prevent large request attacks (BR-GATEWAY-073)
   - **Log Sanitization**: Redact sensitive data from logs (BR-GATEWAY-074)
   - **Security Headers**: Prevent common web vulnerabilities (BR-GATEWAY-075)
   - **Timestamp Validation**: Prevent replay attacks (BR-GATEWAY-076)

3. **Simplify Server Constructor**:
   - Remove `k8sClientset` parameter (no longer needed for auth)
   - Remove `DisableAuth` from `Config` struct
   - Remove authentication-related metrics (`TokenReviewTimeouts`, `SubjectAccessReviewTimeouts`)

#### Deployment Configurations

##### Configuration 1: Network Policies + Service TLS (Recommended for In-Cluster)
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
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8080
---
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
```

##### Configuration 2: Sidecar Authentication (For Custom Requirements)
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
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8080
          name: http
      - name: auth-proxy
        image: envoy:v1.28
        ports:
        - containerPort: 8443
          name: https
        volumeMounts:
        - name: envoy-config
          mountPath: /etc/envoy
        - name: tls-certs
          mountPath: /etc/tls
      volumes:
      - name: envoy-config
        configMap:
          name: envoy-auth-config
      - name: tls-certs
        secret:
          secretName: gateway-tls
```

##### Configuration 3: Reverse Proxy (For External Access)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway-external
  namespace: kubernaut-system
spec:
  type: LoadBalancer
  ports:
  - name: https
    port: 443
    targetPort: 8443
    protocol: TCP
  selector:
    app: nginx-gateway-proxy
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-gateway-proxy
  namespace: kubernaut-system
spec:
  template:
    spec:
      containers:
      - name: nginx
        image: nginx:1.25
        volumeMounts:
        - name: nginx-config
          mountPath: /etc/nginx/nginx.conf
          subPath: nginx.conf
        - name: tls-certs
          mountPath: /etc/nginx/tls
      volumes:
      - name: nginx-config
        configMap:
          name: nginx-gateway-config
      - name: tls-certs
        secret:
          secretName: gateway-external-tls
```

## Consequences

### Positive
1. **Simplified Gateway Service**: Removes authentication complexity from core service logic
2. **Reduced K8s API Load**: No TokenReview/SubjectAccessReview calls on every request
3. **Deployment Flexibility**: Different deployments can choose appropriate authentication mechanisms
4. **Easier Testing**: Integration tests don't require complex ServiceAccount setup
5. **Better Separation of Concerns**: Authentication is a deployment concern, not a service concern
6. **Performance**: Lower latency without K8s API calls for every request

### Negative
1. **Deployment Responsibility**: Operators must configure Network Policies and TLS correctly
2. **Documentation Burden**: Need clear documentation for different deployment scenarios
3. **No Built-in Auth**: Gateway service itself doesn't enforce authentication (relies on network layer)

### Neutral
1. **Security Model Shift**: From application-level to network-level security
2. **Sidecar Complexity**: Optional sidecar authentication adds deployment complexity (but only when needed)

## Implementation Plan

### Phase 1: Remove Authentication Middleware (Immediate - v1.0)
1. Delete `pkg/gateway/middleware/auth.go`
2. Delete `pkg/gateway/middleware/authz.go`
3. Remove `k8sClientset` from `server.NewServer()`
4. Remove `DisableAuth` from `server.Config`
5. Delete `pkg/gateway/server/config_validation.go`
6. Remove authentication metrics from `pkg/gateway/metrics/factory.go`

### Phase 2: Update Tests (Immediate - v1.0)
1. Remove `SetupSecurityTokens()` from `test/integration/gateway/helpers.go`
2. Delete `test/integration/gateway/security_integration_test.go`
3. Remove `KUBERNAUT_ENV` and `KUBERNAUT_DISABLE_AUTH_CONFIRM` environment variable usage
4. Simplify `StartTestGateway()` helper

### Phase 3: Documentation (Immediate - v1.0)
1. Create `docs/deployment/gateway-security.md` with Network Policies + TLS configuration
2. Update `docs/services/stateless/gateway-service/README.md` with security architecture
3. Document deployment prerequisites and security requirements
4. Update `IMPLEMENTATION_PLAN_V2.12.md` to reflect authentication removal

### Phase 4: Deployment Tooling (DEFERRED to v2.0)
**Decision**: Defer Helm/Kustomize support until pilot deployments validate security direction.

**Rationale**:
- **Pilot Focus**: v1.0 targets pilot deployments with manual Network Policy + TLS configuration
- **Security Evolution**: Sidecar authentication patterns need real-world validation before standardization
- **Flexibility**: Manual configuration allows experimentation with different security approaches
- **Complexity**: Helm/Kustomize add maintenance burden without proven deployment patterns

**Deferred Items**:
1. Kustomize overlays for different deployment scenarios
2. Helm chart with configurable security options
3. Automated cert-manager integration
4. Sidecar authentication examples (Envoy, Istio)

**v2.0 Criteria for Re-evaluation**:
- At least 3 pilot deployments successfully running with Network Policies + TLS
- Clear understanding of common deployment patterns and requirements
- Validated need for sidecar authentication in production use cases
- Operator feedback on manual vs. automated configuration preferences

## Alternatives Considered

### Alternative 1: Keep TokenReview with Caching
**Rejected**: Still adds K8s API dependency and complexity. Caching mitigates but doesn't eliminate the issue.

### Alternative 2: API Key Authentication
**Rejected**: Requires key management, rotation, and distribution. Adds complexity without significant benefit over network-level security.

### Alternative 3: mTLS in Gateway Service
**Rejected**: Inflexible for different deployment scenarios. Better implemented as a sidecar or reverse proxy for deployments that need it.

## References

- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [Kubernetes Service TLS](https://kubernetes.io/docs/concepts/services-networking/service/#ssl-support-on-aws)
- [Envoy External Authorization](https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/ext_authz_filter)
- [Istio Security](https://istio.io/latest/docs/concepts/security/)
- [cert-manager](https://cert-manager.io/)
- [DD-GATEWAY-002: Service Mandatory Dependencies](./DD-GATEWAY-002-service-mandatory-dependencies.md)
- [DD-GATEWAY-003: Redis Outage Risk Tracking Metrics](./DD-GATEWAY-003-redis-outage-metrics.md)

## Confidence Assessment

**Confidence Level**: 95%

**Justification**:
- **Industry Best Practice**: Network-level security is standard for in-cluster services (Kubernetes Network Policies, Service Mesh)
- **Proven Patterns**: Sidecar authentication is a well-established pattern (Istio, Envoy, Linkerd)
- **Reduced Complexity**: Removing authentication middleware simplifies the Gateway service significantly
- **Flexibility**: Deployment-time authentication choice allows for different security requirements
- **Performance**: Eliminates K8s API calls for every request, improving latency

**Risks**:
- **Misconfiguration**: Operators must correctly configure Network Policies and TLS (mitigated by documentation and examples)
- **Defense-in-Depth**: Relies on network layer; no application-level authentication fallback (acceptable for in-cluster use case)

**Validation**:
- Integration tests will verify Gateway functionality without authentication middleware
- Deployment examples will demonstrate secure configurations
- Performance testing will confirm latency improvements

