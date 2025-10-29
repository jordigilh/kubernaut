# Gateway Security Deployment Guide

**Status**: Production-Ready (v1.0)
**Last Updated**: 2025-10-27
**Design Decision**: [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md)

## Overview

The Gateway service uses **network-level security** instead of application-level authentication. This guide provides deployment configurations for securing the Gateway using Kubernetes-native mechanisms.

## Security Model

### Layered Security Approach

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: Application Auth (OPTIONAL - Sidecar)             │
│ - Envoy + Authorino (OAuth2/OIDC)                          │
│ - Istio (mTLS + Service Mesh)                              │
│ - Custom sidecar (proprietary auth)                        │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: Transport Security (MANDATORY)                    │
│ - TLS encryption (Service TLS or reverse proxy)            │
│ - Certificate management (cert-manager)                    │
│ - Strong cipher suites (TLS 1.3)                           │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: Network Isolation (MANDATORY)                     │
│ - Kubernetes Network Policies                              │
│ - Namespace isolation                                      │
│ - Ingress restrictions                                     │
└─────────────────────────────────────────────────────────────┘
```

## Configuration 1: Network Policies + Service TLS (Recommended)

### Use Case
- **In-cluster communication** (Prometheus → Gateway)
- **Production deployments** with Kubernetes-native security
- **Pilot deployments** (v1.0 focus)

### Prerequisites
- Kubernetes cluster with Network Policy support (Calico, Cilium, etc.)
- OpenShift Service Serving Certificates or cert-manager

### Step 1: Deploy Gateway Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    # OpenShift: Automatic TLS certificate generation
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
    # OR use cert-manager (see Step 2)
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
  type: ClusterIP
```

### Step 2: Configure TLS (Option A: OpenShift)

OpenShift automatically generates TLS certificates when the `service.beta.openshift.io/serving-cert-secret-name` annotation is present. No additional configuration needed.

### Step 2: Configure TLS (Option B: cert-manager)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway-tls
  namespace: kubernaut-system
spec:
  secretName: gateway-tls
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days
  subject:
    organizations:
    - kubernaut
  commonName: gateway.kubernaut-system.svc.cluster.local
  dnsNames:
  - gateway.kubernaut-system.svc
  - gateway.kubernaut-system.svc.cluster.local
  issuerRef:
    name: kubernaut-ca-issuer
    kind: ClusterIssuer
```

### Step 3: Deploy Gateway with TLS

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        env:
        - name: GATEWAY_PORT
          value: "8443"
        - name: GATEWAY_TLS_CERT
          value: "/etc/tls/tls.crt"
        - name: GATEWAY_TLS_KEY
          value: "/etc/tls/tls.key"
        - name: REDIS_ADDR
          value: "redis-gateway-ha:26379"
        - name: REDIS_MASTER_NAME
          value: "mymaster"
        volumeMounts:
        - name: tls
          mountPath: /etc/tls
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: tls
        secret:
          secretName: gateway-tls
```

### Step 4: Configure Network Policies

#### Ingress Policy (Allow Prometheus Only)

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
  # Allow traffic from Prometheus pods
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8443
  # Allow traffic from within kubernaut-system namespace (for health checks)
  - from:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 8443
```

#### Egress Policy (Allow K8s API + Redis)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-egress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Egress
  egress:
  # Allow DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    - podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow Kubernetes API access
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow Redis access (within same namespace)
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
    - protocol: TCP
      port: 26379 # Sentinel
```

### Step 5: Configure Prometheus to Use TLS

```yaml
# prometheus-config.yaml
scrape_configs:
- job_name: 'kubernaut-gateway'
  scheme: https
  tls_config:
    ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    server_name: gateway.kubernaut-system.svc
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - kubernaut-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name]
    action: keep
    regex: gateway

# AlertManager webhook configuration
alertmanager_config:
  receivers:
  - name: 'kubernaut-gateway'
    webhook_configs:
    - url: 'https://gateway.kubernaut-system.svc:8443/webhook/prometheus'
      send_resolved: true
      http_config:
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
          server_name: gateway.kubernaut-system.svc
```

## Configuration 2: Reverse Proxy (For External Access)

### Use Case
- **External Prometheus** (outside cluster)
- **Multi-cluster monitoring**
- **Edge deployments**

### Architecture

```
External Prometheus
        ▼
   HAProxy/NGINX (TLS termination)
        ▼
   Gateway Service (HTTP)
```

### HAProxy Configuration

```haproxy
# /etc/haproxy/haproxy.cfg
global
    log /dev/log local0
    log /dev/log local1 notice
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # TLS configuration
    ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    ssl-default-bind-options ssl-min-ver TLSv1.3 no-tls-tickets

defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000

frontend gateway_https
    bind *:443 ssl crt /etc/haproxy/certs/gateway.pem
    mode http
    default_backend gateway_backend

backend gateway_backend
    mode http
    balance roundrobin
    option httpchk GET /health/ready
    http-check expect status 200
    server gateway1 gateway.kubernaut-system.svc:8080 check
    server gateway2 gateway.kubernaut-system.svc:8080 check backup
```

### Deploy HAProxy as Kubernetes Service

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: haproxy-config
  namespace: kubernaut-system
data:
  haproxy.cfg: |
    # HAProxy configuration from above
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-proxy
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway-proxy
  template:
    metadata:
      labels:
        app: gateway-proxy
    spec:
      containers:
      - name: haproxy
        image: haproxy:2.8
        ports:
        - containerPort: 443
          name: https
        volumeMounts:
        - name: config
          mountPath: /usr/local/etc/haproxy
        - name: certs
          mountPath: /etc/haproxy/certs
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: haproxy-config
      - name: certs
        secret:
          secretName: gateway-external-tls
---
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
    targetPort: 443
    protocol: TCP
  selector:
    app: gateway-proxy
```

## Configuration 3: Sidecar Authentication (Future - v2.0)

### Use Case
- **Custom authentication protocols**
- **OAuth2/OIDC integration**
- **mTLS requirements**

**Status**: Deferred to v2.0 (see [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md))

**Rationale**: Pilot deployments (v1.0) use Network Policies + TLS. Sidecar patterns will be evaluated after real-world validation.

## Security Best Practices

### 1. Minimize Attack Surface
- **Deploy in dedicated namespace** (`kubernaut-system`)
- **Use least-privilege ServiceAccount** (only `create remediationrequests` permission)
- **Enable Network Policies** (deny-all by default, allow-list specific sources)

### 2. TLS Configuration
- **Use TLS 1.3** (disable TLS 1.0, 1.1, 1.2)
- **Strong cipher suites** (ECDHE-ECDSA-AES128-GCM-SHA256, ECDHE-RSA-AES128-GCM-SHA256)
- **Automated certificate rotation** (cert-manager or OpenShift Service CA)
- **Verify certificate expiration** (alert if < 15 days)

### 3. Monitoring and Alerting
- **Track rejected requests** (`gateway_requests_rejected_total` metric)
- **Monitor Redis availability** (`gateway_redis_availability_seconds` metric)
- **Alert on rate limiting** (`gateway_rate_limit_exceeded_total` metric)
- **Track CRD creation failures** (`gateway_signals_failed_total` metric)

### 4. Incident Response
- **Log all webhook requests** (with sanitized payloads)
- **Retain logs for 30 days** (compliance)
- **Enable audit logging** (Kubernetes audit for CRD creation)
- **Document runbooks** (Redis failover, Gateway restart, Network Policy troubleshooting)

## Troubleshooting

### Issue: Prometheus cannot reach Gateway

**Symptoms**: `connection refused` or `connection timeout` errors in Prometheus logs

**Diagnosis**:
```bash
# Check Network Policy
kubectl describe networkpolicy gateway-ingress -n kubernaut-system

# Check Gateway pods
kubectl get pods -n kubernaut-system -l app=gateway

# Test connectivity from Prometheus pod
kubectl exec -it prometheus-0 -n monitoring -- curl -k https://gateway.kubernaut-system.svc:8443/health
```

**Resolution**:
1. Verify Network Policy allows traffic from Prometheus namespace
2. Ensure Prometheus namespace has `monitoring: "true"` label
3. Check Gateway Service is listening on correct port (8443)

### Issue: TLS certificate errors

**Symptoms**: `x509: certificate signed by unknown authority` or `certificate has expired`

**Diagnosis**:
```bash
# Check certificate expiration
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates

# Check certificate issuer
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -issuer
```

**Resolution**:
1. Renew certificate using cert-manager or OpenShift Service CA
2. Restart Gateway pods to pick up new certificate
3. Update Prometheus `tls_config.ca_file` if CA changed

### Issue: Rate limiting blocking legitimate traffic

**Symptoms**: `429 Too Many Requests` responses, `gateway_rate_limit_exceeded_total` metric increasing

**Diagnosis**:
```bash
# Check rate limit configuration
kubectl get deployment gateway -n kubernaut-system -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="GATEWAY_RATE_LIMIT")].value}'

# Check Redis rate limit keys
kubectl exec -it redis-gateway-ha-0 -n kubernaut-system -- redis-cli KEYS "rate_limit:*"
```

**Resolution**:
1. Increase `GATEWAY_RATE_LIMIT` environment variable (default: 100 req/min)
2. Increase `GATEWAY_RATE_LIMIT_WINDOW` (default: 60s)
3. Scale Gateway horizontally (more replicas)

## References

- [DD-GATEWAY-004: Authentication Strategy](../decisions/DD-GATEWAY-004-authentication-strategy.md)
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [OpenShift Service Serving Certificates](https://docs.openshift.com/container-platform/latest/security/certificates/service-serving-certificate.html)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [HAProxy TLS Configuration](https://www.haproxy.com/documentation/hapee/latest/security/tls/)

## Changelog

### v1.0.0 (2025-10-27)
- Initial release
- Configuration 1: Network Policies + Service TLS (production-ready)
- Configuration 2: Reverse Proxy (production-ready)
- Configuration 3: Sidecar Authentication (deferred to v2.0)



**Status**: Production-Ready (v1.0)
**Last Updated**: 2025-10-27
**Design Decision**: [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md)

## Overview

The Gateway service uses **network-level security** instead of application-level authentication. This guide provides deployment configurations for securing the Gateway using Kubernetes-native mechanisms.

## Security Model

### Layered Security Approach

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: Application Auth (OPTIONAL - Sidecar)             │
│ - Envoy + Authorino (OAuth2/OIDC)                          │
│ - Istio (mTLS + Service Mesh)                              │
│ - Custom sidecar (proprietary auth)                        │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: Transport Security (MANDATORY)                    │
│ - TLS encryption (Service TLS or reverse proxy)            │
│ - Certificate management (cert-manager)                    │
│ - Strong cipher suites (TLS 1.3)                           │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: Network Isolation (MANDATORY)                     │
│ - Kubernetes Network Policies                              │
│ - Namespace isolation                                      │
│ - Ingress restrictions                                     │
└─────────────────────────────────────────────────────────────┘
```

## Configuration 1: Network Policies + Service TLS (Recommended)

### Use Case
- **In-cluster communication** (Prometheus → Gateway)
- **Production deployments** with Kubernetes-native security
- **Pilot deployments** (v1.0 focus)

### Prerequisites
- Kubernetes cluster with Network Policy support (Calico, Cilium, etc.)
- OpenShift Service Serving Certificates or cert-manager

### Step 1: Deploy Gateway Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    # OpenShift: Automatic TLS certificate generation
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
    # OR use cert-manager (see Step 2)
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
  type: ClusterIP
