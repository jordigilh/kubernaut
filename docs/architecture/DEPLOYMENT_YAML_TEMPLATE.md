# Deployment YAML Template - Kubernaut Services

**Version**: v1.0
**Last Updated**: October 6, 2025
**Status**: ✅ **STANDARDIZED**
**Scope**: All 6 Stateless HTTP Services

---

## 📋 **Standard Deployment Template**

### **HTTP Service Deployment** (Gateway, Context API, Data Storage, etc.)

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: kubernaut-system
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {service-name}  # e.g., gateway, context-api
  namespace: kubernaut-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {service-name}
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: {service-name}
    app.kubernetes.io/instance: {service-name}
    app.kubernetes.io/component: http-service
    app.kubernetes.io/part-of: kubernaut
    app.kubernetes.io/managed-by: kubectl
spec:
  replicas: 2  # Production: 2-3, Development: 1
  selector:
    matchLabels:
      app: {service-name}

  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0  # Zero downtime

  template:
    metadata:
      labels:
        app: {service-name}
        app.kubernetes.io/name: {service-name}
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"

    spec:
      serviceAccountName: {service-name}

      # Security context
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000

      containers:
      - name: {service-name}
        image: kubernaut/{service-name}:v1.0.0
        imagePullPolicy: IfNotPresent

        # Ports
        ports:
        - containerPort: 8080
          name: http
          protocol: TCP
        - containerPort: 9090
          name: metrics
          protocol: TCP

        # Environment variables
        env:
        - name: SERVICE_PORT
          value: "8080"
        - name: METRICS_PORT
          value: "9090"
        - name: LOG_LEVEL
          value: "info"
        - name: POD_NAME
          valueFrom:
            fieldRef:
              fieldPath: metadata.name
        - name: POD_NAMESPACE
          valueFrom:
            fieldRef:
              fieldPath: metadata.namespace

        # Health checks
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3

        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
          timeoutSeconds: 5
          failureThreshold: 2

        # Resource limits
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi

        # Security context
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
          readOnlyRootFilesystem: true

        # Volume mounts (if needed)
        volumeMounts:
        - name: tmp
          mountPath: /tmp
        - name: cache
          mountPath: /app/cache

      # Volumes
      volumes:
      - name: tmp
        emptyDir: {}
      - name: cache
        emptyDir: {}

      # Affinity rules (anti-affinity for high availability)
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app: {service-name}
              topologyKey: kubernetes.io/hostname

      # Termination grace period
      terminationGracePeriodSeconds: 30
---
apiVersion: v1
kind: Service
metadata:
  name: {service-name}
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: {service-name}
spec:
  selector:
    app: {service-name}
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
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: {service-name}-pdb
  namespace: kubernaut-system
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: {service-name}
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: {service-name}-hpa
  namespace: kubernaut-system
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: {service-name}
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300  # 5 minutes
      policies:
      - type: Percent
        value: 50
        periodSeconds: 60
    scaleUp:
      stabilizationWindowSeconds: 0
      policies:
      - type: Percent
        value: 100
        periodSeconds: 30
      - type: Pods
        value: 2
        periodSeconds: 30
      selectPolicy: Max
```

---

## 🎯 **Service-Specific Deployments**

### **1. Gateway Service**

```yaml
# deploy/gateway-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  replicas: 3  # High availability for ingestion
  template:
    spec:
      serviceAccountName: gateway
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8080
        - containerPort: 9090
        env:
        - name: REDIS_HOST
          value: "redis.kubernaut-system:6379"
        - name: REGO_POLICY_PATH
          value: "/etc/kubernaut/policies"
        volumeMounts:
        - name: policies
          mountPath: /etc/kubernaut/policies
          readOnly: true
        resources:
          requests:
            cpu: 200m      # Higher for signal processing
            memory: 256Mi
          limits:
            cpu: 1000m
            memory: 1Gi
      volumes:
      - name: policies
        configMap:
          name: rego-policies
```

---

### **2. Context API Service**

```yaml
# deploy/context-api-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: context-api
  namespace: kubernaut-system
spec:
  replicas: 2
  template:
    spec:
      serviceAccountName: context-api
      containers:
      - name: context-api
        image: kubernaut/context-api:v1.0.0
        ports:
        - containerPort: 8080
        - containerPort: 9090
        env:
        - name: POSTGRES_HOST
          value: "postgresql.kubernaut-system:5432"
        - name: POSTGRES_DB
          value: "kubernaut"
        - name: POSTGRES_USER
          valueFrom:
            secretKeyRef:
              name: postgresql-credentials
              key: username
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgresql-credentials
              key: password
        - name: REDIS_HOST
          value: "redis.kubernaut-system:6379"
        resources:
          requests:
            cpu: 150m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 512Mi
```

---

### **3. Data Storage Service**

```yaml
# deploy/data-storage-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: data-storage
  namespace: kubernaut-system
spec:
  replicas: 2
  template:
    spec:
      serviceAccountName: data-storage
      containers:
      - name: data-storage
        image: kubernaut/data-storage:v1.0.0
        ports:
        - containerPort: 8080
        - containerPort: 9090
        env:
        - name: POSTGRES_HOST
          value: "postgresql.kubernaut-system:5432"
        - name: POSTGRES_DB
          value: "kubernaut"
        - name: POSTGRES_USER
          valueFrom:
            secretKeyRef:
              name: postgresql-credentials
              key: username
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: postgresql-credentials
              key: password
        - name: REDIS_HOST
          value: "redis.kubernaut-system:6379"
        - name: LLM_ENDPOINT
          value: "http://llm-service:8080"
        resources:
          requests:
            cpu: 150m
            memory: 256Mi
          limits:
            cpu: 500m
            memory: 512Mi