```

### Step 2: Configure TLS (Option A: OpenShift)

OpenShift automatically generates TLS certificates when the `service.beta.openshift.io/serving-cert-secret-name` annotation is present. No additional configuration needed.

### Step 2: Configure TLS (Option B: cert-manager)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway-tls
  namespace: kubernaut-system
spec:
  secretName: gateway-tls
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days
  subject:
    organizations:
    - kubernaut
  commonName: gateway.kubernaut-system.svc.cluster.local
  dnsNames:
  - gateway.kubernaut-system.svc
  - gateway.kubernaut-system.svc.cluster.local
  issuerRef:
    name: kubernaut-ca-issuer
    kind: ClusterIssuer
```

### Step 3: Deploy Gateway with TLS

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        env:
        - name: GATEWAY_PORT
          value: "8443"
        - name: GATEWAY_TLS_CERT
          value: "/etc/tls/tls.crt"
        - name: GATEWAY_TLS_KEY
          value: "/etc/tls/tls.key"
        - name: REDIS_ADDR
          value: "redis-gateway-ha:26379"
        - name: REDIS_MASTER_NAME
          value: "mymaster"
        volumeMounts:
        - name: tls
          mountPath: /etc/tls
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: tls
        secret:
          secretName: gateway-tls
```

### Step 4: Configure Network Policies

#### Ingress Policy (Allow Prometheus Only)

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
  # Allow traffic from Prometheus pods
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8443
  # Allow traffic from within kubernaut-system namespace (for health checks)
  - from:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 8443
```

#### Egress Policy (Allow K8s API + Redis)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-egress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Egress
  egress:
  # Allow DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    - podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow Kubernetes API access
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow Redis access (within same namespace)
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
    - protocol: TCP
      port: 26379 # Sentinel
```

### Step 5: Configure Prometheus to Use TLS

```yaml
# prometheus-config.yaml
scrape_configs:
- job_name: 'kubernaut-gateway'
  scheme: https
  tls_config:
    ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    server_name: gateway.kubernaut-system.svc
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - kubernaut-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name]
    action: keep
    regex: gateway

# AlertManager webhook configuration
alertmanager_config:
  receivers:
  - name: 'kubernaut-gateway'
    webhook_configs:
    - url: 'https://gateway.kubernaut-system.svc:8443/webhook/prometheus'
      send_resolved: true
      http_config:
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
          server_name: gateway.kubernaut-system.svc
```

## Configuration 2: Reverse Proxy (For External Access)

### Use Case
- **External Prometheus** (outside cluster)
- **Multi-cluster monitoring**
- **Edge deployments**

### Architecture

```
External Prometheus
        ▼
   HAProxy/NGINX (TLS termination)
        ▼
   Gateway Service (HTTP)
```

### HAProxy Configuration

```haproxy
# /etc/haproxy/haproxy.cfg
global
    log /dev/log local0
    log /dev/log local1 notice
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # TLS configuration
    ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    ssl-default-bind-options ssl-min-ver TLSv1.3 no-tls-tickets

defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000

frontend gateway_https
    bind *:443 ssl crt /etc/haproxy/certs/gateway.pem
    mode http
    default_backend gateway_backend

backend gateway_backend
    mode http
    balance roundrobin
    option httpchk GET /health/ready
    http-check expect status 200
    server gateway1 gateway.kubernaut-system.svc:8080 check
    server gateway2 gateway.kubernaut-system.svc:8080 check backup
```

### Deploy HAProxy as Kubernetes Service

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: haproxy-config
  namespace: kubernaut-system
data:
  haproxy.cfg: |
    # HAProxy configuration from above
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-proxy
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway-proxy
  template:
    metadata:
      labels:
        app: gateway-proxy
    spec:
      containers:
      - name: haproxy
        image: haproxy:2.8
        ports:
        - containerPort: 443
          name: https
        volumeMounts:
        - name: config
          mountPath: /usr/local/etc/haproxy
        - name: certs
          mountPath: /etc/haproxy/certs
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: haproxy-config
      - name: certs
        secret:
          secretName: gateway-external-tls
---
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
    targetPort: 443
    protocol: TCP
  selector:
    app: gateway-proxy
```

## Configuration 3: Sidecar Authentication (Future - v2.0)

### Use Case
- **Custom authentication protocols**
- **OAuth2/OIDC integration**
- **mTLS requirements**

**Status**: Deferred to v2.0 (see [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md))

**Rationale**: Pilot deployments (v1.0) use Network Policies + TLS. Sidecar patterns will be evaluated after real-world validation.

## Security Best Practices

### 1. Minimize Attack Surface
- **Deploy in dedicated namespace** (`kubernaut-system`)
- **Use least-privilege ServiceAccount** (only `create remediationrequests` permission)
- **Enable Network Policies** (deny-all by default, allow-list specific sources)

### 2. TLS Configuration
- **Use TLS 1.3** (disable TLS 1.0, 1.1, 1.2)
- **Strong cipher suites** (ECDHE-ECDSA-AES128-GCM-SHA256, ECDHE-RSA-AES128-GCM-SHA256)
- **Automated certificate rotation** (cert-manager or OpenShift Service CA)
- **Verify certificate expiration** (alert if < 15 days)

### 3. Monitoring and Alerting
- **Track rejected requests** (`gateway_requests_rejected_total` metric)
- **Monitor Redis availability** (`gateway_redis_availability_seconds` metric)
- **Alert on rate limiting** (`gateway_rate_limit_exceeded_total` metric)
- **Track CRD creation failures** (`gateway_signals_failed_total` metric)

### 4. Incident Response
- **Log all webhook requests** (with sanitized payloads)
- **Retain logs for 30 days** (compliance)
- **Enable audit logging** (Kubernetes audit for CRD creation)
- **Document runbooks** (Redis failover, Gateway restart, Network Policy troubleshooting)

## Troubleshooting

### Issue: Prometheus cannot reach Gateway

**Symptoms**: `connection refused` or `connection timeout` errors in Prometheus logs

**Diagnosis**:
```bash
# Check Network Policy
kubectl describe networkpolicy gateway-ingress -n kubernaut-system

# Check Gateway pods
kubectl get pods -n kubernaut-system -l app=gateway

# Test connectivity from Prometheus pod
kubectl exec -it prometheus-0 -n monitoring -- curl -k https://gateway.kubernaut-system.svc:8443/health
```

**Resolution**:
1. Verify Network Policy allows traffic from Prometheus namespace
2. Ensure Prometheus namespace has `monitoring: "true"` label
3. Check Gateway Service is listening on correct port (8443)

### Issue: TLS certificate errors

**Symptoms**: `x509: certificate signed by unknown authority` or `certificate has expired`

**Diagnosis**:
```bash
# Check certificate expiration
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates

# Check certificate issuer
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -issuer
```

**Resolution**:
1. Renew certificate using cert-manager or OpenShift Service CA
2. Restart Gateway pods to pick up new certificate
3. Update Prometheus `tls_config.ca_file` if CA changed

### Issue: Rate limiting blocking legitimate traffic

**Symptoms**: `429 Too Many Requests` responses, `gateway_rate_limit_exceeded_total` metric increasing

**Diagnosis**:
```bash
# Check rate limit configuration
kubectl get deployment gateway -n kubernaut-system -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="GATEWAY_RATE_LIMIT")].value}'

# Check Redis rate limit keys
kubectl exec -it redis-gateway-ha-0 -n kubernaut-system -- redis-cli KEYS "rate_limit:*"
```

**Resolution**:
1. Increase `GATEWAY_RATE_LIMIT` environment variable (default: 100 req/min)
2. Increase `GATEWAY_RATE_LIMIT_WINDOW` (default: 60s)
3. Scale Gateway horizontally (more replicas)

## References

- [DD-GATEWAY-004: Authentication Strategy](../decisions/DD-GATEWAY-004-authentication-strategy.md)
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [OpenShift Service Serving Certificates](https://docs.openshift.com/container-platform/latest/security/certificates/service-serving-certificate.html)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [HAProxy TLS Configuration](https://www.haproxy.com/documentation/hapee/latest/security/tls/)

## Changelog

### v1.0.0 (2025-10-27)
- Initial release
- Configuration 1: Network Policies + Service TLS (production-ready)
- Configuration 2: Reverse Proxy (production-ready)
- Configuration 3: Sidecar Authentication (deferred to v2.0)

# Gateway Security Deployment Guide

**Status**: Production-Ready (v1.0)
**Last Updated**: 2025-10-27
**Design Decision**: [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md)

## Overview

The Gateway service uses **network-level security** instead of application-level authentication. This guide provides deployment configurations for securing the Gateway using Kubernetes-native mechanisms.

## Security Model

### Layered Security Approach

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: Application Auth (OPTIONAL - Sidecar)             │
│ - Envoy + Authorino (OAuth2/OIDC)                          │
│ - Istio (mTLS + Service Mesh)                              │
│ - Custom sidecar (proprietary auth)                        │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: Transport Security (MANDATORY)                    │
│ - TLS encryption (Service TLS or reverse proxy)            │
│ - Certificate management (cert-manager)                    │
│ - Strong cipher suites (TLS 1.3)                           │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: Network Isolation (MANDATORY)                     │
│ - Kubernetes Network Policies                              │
│ - Namespace isolation                                      │
│ - Ingress restrictions                                     │
└─────────────────────────────────────────────────────────────┘
```

## Configuration 1: Network Policies + Service TLS (Recommended)

### Use Case
- **In-cluster communication** (Prometheus → Gateway)
- **Production deployments** with Kubernetes-native security
- **Pilot deployments** (v1.0 focus)

### Prerequisites
- Kubernetes cluster with Network Policy support (Calico, Cilium, etc.)
- OpenShift Service Serving Certificates or cert-manager

### Step 1: Deploy Gateway Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    # OpenShift: Automatic TLS certificate generation
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
    # OR use cert-manager (see Step 2)
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
  type: ClusterIP
```

### Step 2: Configure TLS (Option A: OpenShift)

OpenShift automatically generates TLS certificates when the `service.beta.openshift.io/serving-cert-secret-name` annotation is present. No additional configuration needed.

### Step 2: Configure TLS (Option B: cert-manager)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway-tls
  namespace: kubernaut-system
spec:
  secretName: gateway-tls
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days
  subject:
    organizations:
    - kubernaut
  commonName: gateway.kubernaut-system.svc.cluster.local
  dnsNames:
  - gateway.kubernaut-system.svc
  - gateway.kubernaut-system.svc.cluster.local
  issuerRef:
    name: kubernaut-ca-issuer
    kind: ClusterIssuer
```

### Step 3: Deploy Gateway with TLS

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        env:
        - name: GATEWAY_PORT
          value: "8443"
        - name: GATEWAY_TLS_CERT
          value: "/etc/tls/tls.crt"
        - name: GATEWAY_TLS_KEY
          value: "/etc/tls/tls.key"
        - name: REDIS_ADDR
          value: "redis-gateway-ha:26379"
        - name: REDIS_MASTER_NAME
          value: "mymaster"
        volumeMounts:
        - name: tls
          mountPath: /etc/tls
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: tls
        secret:
          secretName: gateway-tls
```

### Step 4: Configure Network Policies

#### Ingress Policy (Allow Prometheus Only)

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
  # Allow traffic from Prometheus pods
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8443
  # Allow traffic from within kubernaut-system namespace (for health checks)
  - from:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 8443
```

#### Egress Policy (Allow K8s API + Redis)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-egress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Egress
  egress:
  # Allow DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    - podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow Kubernetes API access
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow Redis access (within same namespace)
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
    - protocol: TCP
      port: 26379 # Sentinel
```

### Step 5: Configure Prometheus to Use TLS

```yaml
# prometheus-config.yaml
scrape_configs:
- job_name: 'kubernaut-gateway'
  scheme: https
  tls_config:
    ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    server_name: gateway.kubernaut-system.svc
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - kubernaut-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name]
    action: keep
    regex: gateway

# AlertManager webhook configuration
alertmanager_config:
  receivers:
  - name: 'kubernaut-gateway'
    webhook_configs:
    - url: 'https://gateway.kubernaut-system.svc:8443/webhook/prometheus'
      send_resolved: true
      http_config:
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
          server_name: gateway.kubernaut-system.svc
```

## Configuration 2: Reverse Proxy (For External Access)

### Use Case
- **External Prometheus** (outside cluster)
- **Multi-cluster monitoring**
- **Edge deployments**

### Architecture

```
External Prometheus
        ▼
   HAProxy/NGINX (TLS termination)
        ▼
   Gateway Service (HTTP)
```

### HAProxy Configuration

```haproxy
# /etc/haproxy/haproxy.cfg
global
    log /dev/log local0
    log /dev/log local1 notice
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # TLS configuration
    ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    ssl-default-bind-options ssl-min-ver TLSv1.3 no-tls-tickets

defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000

frontend gateway_https
    bind *:443 ssl crt /etc/haproxy/certs/gateway.pem
    mode http
    default_backend gateway_backend

backend gateway_backend
    mode http
    balance roundrobin
    option httpchk GET /health/ready
    http-check expect status 200
    server gateway1 gateway.kubernaut-system.svc:8080 check
    server gateway2 gateway.kubernaut-system.svc:8080 check backup
```

### Deploy HAProxy as Kubernetes Service

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: haproxy-config
  namespace: kubernaut-system
data:
  haproxy.cfg: |
    # HAProxy configuration from above
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-proxy
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway-proxy
  template:
    metadata:
      labels:
        app: gateway-proxy
    spec:
      containers:
      - name: haproxy
        image: haproxy:2.8
        ports:
        - containerPort: 443
          name: https
        volumeMounts:
        - name: config
          mountPath: /usr/local/etc/haproxy
        - name: certs
          mountPath: /etc/haproxy/certs
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: haproxy-config
      - name: certs
        secret:
          secretName: gateway-external-tls
---
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
    targetPort: 443
    protocol: TCP
  selector:
    app: gateway-proxy
```

## Configuration 3: Sidecar Authentication (Future - v2.0)

### Use Case
- **Custom authentication protocols**
- **OAuth2/OIDC integration**
- **mTLS requirements**

**Status**: Deferred to v2.0 (see [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md))

**Rationale**: Pilot deployments (v1.0) use Network Policies + TLS. Sidecar patterns will be evaluated after real-world validation.

## Security Best Practices

### 1. Minimize Attack Surface
- **Deploy in dedicated namespace** (`kubernaut-system`)
- **Use least-privilege ServiceAccount** (only `create remediationrequests` permission)
- **Enable Network Policies** (deny-all by default, allow-list specific sources)

### 2. TLS Configuration
- **Use TLS 1.3** (disable TLS 1.0, 1.1, 1.2)
- **Strong cipher suites** (ECDHE-ECDSA-AES128-GCM-SHA256, ECDHE-RSA-AES128-GCM-SHA256)
- **Automated certificate rotation** (cert-manager or OpenShift Service CA)
- **Verify certificate expiration** (alert if < 15 days)

### 3. Monitoring and Alerting
- **Track rejected requests** (`gateway_requests_rejected_total` metric)
- **Monitor Redis availability** (`gateway_redis_availability_seconds` metric)
- **Alert on rate limiting** (`gateway_rate_limit_exceeded_total` metric)
- **Track CRD creation failures** (`gateway_signals_failed_total` metric)

### 4. Incident Response
- **Log all webhook requests** (with sanitized payloads)
- **Retain logs for 30 days** (compliance)
- **Enable audit logging** (Kubernetes audit for CRD creation)
- **Document runbooks** (Redis failover, Gateway restart, Network Policy troubleshooting)

## Troubleshooting

### Issue: Prometheus cannot reach Gateway

**Symptoms**: `connection refused` or `connection timeout` errors in Prometheus logs

**Diagnosis**:
```bash
# Check Network Policy
kubectl describe networkpolicy gateway-ingress -n kubernaut-system

# Check Gateway pods
kubectl get pods -n kubernaut-system -l app=gateway

# Test connectivity from Prometheus pod
kubectl exec -it prometheus-0 -n monitoring -- curl -k https://gateway.kubernaut-system.svc:8443/health
```

**Resolution**:
1. Verify Network Policy allows traffic from Prometheus namespace
2. Ensure Prometheus namespace has `monitoring: "true"` label
3. Check Gateway Service is listening on correct port (8443)

### Issue: TLS certificate errors

**Symptoms**: `x509: certificate signed by unknown authority` or `certificate has expired`

**Diagnosis**:
```bash
# Check certificate expiration
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates

# Check certificate issuer
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -issuer
```

**Resolution**:
1. Renew certificate using cert-manager or OpenShift Service CA
2. Restart Gateway pods to pick up new certificate
3. Update Prometheus `tls_config.ca_file` if CA changed

### Issue: Rate limiting blocking legitimate traffic

**Symptoms**: `429 Too Many Requests` responses, `gateway_rate_limit_exceeded_total` metric increasing

**Diagnosis**:
```bash
# Check rate limit configuration
kubectl get deployment gateway -n kubernaut-system -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="GATEWAY_RATE_LIMIT")].value}'

# Check Redis rate limit keys
kubectl exec -it redis-gateway-ha-0 -n kubernaut-system -- redis-cli KEYS "rate_limit:*"
```

**Resolution**:
1. Increase `GATEWAY_RATE_LIMIT` environment variable (default: 100 req/min)
2. Increase `GATEWAY_RATE_LIMIT_WINDOW` (default: 60s)
3. Scale Gateway horizontally (more replicas)

## References

- [DD-GATEWAY-004: Authentication Strategy](../decisions/DD-GATEWAY-004-authentication-strategy.md)
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [OpenShift Service Serving Certificates](https://docs.openshift.com/container-platform/latest/security/certificates/service-serving-certificate.html)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [HAProxy TLS Configuration](https://www.haproxy.com/documentation/hapee/latest/security/tls/)

## Changelog

### v1.0.0 (2025-10-27)
- Initial release
- Configuration 1: Network Policies + Service TLS (production-ready)
- Configuration 2: Reverse Proxy (production-ready)
- Configuration 3: Sidecar Authentication (deferred to v2.0)

# Gateway Security Deployment Guide

**Status**: Production-Ready (v1.0)
**Last Updated**: 2025-10-27
**Design Decision**: [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md)

## Overview

The Gateway service uses **network-level security** instead of application-level authentication. This guide provides deployment configurations for securing the Gateway using Kubernetes-native mechanisms.

## Security Model

### Layered Security Approach

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: Application Auth (OPTIONAL - Sidecar)             │
│ - Envoy + Authorino (OAuth2/OIDC)                          │
│ - Istio (mTLS + Service Mesh)                              │
│ - Custom sidecar (proprietary auth)                        │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: Transport Security (MANDATORY)                    │
│ - TLS encryption (Service TLS or reverse proxy)            │
│ - Certificate management (cert-manager)                    │
│ - Strong cipher suites (TLS 1.3)                           │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: Network Isolation (MANDATORY)                     │
│ - Kubernetes Network Policies                              │
│ - Namespace isolation                                      │
│ - Ingress restrictions                                     │
└─────────────────────────────────────────────────────────────┘
```

## Configuration 1: Network Policies + Service TLS (Recommended)

### Use Case
- **In-cluster communication** (Prometheus → Gateway)
- **Production deployments** with Kubernetes-native security
- **Pilot deployments** (v1.0 focus)

### Prerequisites
- Kubernetes cluster with Network Policy support (Calico, Cilium, etc.)
- OpenShift Service Serving Certificates or cert-manager

### Step 1: Deploy Gateway Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    # OpenShift: Automatic TLS certificate generation
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
    # OR use cert-manager (see Step 2)
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
  type: ClusterIP
```

### Step 2: Configure TLS (Option A: OpenShift)

OpenShift automatically generates TLS certificates when the `service.beta.openshift.io/serving-cert-secret-name` annotation is present. No additional configuration needed.

### Step 2: Configure TLS (Option B: cert-manager)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway-tls
  namespace: kubernaut-system
spec:
  secretName: gateway-tls
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days
  subject:
    organizations:
    - kubernaut
  commonName: gateway.kubernaut-system.svc.cluster.local
  dnsNames:
  - gateway.kubernaut-system.svc
  - gateway.kubernaut-system.svc.cluster.local
  issuerRef:
    name: kubernaut-ca-issuer
    kind: ClusterIssuer
```

### Step 3: Deploy Gateway with TLS

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        env:
        - name: GATEWAY_PORT
          value: "8443"
        - name: GATEWAY_TLS_CERT
          value: "/etc/tls/tls.crt"
        - name: GATEWAY_TLS_KEY
          value: "/etc/tls/tls.key"
        - name: REDIS_ADDR
          value: "redis-gateway-ha:26379"
        - name: REDIS_MASTER_NAME
          value: "mymaster"
        volumeMounts:
        - name: tls
          mountPath: /etc/tls
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: tls
        secret:
          secretName: gateway-tls
```

### Step 4: Configure Network Policies

#### Ingress Policy (Allow Prometheus Only)

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
  # Allow traffic from Prometheus pods
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8443
  # Allow traffic from within kubernaut-system namespace (for health checks)
  - from:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 8443
```

#### Egress Policy (Allow K8s API + Redis)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-egress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Egress
  egress:
  # Allow DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    - podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow Kubernetes API access
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow Redis access (within same namespace)
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
    - protocol: TCP
      port: 26379 # Sentinel
```

### Step 5: Configure Prometheus to Use TLS

```yaml
# prometheus-config.yaml
scrape_configs:
- job_name: 'kubernaut-gateway'
  scheme: https
  tls_config:
    ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    server_name: gateway.kubernaut-system.svc
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - kubernaut-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name]
    action: keep
    regex: gateway

# AlertManager webhook configuration
alertmanager_config:
  receivers:
  - name: 'kubernaut-gateway'
    webhook_configs:
    - url: 'https://gateway.kubernaut-system.svc:8443/webhook/prometheus'
      send_resolved: true
      http_config:
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
          server_name: gateway.kubernaut-system.svc
```

## Configuration 2: Reverse Proxy (For External Access)

### Use Case
- **External Prometheus** (outside cluster)
- **Multi-cluster monitoring**
- **Edge deployments**

### Architecture

```
External Prometheus
        ▼
   HAProxy/NGINX (TLS termination)
        ▼
   Gateway Service (HTTP)
```

### HAProxy Configuration

```haproxy
# /etc/haproxy/haproxy.cfg
global
    log /dev/log local0
    log /dev/log local1 notice
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # TLS configuration
    ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    ssl-default-bind-options ssl-min-ver TLSv1.3 no-tls-tickets

defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000

frontend gateway_https
    bind *:443 ssl crt /etc/haproxy/certs/gateway.pem
    mode http
    default_backend gateway_backend