```

---

### **4. HolmesGPT API Service**

```yaml
# deploy/holmesgpt-api-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
  namespace: kubernaut-system
spec:
  replicas: 2
  template:
    spec:
      serviceAccountName: holmesgpt-api
      containers:
      - name: holmesgpt-api
        image: kubernaut/holmesgpt-api:v1.0.0
        ports:
        - containerPort: 8080
        - containerPort: 9090
        env:
        - name: LLM_PROVIDER
          value: "openai"
        - name: LLM_MODEL
          value: "gpt-4"
        - name: OPENAI_API_KEY
          valueFrom:
            secretKeyRef:
              name: llm-credentials
              key: openai-api-key
        volumeMounts:
        - name: toolsets
          mountPath: /etc/kubernaut/toolsets
          readOnly: true
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
      volumes:
      - name: toolsets
        configMap:
          name: kubernaut-toolset-config
```

---

### **5. Notification Service**

```yaml
# deploy/notification-service-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: notification
  namespace: kubernaut-system
spec:
  replicas: 2
  template:
    spec:
      serviceAccountName: notification
      containers:
      - name: notification
        image: kubernaut/notification:v1.0.0
        ports:
        - containerPort: 8080
        - containerPort: 9090
        env:
        - name: SLACK_TOKEN
          valueFrom:
            secretKeyRef:
              name: notification-credentials
              key: slack-token
        - name: TEAMS_WEBHOOK
          valueFrom:
            secretKeyRef:
              name: notification-credentials
              key: teams-webhook
        - name: SMTP_HOST
          value: "smtp.example.com"
        - name: SMTP_PORT
          value: "587"
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 300m
            memory: 256Mi
```

---

### **6. Dynamic Toolset Service**

```yaml
# deploy/dynamic-toolset-deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: dynamic-toolset
  namespace: kubernaut-system
spec:
  replicas: 2
  template:
    spec:
      serviceAccountName: dynamic-toolset
      containers:
      - name: dynamic-toolset
        image: kubernaut/dynamic-toolset:v1.0.0
        ports:
        - containerPort: 8080
        - containerPort: 9090
        env:
        - name: DISCOVERY_INTERVAL
          value: "5m"
        - name: RECONCILIATION_INTERVAL
          value: "30s"
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 200m
            memory: 256Mi
```

---

## 📊 **Resource Sizing Recommendations**

### **Production Sizing**

| Service | CPU Request | CPU Limit | Memory Request | Memory Limit | Replicas |
|---------|-------------|-----------|----------------|--------------|----------|
| **Gateway** | 200m | 1000m | 256Mi | 1Gi | 3 |
| **Context API** | 150m | 500m | 256Mi | 512Mi | 2 |
| **Data Storage** | 150m | 500m | 256Mi | 512Mi | 2 |
| **HolmesGPT API** | 100m | 500m | 128Mi | 512Mi | 2 |
| **Notification** | 100m | 300m | 128Mi | 256Mi | 2 |
| **Dynamic Toolset** | 100m | 200m | 128Mi | 256Mi | 2 |

---

### **Development Sizing**

| Service | CPU Request | CPU Limit | Memory Request | Memory Limit | Replicas |
|---------|-------------|-----------|----------------|--------------|----------|
| **All Services** | 50m | 200m | 64Mi | 128Mi | 1 |

---

## 🔒 **RBAC Configuration**

### **HTTP Service RBAC Template**

```yaml
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: {service-name}-reader
rules:
# Read services for discovery
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list", "watch"]

# Read ConfigMaps (if needed)
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list"]

# TokenReviewer for authentication
- apiGroups: ["authentication.k8s.io"]
  resources: ["tokenreviews"]
  verbs: ["create"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: {service-name}-reader-binding
subjects:
- kind: ServiceAccount
  name: {service-name}
  namespace: kubernaut-system
roleRef:
  kind: ClusterRole
  name: {service-name}-reader
  apiGroup: rbac.authorization.k8s.io
```

---

## ✅ **Deployment Checklist**

### **Before Deploying**:

1. ✅ **Namespace exists**: `kubectl create ns kubernaut-system`
2. ✅ **Secrets created**: PostgreSQL, Redis, LLM credentials
3. ✅ **ConfigMaps created**: Policies, toolsets, configuration
4. ✅ **RBAC applied**: ServiceAccount, ClusterRole, ClusterRoleBinding
5. ✅ **Dependencies running**: PostgreSQL, Redis, etc.

### **Deployment Order**:

1. Dependencies (PostgreSQL, Redis)
2. Dynamic Toolset Service
3. Data Storage Service
4. Context API Service
5. HolmesGPT API Service
6. Gateway Service
7. Notification Service

### **Post-Deployment**:

1. ✅ **Health checks passing**: `kubectl get pods -n kubernaut-system`
2. ✅ **Services registered**: `kubectl get svc -n kubernaut-system`
3. ✅ **Metrics endpoint**: Prometheus scraping successfully
4. ✅ **Logs clean**: No errors in pod logs

---

## 📚 **Related Documentation**

- [STATELESS_SERVICES_PORT_STANDARD.md](./STATELESS_SERVICES_PORT_STANDARD.md) - Port configuration
- [SERVICEACCOUNT_NAMING_STANDARD.md](./SERVICEACCOUNT_NAMING_STANDARD.md) - ServiceAccount names
- [HEALTH_CHECK_STANDARD.md](./HEALTH_CHECK_STANDARD.md) - Health probe configuration
- [PROMETHEUS_SERVICEMONITOR_PATTERN.md](./PROMETHEUS_SERVICEMONITOR_PATTERN.md) - Metrics scraping

---

**Document Status**: ✅ Complete
**Compliance**: 6/6 services covered
**Last Updated**: October 6, 2025
**Version**: 1.0