backend gateway_backend
    mode http
    balance roundrobin
    option httpchk GET /health/ready
    http-check expect status 200
    server gateway1 gateway.kubernaut-system.svc:8080 check
    server gateway2 gateway.kubernaut-system.svc:8080 check backup
```

### Deploy HAProxy as Kubernetes Service

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: haproxy-config
  namespace: kubernaut-system
data:
  haproxy.cfg: |
    # HAProxy configuration from above
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-proxy
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway-proxy
  template:
    metadata:
      labels:
        app: gateway-proxy
    spec:
      containers:
      - name: haproxy
        image: haproxy:2.8
        ports:
        - containerPort: 443
          name: https
        volumeMounts:
        - name: config
          mountPath: /usr/local/etc/haproxy
        - name: certs
          mountPath: /etc/haproxy/certs
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: haproxy-config
      - name: certs
        secret:
          secretName: gateway-external-tls
---
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
    targetPort: 443
    protocol: TCP
  selector:
    app: gateway-proxy
```

## Configuration 3: Sidecar Authentication (Future - v2.0)

### Use Case
- **Custom authentication protocols**
- **OAuth2/OIDC integration**
- **mTLS requirements**

**Status**: Deferred to v2.0 (see [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md))

**Rationale**: Pilot deployments (v1.0) use Network Policies + TLS. Sidecar patterns will be evaluated after real-world validation.

## Security Best Practices

### 1. Minimize Attack Surface
- **Deploy in dedicated namespace** (`kubernaut-system`)
- **Use least-privilege ServiceAccount** (only `create remediationrequests` permission)
- **Enable Network Policies** (deny-all by default, allow-list specific sources)

### 2. TLS Configuration
- **Use TLS 1.3** (disable TLS 1.0, 1.1, 1.2)
- **Strong cipher suites** (ECDHE-ECDSA-AES128-GCM-SHA256, ECDHE-RSA-AES128-GCM-SHA256)
- **Automated certificate rotation** (cert-manager or OpenShift Service CA)
- **Verify certificate expiration** (alert if < 15 days)

### 3. Monitoring and Alerting
- **Track rejected requests** (`gateway_requests_rejected_total` metric)
- **Monitor Redis availability** (`gateway_redis_availability_seconds` metric)
- **Alert on rate limiting** (`gateway_rate_limit_exceeded_total` metric)
- **Track CRD creation failures** (`gateway_signals_failed_total` metric)

### 4. Incident Response
- **Log all webhook requests** (with sanitized payloads)
- **Retain logs for 30 days** (compliance)
- **Enable audit logging** (Kubernetes audit for CRD creation)
- **Document runbooks** (Redis failover, Gateway restart, Network Policy troubleshooting)

## Troubleshooting

### Issue: Prometheus cannot reach Gateway

**Symptoms**: `connection refused` or `connection timeout` errors in Prometheus logs

**Diagnosis**:
```bash
# Check Network Policy
kubectl describe networkpolicy gateway-ingress -n kubernaut-system

# Check Gateway pods
kubectl get pods -n kubernaut-system -l app=gateway

# Test connectivity from Prometheus pod
kubectl exec -it prometheus-0 -n monitoring -- curl -k https://gateway.kubernaut-system.svc:8443/health
```

**Resolution**:
1. Verify Network Policy allows traffic from Prometheus namespace
2. Ensure Prometheus namespace has `monitoring: "true"` label
3. Check Gateway Service is listening on correct port (8443)

### Issue: TLS certificate errors

**Symptoms**: `x509: certificate signed by unknown authority` or `certificate has expired`

**Diagnosis**:
```bash
# Check certificate expiration
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates

# Check certificate issuer
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -issuer
```

**Resolution**:
1. Renew certificate using cert-manager or OpenShift Service CA
2. Restart Gateway pods to pick up new certificate
3. Update Prometheus `tls_config.ca_file` if CA changed

### Issue: Rate limiting blocking legitimate traffic

**Symptoms**: `429 Too Many Requests` responses, `gateway_rate_limit_exceeded_total` metric increasing

**Diagnosis**:
```bash
# Check rate limit configuration
kubectl get deployment gateway -n kubernaut-system -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="GATEWAY_RATE_LIMIT")].value}'

# Check Redis rate limit keys
kubectl exec -it redis-gateway-ha-0 -n kubernaut-system -- redis-cli KEYS "rate_limit:*"
```

**Resolution**:
1. Increase `GATEWAY_RATE_LIMIT` environment variable (default: 100 req/min)
2. Increase `GATEWAY_RATE_LIMIT_WINDOW` (default: 60s)
3. Scale Gateway horizontally (more replicas)

## References

- [DD-GATEWAY-004: Authentication Strategy](../decisions/DD-GATEWAY-004-authentication-strategy.md)
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [OpenShift Service Serving Certificates](https://docs.openshift.com/container-platform/latest/security/certificates/service-serving-certificate.html)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [HAProxy TLS Configuration](https://www.haproxy.com/documentation/hapee/latest/security/tls/)

## Changelog

### v1.0.0 (2025-10-27)
- Initial release
- Configuration 1: Network Policies + Service TLS (production-ready)
- Configuration 2: Reverse Proxy (production-ready)
- Configuration 3: Sidecar Authentication (deferred to v2.0)



**Status**: Production-Ready (v1.0)
**Last Updated**: 2025-10-27
**Design Decision**: [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md)

## Overview

The Gateway service uses **network-level security** instead of application-level authentication. This guide provides deployment configurations for securing the Gateway using Kubernetes-native mechanisms.

## Security Model

### Layered Security Approach

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: Application Auth (OPTIONAL - Sidecar)             │
│ - Envoy + Authorino (OAuth2/OIDC)                          │
│ - Istio (mTLS + Service Mesh)                              │
│ - Custom sidecar (proprietary auth)                        │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: Transport Security (MANDATORY)                    │
│ - TLS encryption (Service TLS or reverse proxy)            │
│ - Certificate management (cert-manager)                    │
│ - Strong cipher suites (TLS 1.3)                           │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: Network Isolation (MANDATORY)                     │
│ - Kubernetes Network Policies                              │
│ - Namespace isolation                                      │
│ - Ingress restrictions                                     │
└─────────────────────────────────────────────────────────────┘
```

## Configuration 1: Network Policies + Service TLS (Recommended)

### Use Case
- **In-cluster communication** (Prometheus → Gateway)
- **Production deployments** with Kubernetes-native security
- **Pilot deployments** (v1.0 focus)

### Prerequisites
- Kubernetes cluster with Network Policy support (Calico, Cilium, etc.)
- OpenShift Service Serving Certificates or cert-manager

### Step 1: Deploy Gateway Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    # OpenShift: Automatic TLS certificate generation
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
    # OR use cert-manager (see Step 2)
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
  type: ClusterIP
```

### Step 2: Configure TLS (Option A: OpenShift)

OpenShift automatically generates TLS certificates when the `service.beta.openshift.io/serving-cert-secret-name` annotation is present. No additional configuration needed.

### Step 2: Configure TLS (Option B: cert-manager)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway-tls
  namespace: kubernaut-system
spec:
  secretName: gateway-tls
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days
  subject:
    organizations:
    - kubernaut
  commonName: gateway.kubernaut-system.svc.cluster.local
  dnsNames:
  - gateway.kubernaut-system.svc
  - gateway.kubernaut-system.svc.cluster.local
  issuerRef:
    name: kubernaut-ca-issuer
    kind: ClusterIssuer
```

### Step 3: Deploy Gateway with TLS

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        env:
        - name: GATEWAY_PORT
          value: "8443"
        - name: GATEWAY_TLS_CERT
          value: "/etc/tls/tls.crt"
        - name: GATEWAY_TLS_KEY
          value: "/etc/tls/tls.key"
        - name: REDIS_ADDR
          value: "redis-gateway-ha:26379"
        - name: REDIS_MASTER_NAME
          value: "mymaster"
        volumeMounts:
        - name: tls
          mountPath: /etc/tls
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: tls
        secret:
          secretName: gateway-tls
```

### Step 4: Configure Network Policies

#### Ingress Policy (Allow Prometheus Only)

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
  # Allow traffic from Prometheus pods
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8443
  # Allow traffic from within kubernaut-system namespace (for health checks)
  - from:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 8443
```

#### Egress Policy (Allow K8s API + Redis)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-egress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Egress
  egress:
  # Allow DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    - podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow Kubernetes API access
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow Redis access (within same namespace)
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
    - protocol: TCP
      port: 26379 # Sentinel
```

### Step 5: Configure Prometheus to Use TLS

```yaml
# prometheus-config.yaml
scrape_configs:
- job_name: 'kubernaut-gateway'
  scheme: https
  tls_config:
    ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    server_name: gateway.kubernaut-system.svc
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - kubernaut-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name]
    action: keep
    regex: gateway

# AlertManager webhook configuration
alertmanager_config:
  receivers:
  - name: 'kubernaut-gateway'
    webhook_configs:
    - url: 'https://gateway.kubernaut-system.svc:8443/webhook/prometheus'
      send_resolved: true
      http_config:
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
          server_name: gateway.kubernaut-system.svc
```

## Configuration 2: Reverse Proxy (For External Access)

### Use Case
- **External Prometheus** (outside cluster)
- **Multi-cluster monitoring**
- **Edge deployments**

### Architecture

```
External Prometheus
        ▼
   HAProxy/NGINX (TLS termination)
        ▼
   Gateway Service (HTTP)
```

### HAProxy Configuration

```haproxy
# /etc/haproxy/haproxy.cfg
global
    log /dev/log local0
    log /dev/log local1 notice
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # TLS configuration
    ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    ssl-default-bind-options ssl-min-ver TLSv1.3 no-tls-tickets

defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000

frontend gateway_https
    bind *:443 ssl crt /etc/haproxy/certs/gateway.pem
    mode http
    default_backend gateway_backend

backend gateway_backend
    mode http
    balance roundrobin
    option httpchk GET /health/ready
    http-check expect status 200
    server gateway1 gateway.kubernaut-system.svc:8080 check
    server gateway2 gateway.kubernaut-system.svc:8080 check backup
```

### Deploy HAProxy as Kubernetes Service

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: haproxy-config
  namespace: kubernaut-system
data:
  haproxy.cfg: |
    # HAProxy configuration from above
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-proxy
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway-proxy
  template:
    metadata:
      labels:
        app: gateway-proxy
    spec:
      containers:
      - name: haproxy
        image: haproxy:2.8
        ports:
        - containerPort: 443
          name: https
        volumeMounts:
        - name: config
          mountPath: /usr/local/etc/haproxy
        - name: certs
          mountPath: /etc/haproxy/certs
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: haproxy-config
      - name: certs
        secret:
          secretName: gateway-external-tls
---
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
    targetPort: 443
    protocol: TCP
  selector:
    app: gateway-proxy
```

## Configuration 3: Sidecar Authentication (Future - v2.0)

### Use Case
- **Custom authentication protocols**
- **OAuth2/OIDC integration**
- **mTLS requirements**

**Status**: Deferred to v2.0 (see [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md))

**Rationale**: Pilot deployments (v1.0) use Network Policies + TLS. Sidecar patterns will be evaluated after real-world validation.

## Security Best Practices

### 1. Minimize Attack Surface
- **Deploy in dedicated namespace** (`kubernaut-system`)
- **Use least-privilege ServiceAccount** (only `create remediationrequests` permission)
- **Enable Network Policies** (deny-all by default, allow-list specific sources)

### 2. TLS Configuration
- **Use TLS 1.3** (disable TLS 1.0, 1.1, 1.2)
- **Strong cipher suites** (ECDHE-ECDSA-AES128-GCM-SHA256, ECDHE-RSA-AES128-GCM-SHA256)
- **Automated certificate rotation** (cert-manager or OpenShift Service CA)
- **Verify certificate expiration** (alert if < 15 days)

### 3. Monitoring and Alerting
- **Track rejected requests** (`gateway_requests_rejected_total` metric)
- **Monitor Redis availability** (`gateway_redis_availability_seconds` metric)
- **Alert on rate limiting** (`gateway_rate_limit_exceeded_total` metric)
- **Track CRD creation failures** (`gateway_signals_failed_total` metric)

### 4. Incident Response
- **Log all webhook requests** (with sanitized payloads)
- **Retain logs for 30 days** (compliance)
- **Enable audit logging** (Kubernetes audit for CRD creation)
- **Document runbooks** (Redis failover, Gateway restart, Network Policy troubleshooting)

## Troubleshooting

### Issue: Prometheus cannot reach Gateway

**Symptoms**: `connection refused` or `connection timeout` errors in Prometheus logs

**Diagnosis**:
```bash
# Check Network Policy
kubectl describe networkpolicy gateway-ingress -n kubernaut-system

# Check Gateway pods
kubectl get pods -n kubernaut-system -l app=gateway

# Test connectivity from Prometheus pod
kubectl exec -it prometheus-0 -n monitoring -- curl -k https://gateway.kubernaut-system.svc:8443/health
```

**Resolution**:
1. Verify Network Policy allows traffic from Prometheus namespace
2. Ensure Prometheus namespace has `monitoring: "true"` label
3. Check Gateway Service is listening on correct port (8443)

### Issue: TLS certificate errors

**Symptoms**: `x509: certificate signed by unknown authority` or `certificate has expired`

**Diagnosis**:
```bash
# Check certificate expiration
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates

# Check certificate issuer
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -issuer
```

**Resolution**:
1. Renew certificate using cert-manager or OpenShift Service CA
2. Restart Gateway pods to pick up new certificate
3. Update Prometheus `tls_config.ca_file` if CA changed

### Issue: Rate limiting blocking legitimate traffic

**Symptoms**: `429 Too Many Requests` responses, `gateway_rate_limit_exceeded_total` metric increasing

**Diagnosis**:
```bash
# Check rate limit configuration
kubectl get deployment gateway -n kubernaut-system -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="GATEWAY_RATE_LIMIT")].value}'

# Check Redis rate limit keys
kubectl exec -it redis-gateway-ha-0 -n kubernaut-system -- redis-cli KEYS "rate_limit:*"
```

**Resolution**:
1. Increase `GATEWAY_RATE_LIMIT` environment variable (default: 100 req/min)
2. Increase `GATEWAY_RATE_LIMIT_WINDOW` (default: 60s)
3. Scale Gateway horizontally (more replicas)

## References

- [DD-GATEWAY-004: Authentication Strategy](../decisions/DD-GATEWAY-004-authentication-strategy.md)
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [OpenShift Service Serving Certificates](https://docs.openshift.com/container-platform/latest/security/certificates/service-serving-certificate.html)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [HAProxy TLS Configuration](https://www.haproxy.com/documentation/hapee/latest/security/tls/)

## Changelog

### v1.0.0 (2025-10-27)
- Initial release
- Configuration 1: Network Policies + Service TLS (production-ready)
- Configuration 2: Reverse Proxy (production-ready)
- Configuration 3: Sidecar Authentication (deferred to v2.0)

# Gateway Security Deployment Guide

**Status**: Production-Ready (v1.0)
**Last Updated**: 2025-10-27
**Design Decision**: [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md)

## Overview

The Gateway service uses **network-level security** instead of application-level authentication. This guide provides deployment configurations for securing the Gateway using Kubernetes-native mechanisms.

## Security Model

### Layered Security Approach

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: Application Auth (OPTIONAL - Sidecar)             │
│ - Envoy + Authorino (OAuth2/OIDC)                          │
│ - Istio (mTLS + Service Mesh)                              │
│ - Custom sidecar (proprietary auth)                        │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: Transport Security (MANDATORY)                    │
│ - TLS encryption (Service TLS or reverse proxy)            │
│ - Certificate management (cert-manager)                    │
│ - Strong cipher suites (TLS 1.3)                           │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: Network Isolation (MANDATORY)                     │
│ - Kubernetes Network Policies                              │
│ - Namespace isolation                                      │
│ - Ingress restrictions                                     │
└─────────────────────────────────────────────────────────────┘
```

## Configuration 1: Network Policies + Service TLS (Recommended)

### Use Case
- **In-cluster communication** (Prometheus → Gateway)
- **Production deployments** with Kubernetes-native security
- **Pilot deployments** (v1.0 focus)

### Prerequisites
- Kubernetes cluster with Network Policy support (Calico, Cilium, etc.)
- OpenShift Service Serving Certificates or cert-manager

### Step 1: Deploy Gateway Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    # OpenShift: Automatic TLS certificate generation
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
    # OR use cert-manager (see Step 2)
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
  type: ClusterIP
```

### Step 2: Configure TLS (Option A: OpenShift)

OpenShift automatically generates TLS certificates when the `service.beta.openshift.io/serving-cert-secret-name` annotation is present. No additional configuration needed.

### Step 2: Configure TLS (Option B: cert-manager)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway-tls
  namespace: kubernaut-system
spec:
  secretName: gateway-tls
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days
  subject:
    organizations:
    - kubernaut
  commonName: gateway.kubernaut-system.svc.cluster.local
  dnsNames:
  - gateway.kubernaut-system.svc
  - gateway.kubernaut-system.svc.cluster.local
  issuerRef:
    name: kubernaut-ca-issuer
    kind: ClusterIssuer
```

### Step 3: Deploy Gateway with TLS

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        env:
        - name: GATEWAY_PORT
          value: "8443"
        - name: GATEWAY_TLS_CERT
          value: "/etc/tls/tls.crt"
        - name: GATEWAY_TLS_KEY
          value: "/etc/tls/tls.key"
        - name: REDIS_ADDR
          value: "redis-gateway-ha:26379"
        - name: REDIS_MASTER_NAME
          value: "mymaster"
        volumeMounts:
        - name: tls
          mountPath: /etc/tls
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: tls
        secret:
          secretName: gateway-tls
```

### Step 4: Configure Network Policies

#### Ingress Policy (Allow Prometheus Only)

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
  # Allow traffic from Prometheus pods
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8443
  # Allow traffic from within kubernaut-system namespace (for health checks)
  - from:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 8443
```

#### Egress Policy (Allow K8s API + Redis)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-egress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Egress
  egress:
  # Allow DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    - podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow Kubernetes API access
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow Redis access (within same namespace)
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
    - protocol: TCP
      port: 26379 # Sentinel
```

### Step 5: Configure Prometheus to Use TLS

```yaml
# prometheus-config.yaml
scrape_configs:
- job_name: 'kubernaut-gateway'
  scheme: https
  tls_config:
    ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    server_name: gateway.kubernaut-system.svc
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - kubernaut-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name]
    action: keep
    regex: gateway

# AlertManager webhook configuration
alertmanager_config:
  receivers:
  - name: 'kubernaut-gateway'
    webhook_configs:
    - url: 'https://gateway.kubernaut-system.svc:8443/webhook/prometheus'
      send_resolved: true
      http_config:
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
          server_name: gateway.kubernaut-system.svc
```

## Configuration 2: Reverse Proxy (For External Access)

### Use Case
- **External Prometheus** (outside cluster)
- **Multi-cluster monitoring**
- **Edge deployments**

### Architecture

```
External Prometheus
        ▼
   HAProxy/NGINX (TLS termination)
        ▼
   Gateway Service (HTTP)
```

### HAProxy Configuration

```haproxy
# /etc/haproxy/haproxy.cfg
global
    log /dev/log local0
    log /dev/log local1 notice
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # TLS configuration
    ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    ssl-default-bind-options ssl-min-ver TLSv1.3 no-tls-tickets

defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000

frontend gateway_https
    bind *:443 ssl crt /etc/haproxy/certs/gateway.pem
    mode http
    default_backend gateway_backend

backend gateway_backend
    mode http
    balance roundrobin
    option httpchk GET /health/ready
    http-check expect status 200
    server gateway1 gateway.kubernaut-system.svc:8080 check
    server gateway2 gateway.kubernaut-system.svc:8080 check backup
```

### Deploy HAProxy as Kubernetes Service

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: haproxy-config
  namespace: kubernaut-system
data:
  haproxy.cfg: |
    # HAProxy configuration from above
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-proxy
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway-proxy
  template:
    metadata:
      labels:
        app: gateway-proxy
    spec:
      containers:
      - name: haproxy
        image: haproxy:2.8
        ports:
        - containerPort: 443
          name: https
        volumeMounts:
        - name: config
          mountPath: /usr/local/etc/haproxy
        - name: certs
          mountPath: /etc/haproxy/certs
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: haproxy-config
      - name: certs
        secret:
          secretName: gateway-external-tls
---
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
    targetPort: 443
    protocol: TCP
  selector:
    app: gateway-proxy
```

## Configuration 3: Sidecar Authentication (Future - v2.0)

### Use Case
- **Custom authentication protocols**
- **OAuth2/OIDC integration**
- **mTLS requirements**

**Status**: Deferred to v2.0 (see [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md))

**Rationale**: Pilot deployments (v1.0) use Network Policies + TLS. Sidecar patterns will be evaluated after real-world validation.

## Security Best Practices

### 1. Minimize Attack Surface
- **Deploy in dedicated namespace** (`kubernaut-system`)
- **Use least-privilege ServiceAccount** (only `create remediationrequests` permission)
- **Enable Network Policies** (deny-all by default, allow-list specific sources)

### 2. TLS Configuration
- **Use TLS 1.3** (disable TLS 1.0, 1.1, 1.2)
- **Strong cipher suites** (ECDHE-ECDSA-AES128-GCM-SHA256, ECDHE-RSA-AES128-GCM-SHA256)
- **Automated certificate rotation** (cert-manager or OpenShift Service CA)
- **Verify certificate expiration** (alert if < 15 days)

### 3. Monitoring and Alerting
- **Track rejected requests** (`gateway_requests_rejected_total` metric)
- **Monitor Redis availability** (`gateway_redis_availability_seconds` metric)
- **Alert on rate limiting** (`gateway_rate_limit_exceeded_total` metric)
- **Track CRD creation failures** (`gateway_signals_failed_total` metric)

### 4. Incident Response
- **Log all webhook requests** (with sanitized payloads)
- **Retain logs for 30 days** (compliance)
- **Enable audit logging** (Kubernetes audit for CRD creation)
- **Document runbooks** (Redis failover, Gateway restart, Network Policy troubleshooting)

## Troubleshooting

### Issue: Prometheus cannot reach Gateway

**Symptoms**: `connection refused` or `connection timeout` errors in Prometheus logs

**Diagnosis**:
```bash
# Check Network Policy
kubectl describe networkpolicy gateway-ingress -n kubernaut-system

# Check Gateway pods
kubectl get pods -n kubernaut-system -l app=gateway

# Test connectivity from Prometheus pod
kubectl exec -it prometheus-0 -n monitoring -- curl -k https://gateway.kubernaut-system.svc:8443/health
```

**Resolution**:
1. Verify Network Policy allows traffic from Prometheus namespace
2. Ensure Prometheus namespace has `monitoring: "true"` label
3. Check Gateway Service is listening on correct port (8443)

### Issue: TLS certificate errors

**Symptoms**: `x509: certificate signed by unknown authority` or `certificate has expired`

**Diagnosis**:
```bash
# Check certificate expiration
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates

# Check certificate issuer
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -issuer
```

**Resolution**:
1. Renew certificate using cert-manager or OpenShift Service CA
2. Restart Gateway pods to pick up new certificate
3. Update Prometheus `tls_config.ca_file` if CA changed

### Issue: Rate limiting blocking legitimate traffic

**Symptoms**: `429 Too Many Requests` responses, `gateway_rate_limit_exceeded_total` metric increasing

**Diagnosis**:
```bash
# Check rate limit configuration
kubectl get deployment gateway -n kubernaut-system -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="GATEWAY_RATE_LIMIT")].value}'

# Check Redis rate limit keys
kubectl exec -it redis-gateway-ha-0 -n kubernaut-system -- redis-cli KEYS "rate_limit:*"
```

**Resolution**:
1. Increase `GATEWAY_RATE_LIMIT` environment variable (default: 100 req/min)
2. Increase `GATEWAY_RATE_LIMIT_WINDOW` (default: 60s)
3. Scale Gateway horizontally (more replicas)

## References

- [DD-GATEWAY-004: Authentication Strategy](../decisions/DD-GATEWAY-004-authentication-strategy.md)
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [OpenShift Service Serving Certificates](https://docs.openshift.com/container-platform/latest/security/certificates/service-serving-certificate.html)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [HAProxy TLS Configuration](https://www.haproxy.com/documentation/hapee/latest/security/tls/)

## Changelog

### v1.0.0 (2025-10-27)
- Initial release
- Configuration 1: Network Policies + Service TLS (production-ready)
- Configuration 2: Reverse Proxy (production-ready)
- Configuration 3: Sidecar Authentication (deferred to v2.0)

# Gateway Security Deployment Guide

**Status**: Production-Ready (v1.0)
**Last Updated**: 2025-10-27
**Design Decision**: [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md)

## Overview

The Gateway service uses **network-level security** instead of application-level authentication. This guide provides deployment configurations for securing the Gateway using Kubernetes-native mechanisms.

## Security Model

### Layered Security Approach

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: Application Auth (OPTIONAL - Sidecar)             │
│ - Envoy + Authorino (OAuth2/OIDC)                          │
│ - Istio (mTLS + Service Mesh)                              │
│ - Custom sidecar (proprietary auth)                        │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: Transport Security (MANDATORY)                    │
│ - TLS encryption (Service TLS or reverse proxy)            │
│ - Certificate management (cert-manager)                    │
│ - Strong cipher suites (TLS 1.3)                           │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: Network Isolation (MANDATORY)                     │
│ - Kubernetes Network Policies                              │
│ - Namespace isolation                                      │
│ - Ingress restrictions                                     │
└─────────────────────────────────────────────────────────────┘
```

## Configuration 1: Network Policies + Service TLS (Recommended)

### Use Case
- **In-cluster communication** (Prometheus → Gateway)
- **Production deployments** with Kubernetes-native security
- **Pilot deployments** (v1.0 focus)

### Prerequisites
- Kubernetes cluster with Network Policy support (Calico, Cilium, etc.)
- OpenShift Service Serving Certificates or cert-manager

### Step 1: Deploy Gateway Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    # OpenShift: Automatic TLS certificate generation
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
    # OR use cert-manager (see Step 2)
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
  type: ClusterIP
```

### Step 2: Configure TLS (Option A: OpenShift)

OpenShift automatically generates TLS certificates when the `service.beta.openshift.io/serving-cert-secret-name` annotation is present. No additional configuration needed.

### Step 2: Configure TLS (Option B: cert-manager)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway-tls
  namespace: kubernaut-system
spec:
  secretName: gateway-tls
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days
  subject:
    organizations:
    - kubernaut
  commonName: gateway.kubernaut-system.svc.cluster.local
  dnsNames:
  - gateway.kubernaut-system.svc
  - gateway.kubernaut-system.svc.cluster.local
  issuerRef:
    name: kubernaut-ca-issuer
    kind: ClusterIssuer
```

### Step 3: Deploy Gateway with TLS

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        env:
        - name: GATEWAY_PORT
          value: "8443"
        - name: GATEWAY_TLS_CERT
          value: "/etc/tls/tls.crt"
        - name: GATEWAY_TLS_KEY
          value: "/etc/tls/tls.key"
        - name: REDIS_ADDR
          value: "redis-gateway-ha:26379"
        - name: REDIS_MASTER_NAME
          value: "mymaster"
        volumeMounts:
        - name: tls
          mountPath: /etc/tls
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: tls
        secret:
          secretName: gateway-tls
```

### Step 4: Configure Network Policies

#### Ingress Policy (Allow Prometheus Only)

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
  # Allow traffic from Prometheus pods
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8443
  # Allow traffic from within kubernaut-system namespace (for health checks)
  - from:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 8443
```

#### Egress Policy (Allow K8s API + Redis)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-egress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Egress
  egress:
  # Allow DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    - podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow Kubernetes API access
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow Redis access (within same namespace)
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
    - protocol: TCP
      port: 26379 # Sentinel
```

### Step 5: Configure Prometheus to Use TLS

```yaml
# prometheus-config.yaml
scrape_configs:
- job_name: 'kubernaut-gateway'
  scheme: https
  tls_config:
    ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    server_name: gateway.kubernaut-system.svc
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - kubernaut-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name]
    action: keep
    regex: gateway

# AlertManager webhook configuration
alertmanager_config:
  receivers:
  - name: 'kubernaut-gateway'
    webhook_configs:
    - url: 'https://gateway.kubernaut-system.svc:8443/webhook/prometheus'
      send_resolved: true
      http_config:
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
          server_name: gateway.kubernaut-system.svc
```

## Configuration 2: Reverse Proxy (For External Access)

### Use Case
- **External Prometheus** (outside cluster)
- **Multi-cluster monitoring**
- **Edge deployments**

### Architecture

```
External Prometheus
        ▼
   HAProxy/NGINX (TLS termination)
        ▼
   Gateway Service (HTTP)
```

### HAProxy Configuration

```haproxy
# /etc/haproxy/haproxy.cfg
global
    log /dev/log local0
    log /dev/log local1 notice
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # TLS configuration
    ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    ssl-default-bind-options ssl-min-ver TLSv1.3 no-tls-tickets

defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000

frontend gateway_https
    bind *:443 ssl crt /etc/haproxy/certs/gateway.pem
    mode http
    default_backend gateway_backend

backend gateway_backend
    mode http
    balance roundrobin
    option httpchk GET /health/ready
    http-check expect status 200
    server gateway1 gateway.kubernaut-system.svc:8080 check
    server gateway2 gateway.kubernaut-system.svc:8080 check backup
```

### Deploy HAProxy as Kubernetes Service

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: haproxy-config
  namespace: kubernaut-system
data:
  haproxy.cfg: |
    # HAProxy configuration from above
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-proxy
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway-proxy
  template:
    metadata:
      labels:
        app: gateway-proxy
    spec:
      containers:
      - name: haproxy
        image: haproxy:2.8
        ports:
        - containerPort: 443
          name: https
        volumeMounts:
        - name: config
          mountPath: /usr/local/etc/haproxy
        - name: certs
          mountPath: /etc/haproxy/certs
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: haproxy-config
      - name: certs
        secret:
          secretName: gateway-external-tls
---
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
    targetPort: 443
    protocol: TCP
  selector:
    app: gateway-proxy
```

## Configuration 3: Sidecar Authentication (Future - v2.0)

### Use Case
- **Custom authentication protocols**
- **OAuth2/OIDC integration**
- **mTLS requirements**

**Status**: Deferred to v2.0 (see [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md))

**Rationale**: Pilot deployments (v1.0) use Network Policies + TLS. Sidecar patterns will be evaluated after real-world validation.

## Security Best Practices

### 1. Minimize Attack Surface
- **Deploy in dedicated namespace** (`kubernaut-system`)
- **Use least-privilege ServiceAccount** (only `create remediationrequests` permission)
- **Enable Network Policies** (deny-all by default, allow-list specific sources)

### 2. TLS Configuration
- **Use TLS 1.3** (disable TLS 1.0, 1.1, 1.2)
- **Strong cipher suites** (ECDHE-ECDSA-AES128-GCM-SHA256, ECDHE-RSA-AES128-GCM-SHA256)
- **Automated certificate rotation** (cert-manager or OpenShift Service CA)
- **Verify certificate expiration** (alert if < 15 days)

### 3. Monitoring and Alerting
- **Track rejected requests** (`gateway_requests_rejected_total` metric)
- **Monitor Redis availability** (`gateway_redis_availability_seconds` metric)
- **Alert on rate limiting** (`gateway_rate_limit_exceeded_total` metric)
- **Track CRD creation failures** (`gateway_signals_failed_total` metric)

### 4. Incident Response
- **Log all webhook requests** (with sanitized payloads)
- **Retain logs for 30 days** (compliance)
- **Enable audit logging** (Kubernetes audit for CRD creation)
- **Document runbooks** (Redis failover, Gateway restart, Network Policy troubleshooting)

## Troubleshooting

### Issue: Prometheus cannot reach Gateway

**Symptoms**: `connection refused` or `connection timeout` errors in Prometheus logs

**Diagnosis**:
```bash
# Check Network Policy
kubectl describe networkpolicy gateway-ingress -n kubernaut-system

# Check Gateway pods
kubectl get pods -n kubernaut-system -l app=gateway

# Test connectivity from Prometheus pod
kubectl exec -it prometheus-0 -n monitoring -- curl -k https://gateway.kubernaut-system.svc:8443/health
```

**Resolution**:
1. Verify Network Policy allows traffic from Prometheus namespace
2. Ensure Prometheus namespace has `monitoring: "true"` label
3. Check Gateway Service is listening on correct port (8443)

### Issue: TLS certificate errors

**Symptoms**: `x509: certificate signed by unknown authority` or `certificate has expired`

**Diagnosis**:
```bash
# Check certificate expiration
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates

# Check certificate issuer
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -issuer
```

**Resolution**:
1. Renew certificate using cert-manager or OpenShift Service CA
2. Restart Gateway pods to pick up new certificate
3. Update Prometheus `tls_config.ca_file` if CA changed

### Issue: Rate limiting blocking legitimate traffic

**Symptoms**: `429 Too Many Requests` responses, `gateway_rate_limit_exceeded_total` metric increasing

**Diagnosis**:
```bash
# Check rate limit configuration
kubectl get deployment gateway -n kubernaut-system -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="GATEWAY_RATE_LIMIT")].value}'

# Check Redis rate limit keys
kubectl exec -it redis-gateway-ha-0 -n kubernaut-system -- redis-cli KEYS "rate_limit:*"
```

**Resolution**:
1. Increase `GATEWAY_RATE_LIMIT` environment variable (default: 100 req/min)
2. Increase `GATEWAY_RATE_LIMIT_WINDOW` (default: 60s)
3. Scale Gateway horizontally (more replicas)

## References

- [DD-GATEWAY-004: Authentication Strategy](../decisions/DD-GATEWAY-004-authentication-strategy.md)
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [OpenShift Service Serving Certificates](https://docs.openshift.com/container-platform/latest/security/certificates/service-serving-certificate.html)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [HAProxy TLS Configuration](https://www.haproxy.com/documentation/hapee/latest/security/tls/)

## Changelog

### v1.0.0 (2025-10-27)
- Initial release
- Configuration 1: Network Policies + Service TLS (production-ready)
- Configuration 2: Reverse Proxy (production-ready)
- Configuration 3: Sidecar Authentication (deferred to v2.0)



**Status**: Production-Ready (v1.0)
**Last Updated**: 2025-10-27
**Design Decision**: [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md)

## Overview

The Gateway service uses **network-level security** instead of application-level authentication. This guide provides deployment configurations for securing the Gateway using Kubernetes-native mechanisms.

## Security Model

### Layered Security Approach

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: Application Auth (OPTIONAL - Sidecar)             │
│ - Envoy + Authorino (OAuth2/OIDC)                          │
│ - Istio (mTLS + Service Mesh)                              │
│ - Custom sidecar (proprietary auth)                        │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: Transport Security (MANDATORY)                    │
│ - TLS encryption (Service TLS or reverse proxy)            │
│ - Certificate management (cert-manager)                    │
│ - Strong cipher suites (TLS 1.3)                           │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: Network Isolation (MANDATORY)                     │
│ - Kubernetes Network Policies                              │
│ - Namespace isolation                                      │
│ - Ingress restrictions                                     │
└─────────────────────────────────────────────────────────────┘
```

## Configuration 1: Network Policies + Service TLS (Recommended)

### Use Case
- **In-cluster communication** (Prometheus → Gateway)
- **Production deployments** with Kubernetes-native security
- **Pilot deployments** (v1.0 focus)

### Prerequisites
- Kubernetes cluster with Network Policy support (Calico, Cilium, etc.)
- OpenShift Service Serving Certificates or cert-manager

### Step 1: Deploy Gateway Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    # OpenShift: Automatic TLS certificate generation
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
    # OR use cert-manager (see Step 2)
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
  type: ClusterIP
```

### Step 2: Configure TLS (Option A: OpenShift)

OpenShift automatically generates TLS certificates when the `service.beta.openshift.io/serving-cert-secret-name` annotation is present. No additional configuration needed.

### Step 2: Configure TLS (Option B: cert-manager)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway-tls
  namespace: kubernaut-system
spec:
  secretName: gateway-tls
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days
  subject:
    organizations:
    - kubernaut
  commonName: gateway.kubernaut-system.svc.cluster.local
  dnsNames:
  - gateway.kubernaut-system.svc
  - gateway.kubernaut-system.svc.cluster.local
  issuerRef:
    name: kubernaut-ca-issuer
    kind: ClusterIssuer
```

### Step 3: Deploy Gateway with TLS

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        env:
        - name: GATEWAY_PORT
          value: "8443"
        - name: GATEWAY_TLS_CERT
          value: "/etc/tls/tls.crt"
        - name: GATEWAY_TLS_KEY
          value: "/etc/tls/tls.key"
        - name: REDIS_ADDR
          value: "redis-gateway-ha:26379"
        - name: REDIS_MASTER_NAME
          value: "mymaster"
        volumeMounts:
        - name: tls
          mountPath: /etc/tls
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: tls
        secret:
          secretName: gateway-tls
```

### Step 4: Configure Network Policies

#### Ingress Policy (Allow Prometheus Only)

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
  # Allow traffic from Prometheus pods
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8443
  # Allow traffic from within kubernaut-system namespace (for health checks)
  - from:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 8443
```

#### Egress Policy (Allow K8s API + Redis)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-egress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Egress
  egress:
  # Allow DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    - podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow Kubernetes API access
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow Redis access (within same namespace)
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
    - protocol: TCP
      port: 26379 # Sentinel
```

### Step 5: Configure Prometheus to Use TLS

```yaml
# prometheus-config.yaml
scrape_configs:
- job_name: 'kubernaut-gateway'
  scheme: https
  tls_config:
    ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    server_name: gateway.kubernaut-system.svc
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - kubernaut-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name]
    action: keep
    regex: gateway

# AlertManager webhook configuration
alertmanager_config:
  receivers:
  - name: 'kubernaut-gateway'
    webhook_configs:
    - url: 'https://gateway.kubernaut-system.svc:8443/webhook/prometheus'
      send_resolved: true
      http_config:
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
          server_name: gateway.kubernaut-system.svc
```

## Configuration 2: Reverse Proxy (For External Access)

### Use Case
- **External Prometheus** (outside cluster)
- **Multi-cluster monitoring**
- **Edge deployments**

### Architecture

```
External Prometheus
        ▼
   HAProxy/NGINX (TLS termination)
        ▼
   Gateway Service (HTTP)
```

### HAProxy Configuration

```haproxy
# /etc/haproxy/haproxy.cfg
global
    log /dev/log local0
    log /dev/log local1 notice
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # TLS configuration
    ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    ssl-default-bind-options ssl-min-ver TLSv1.3 no-tls-tickets

defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000

frontend gateway_https
    bind *:443 ssl crt /etc/haproxy/certs/gateway.pem
    mode http
    default_backend gateway_backend

backend gateway_backend
    mode http
    balance roundrobin
    option httpchk GET /health/ready
    http-check expect status 200
    server gateway1 gateway.kubernaut-system.svc:8080 check
    server gateway2 gateway.kubernaut-system.svc:8080 check backup
```

### Deploy HAProxy as Kubernetes Service

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: haproxy-config
  namespace: kubernaut-system
data:
  haproxy.cfg: |
    # HAProxy configuration from above
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-proxy
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway-proxy
  template:
    metadata:
      labels:
        app: gateway-proxy
    spec:
      containers:
      - name: haproxy
        image: haproxy:2.8
        ports:
        - containerPort: 443
          name: https
        volumeMounts:
        - name: config
          mountPath: /usr/local/etc/haproxy
        - name: certs
          mountPath: /etc/haproxy/certs
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: haproxy-config
      - name: certs
        secret:
          secretName: gateway-external-tls
---
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
    targetPort: 443
    protocol: TCP
  selector:
    app: gateway-proxy
```

## Configuration 3: Sidecar Authentication (Future - v2.0)

### Use Case
- **Custom authentication protocols**
- **OAuth2/OIDC integration**
- **mTLS requirements**

**Status**: Deferred to v2.0 (see [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md))

**Rationale**: Pilot deployments (v1.0) use Network Policies + TLS. Sidecar patterns will be evaluated after real-world validation.

## Security Best Practices

### 1. Minimize Attack Surface
- **Deploy in dedicated namespace** (`kubernaut-system`)
- **Use least-privilege ServiceAccount** (only `create remediationrequests` permission)
- **Enable Network Policies** (deny-all by default, allow-list specific sources)

### 2. TLS Configuration
- **Use TLS 1.3** (disable TLS 1.0, 1.1, 1.2)
- **Strong cipher suites** (ECDHE-ECDSA-AES128-GCM-SHA256, ECDHE-RSA-AES128-GCM-SHA256)
- **Automated certificate rotation** (cert-manager or OpenShift Service CA)
- **Verify certificate expiration** (alert if < 15 days)

### 3. Monitoring and Alerting
- **Track rejected requests** (`gateway_requests_rejected_total` metric)
- **Monitor Redis availability** (`gateway_redis_availability_seconds` metric)
- **Alert on rate limiting** (`gateway_rate_limit_exceeded_total` metric)
- **Track CRD creation failures** (`gateway_signals_failed_total` metric)

### 4. Incident Response
- **Log all webhook requests** (with sanitized payloads)
- **Retain logs for 30 days** (compliance)
- **Enable audit logging** (Kubernetes audit for CRD creation)
- **Document runbooks** (Redis failover, Gateway restart, Network Policy troubleshooting)

## Troubleshooting

### Issue: Prometheus cannot reach Gateway

**Symptoms**: `connection refused` or `connection timeout` errors in Prometheus logs

**Diagnosis**:
```bash
# Check Network Policy
kubectl describe networkpolicy gateway-ingress -n kubernaut-system

# Check Gateway pods
kubectl get pods -n kubernaut-system -l app=gateway

# Test connectivity from Prometheus pod
kubectl exec -it prometheus-0 -n monitoring -- curl -k https://gateway.kubernaut-system.svc:8443/health
```

**Resolution**:
1. Verify Network Policy allows traffic from Prometheus namespace
2. Ensure Prometheus namespace has `monitoring: "true"` label
3. Check Gateway Service is listening on correct port (8443)

### Issue: TLS certificate errors

**Symptoms**: `x509: certificate signed by unknown authority` or `certificate has expired`

**Diagnosis**:
```bash
# Check certificate expiration
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates

# Check certificate issuer
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -issuer
```

**Resolution**:
1. Renew certificate using cert-manager or OpenShift Service CA
2. Restart Gateway pods to pick up new certificate
3. Update Prometheus `tls_config.ca_file` if CA changed

### Issue: Rate limiting blocking legitimate traffic

**Symptoms**: `429 Too Many Requests` responses, `gateway_rate_limit_exceeded_total` metric increasing

**Diagnosis**:
```bash
# Check rate limit configuration
kubectl get deployment gateway -n kubernaut-system -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="GATEWAY_RATE_LIMIT")].value}'

# Check Redis rate limit keys
kubectl exec -it redis-gateway-ha-0 -n kubernaut-system -- redis-cli KEYS "rate_limit:*"
```

**Resolution**:
1. Increase `GATEWAY_RATE_LIMIT` environment variable (default: 100 req/min)
2. Increase `GATEWAY_RATE_LIMIT_WINDOW` (default: 60s)
3. Scale Gateway horizontally (more replicas)

## References

- [DD-GATEWAY-004: Authentication Strategy](../decisions/DD-GATEWAY-004-authentication-strategy.md)
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [OpenShift Service Serving Certificates](https://docs.openshift.com/container-platform/latest/security/certificates/service-serving-certificate.html)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [HAProxy TLS Configuration](https://www.haproxy.com/documentation/hapee/latest/security/tls/)

## Changelog

### v1.0.0 (2025-10-27)
- Initial release
- Configuration 1: Network Policies + Service TLS (production-ready)
- Configuration 2: Reverse Proxy (production-ready)
- Configuration 3: Sidecar Authentication (deferred to v2.0)

# Gateway Security Deployment Guide

**Status**: Production-Ready (v1.0)
**Last Updated**: 2025-10-27
**Design Decision**: [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md)

## Overview

The Gateway service uses **network-level security** instead of application-level authentication. This guide provides deployment configurations for securing the Gateway using Kubernetes-native mechanisms.

## Security Model

### Layered Security Approach

```
┌─────────────────────────────────────────────────────────────┐
│ Layer 3: Application Auth (OPTIONAL - Sidecar)             │
│ - Envoy + Authorino (OAuth2/OIDC)                          │
│ - Istio (mTLS + Service Mesh)                              │
│ - Custom sidecar (proprietary auth)                        │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 2: Transport Security (MANDATORY)                    │
│ - TLS encryption (Service TLS or reverse proxy)            │
│ - Certificate management (cert-manager)                    │
│ - Strong cipher suites (TLS 1.3)                           │
└─────────────────────────────────────────────────────────────┘
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ Layer 1: Network Isolation (MANDATORY)                     │
│ - Kubernetes Network Policies                              │
│ - Namespace isolation                                      │
│ - Ingress restrictions                                     │
└─────────────────────────────────────────────────────────────┘
```

## Configuration 1: Network Policies + Service TLS (Recommended)

### Use Case
- **In-cluster communication** (Prometheus → Gateway)
- **Production deployments** with Kubernetes-native security
- **Pilot deployments** (v1.0 focus)

### Prerequisites
- Kubernetes cluster with Network Policy support (Calico, Cilium, etc.)
- OpenShift Service Serving Certificates or cert-manager

### Step 1: Deploy Gateway Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: gateway
  namespace: kubernaut-system
  annotations:
    # OpenShift: Automatic TLS certificate generation
    service.beta.openshift.io/serving-cert-secret-name: gateway-tls
    # OR use cert-manager (see Step 2)
spec:
  ports:
  - name: https
    port: 8443
    targetPort: 8443
    protocol: TCP
  selector:
    app: gateway
  type: ClusterIP
```

### Step 2: Configure TLS (Option A: OpenShift)

OpenShift automatically generates TLS certificates when the `service.beta.openshift.io/serving-cert-secret-name` annotation is present. No additional configuration needed.

### Step 2: Configure TLS (Option B: cert-manager)

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: gateway-tls
  namespace: kubernaut-system
spec:
  secretName: gateway-tls
  duration: 2160h # 90 days
  renewBefore: 360h # 15 days
  subject:
    organizations:
    - kubernaut
  commonName: gateway.kubernaut-system.svc.cluster.local
  dnsNames:
  - gateway.kubernaut-system.svc
  - gateway.kubernaut-system.svc.cluster.local
  issuerRef:
    name: kubernaut-ca-issuer
    kind: ClusterIssuer
```

### Step 3: Deploy Gateway with TLS

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway
  template:
    metadata:
      labels:
        app: gateway
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8443
          name: https
          protocol: TCP
        env:
        - name: GATEWAY_PORT
          value: "8443"
        - name: GATEWAY_TLS_CERT
          value: "/etc/tls/tls.crt"
        - name: GATEWAY_TLS_KEY
          value: "/etc/tls/tls.key"
        - name: REDIS_ADDR
          value: "redis-gateway-ha:26379"
        - name: REDIS_MASTER_NAME
          value: "mymaster"
        volumeMounts:
        - name: tls
          mountPath: /etc/tls
          readOnly: true
        resources:
          requests:
            memory: "256Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 10
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8443
            scheme: HTTPS
          initialDelaySeconds: 5
          periodSeconds: 5
      volumes:
      - name: tls
        secret:
          secretName: gateway-tls
```

### Step 4: Configure Network Policies

#### Ingress Policy (Allow Prometheus Only)

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
  # Allow traffic from Prometheus pods
  - from:
    - namespaceSelector:
        matchLabels:
          monitoring: "true"
    - podSelector:
        matchLabels:
          app: prometheus
    ports:
    - protocol: TCP
      port: 8443
  # Allow traffic from within kubernaut-system namespace (for health checks)
  - from:
    - podSelector: {}
    ports:
    - protocol: TCP
      port: 8443
```

#### Egress Policy (Allow K8s API + Redis)

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: gateway-egress
  namespace: kubernaut-system
spec:
  podSelector:
    matchLabels:
      app: gateway
  policyTypes:
  - Egress
  egress:
  # Allow DNS resolution
  - to:
    - namespaceSelector:
        matchLabels:
          name: kube-system
    - podSelector:
        matchLabels:
          k8s-app: kube-dns
    ports:
    - protocol: UDP
      port: 53
  # Allow Kubernetes API access
  - to:
    - namespaceSelector: {}
      podSelector:
        matchLabels:
          component: apiserver
    ports:
    - protocol: TCP
      port: 6443
  # Allow Redis access (within same namespace)
  - to:
    - podSelector:
        matchLabels:
          app: redis
    ports:
    - protocol: TCP
      port: 6379
    - protocol: TCP
      port: 26379 # Sentinel
```

### Step 5: Configure Prometheus to Use TLS

```yaml
# prometheus-config.yaml
scrape_configs:
- job_name: 'kubernaut-gateway'
  scheme: https
  tls_config:
    ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
    server_name: gateway.kubernaut-system.svc
  kubernetes_sd_configs:
  - role: endpoints
    namespaces:
      names:
      - kubernaut-system
  relabel_configs:
  - source_labels: [__meta_kubernetes_service_name]
    action: keep
    regex: gateway

# AlertManager webhook configuration
alertmanager_config:
  receivers:
  - name: 'kubernaut-gateway'
    webhook_configs:
    - url: 'https://gateway.kubernaut-system.svc:8443/webhook/prometheus'
      send_resolved: true
      http_config:
        tls_config:
          ca_file: /var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt
          server_name: gateway.kubernaut-system.svc
```

## Configuration 2: Reverse Proxy (For External Access)

### Use Case
- **External Prometheus** (outside cluster)
- **Multi-cluster monitoring**
- **Edge deployments**

### Architecture

```
External Prometheus
        ▼
   HAProxy/NGINX (TLS termination)
        ▼
   Gateway Service (HTTP)
```

### HAProxy Configuration

```haproxy
# /etc/haproxy/haproxy.cfg
global
    log /dev/log local0
    log /dev/log local1 notice
    chroot /var/lib/haproxy
    stats socket /run/haproxy/admin.sock mode 660 level admin
    stats timeout 30s
    user haproxy
    group haproxy
    daemon

    # TLS configuration
    ssl-default-bind-ciphers ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384
    ssl-default-bind-options ssl-min-ver TLSv1.3 no-tls-tickets

defaults
    log     global
    mode    http
    option  httplog
    option  dontlognull
    timeout connect 5000
    timeout client  50000
    timeout server  50000

frontend gateway_https
    bind *:443 ssl crt /etc/haproxy/certs/gateway.pem
    mode http
    default_backend gateway_backend

backend gateway_backend
    mode http
    balance roundrobin
    option httpchk GET /health/ready
    http-check expect status 200
    server gateway1 gateway.kubernaut-system.svc:8080 check
    server gateway2 gateway.kubernaut-system.svc:8080 check backup
```

### Deploy HAProxy as Kubernetes Service

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: haproxy-config
  namespace: kubernaut-system
data:
  haproxy.cfg: |
    # HAProxy configuration from above
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway-proxy
  namespace: kubernaut-system
spec:
  replicas: 2
  selector:
    matchLabels:
      app: gateway-proxy
  template:
    metadata:
      labels:
        app: gateway-proxy
    spec:
      containers:
      - name: haproxy
        image: haproxy:2.8
        ports:
        - containerPort: 443
          name: https
        volumeMounts:
        - name: config
          mountPath: /usr/local/etc/haproxy
        - name: certs
          mountPath: /etc/haproxy/certs
          readOnly: true
      volumes:
      - name: config
        configMap:
          name: haproxy-config
      - name: certs
        secret:
          secretName: gateway-external-tls
---
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
    targetPort: 443
    protocol: TCP
  selector:
    app: gateway-proxy
```

## Configuration 3: Sidecar Authentication (Future - v2.0)

### Use Case
- **Custom authentication protocols**
- **OAuth2/OIDC integration**
- **mTLS requirements**

**Status**: Deferred to v2.0 (see [DD-GATEWAY-004](../decisions/DD-GATEWAY-004-authentication-strategy.md))

**Rationale**: Pilot deployments (v1.0) use Network Policies + TLS. Sidecar patterns will be evaluated after real-world validation.

## Security Best Practices

### 1. Minimize Attack Surface
- **Deploy in dedicated namespace** (`kubernaut-system`)
- **Use least-privilege ServiceAccount** (only `create remediationrequests` permission)
- **Enable Network Policies** (deny-all by default, allow-list specific sources)

### 2. TLS Configuration
- **Use TLS 1.3** (disable TLS 1.0, 1.1, 1.2)
- **Strong cipher suites** (ECDHE-ECDSA-AES128-GCM-SHA256, ECDHE-RSA-AES128-GCM-SHA256)
- **Automated certificate rotation** (cert-manager or OpenShift Service CA)
- **Verify certificate expiration** (alert if < 15 days)

### 3. Monitoring and Alerting
- **Track rejected requests** (`gateway_requests_rejected_total` metric)
- **Monitor Redis availability** (`gateway_redis_availability_seconds` metric)
- **Alert on rate limiting** (`gateway_rate_limit_exceeded_total` metric)
- **Track CRD creation failures** (`gateway_signals_failed_total` metric)

### 4. Incident Response
- **Log all webhook requests** (with sanitized payloads)
- **Retain logs for 30 days** (compliance)
- **Enable audit logging** (Kubernetes audit for CRD creation)
- **Document runbooks** (Redis failover, Gateway restart, Network Policy troubleshooting)

## Troubleshooting

### Issue: Prometheus cannot reach Gateway

**Symptoms**: `connection refused` or `connection timeout` errors in Prometheus logs

**Diagnosis**:
```bash
# Check Network Policy
kubectl describe networkpolicy gateway-ingress -n kubernaut-system

# Check Gateway pods
kubectl get pods -n kubernaut-system -l app=gateway

# Test connectivity from Prometheus pod
kubectl exec -it prometheus-0 -n monitoring -- curl -k https://gateway.kubernaut-system.svc:8443/health
```

**Resolution**:
1. Verify Network Policy allows traffic from Prometheus namespace
2. Ensure Prometheus namespace has `monitoring: "true"` label
3. Check Gateway Service is listening on correct port (8443)

### Issue: TLS certificate errors

**Symptoms**: `x509: certificate signed by unknown authority` or `certificate has expired`

**Diagnosis**:
```bash
# Check certificate expiration
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -dates

# Check certificate issuer
kubectl get secret gateway-tls -n kubernaut-system -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -issuer
```

**Resolution**:
1. Renew certificate using cert-manager or OpenShift Service CA
2. Restart Gateway pods to pick up new certificate
3. Update Prometheus `tls_config.ca_file` if CA changed

### Issue: Rate limiting blocking legitimate traffic

**Symptoms**: `429 Too Many Requests` responses, `gateway_rate_limit_exceeded_total` metric increasing

**Diagnosis**:
```bash
# Check rate limit configuration
kubectl get deployment gateway -n kubernaut-system -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="GATEWAY_RATE_LIMIT")].value}'

# Check Redis rate limit keys
kubectl exec -it redis-gateway-ha-0 -n kubernaut-system -- redis-cli KEYS "rate_limit:*"
```

**Resolution**:
1. Increase `GATEWAY_RATE_LIMIT` environment variable (default: 100 req/min)
2. Increase `GATEWAY_RATE_LIMIT_WINDOW` (default: 60s)
3. Scale Gateway horizontally (more replicas)

## References

- [DD-GATEWAY-004: Authentication Strategy](../decisions/DD-GATEWAY-004-authentication-strategy.md)
- [Kubernetes Network Policies](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [OpenShift Service Serving Certificates](https://docs.openshift.com/container-platform/latest/security/certificates/service-serving-certificate.html)
- [cert-manager Documentation](https://cert-manager.io/docs/)
- [HAProxy TLS Configuration](https://www.haproxy.com/documentation/hapee/latest/security/tls/)

## Changelog

### v1.0.0 (2025-10-27)
- Initial release
- Configuration 1: Network Policies + Service TLS (production-ready)
- Configuration 2: Reverse Proxy (production-ready)
- Configuration 3: Sidecar Authentication (deferred to v2.0)




