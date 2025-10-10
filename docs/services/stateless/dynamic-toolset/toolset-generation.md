# Dynamic Toolset Service - Toolset Generation Deep Dive

**Version**: v1.0
**Last Updated**: October 10, 2025
**Status**: ✅ Design Complete

---

## Overview

Toolset Generation transforms discovered services into HolmesGPT-compatible toolset configurations. Each service type has a dedicated generator that produces YAML configurations following HolmesGPT's toolset specification.

---

## Generation Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│           Toolset Generation Pipeline                       │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  ToolsetConfigMapBuilder (Orchestrator)              │  │
│  │  - Coordinates multiple generators                   │  │
│  │  - Builds complete ConfigMap                         │  │
│  │  - Merges admin overrides                            │  │
│  └────────────┬─────────────────────────────────────────┘  │
│               │                                             │
│               │ Delegates to registered generators         │
│               ▼                                             │
│  ┌──────────────────────────────────────────────────────┐  │
│  │  ConfigMapGenerator Interface                        │  │
│  │  - Generate(service) → YAML string                   │  │
│  │  - ServiceType() → string                            │  │
│  └─────┬────────┬─────────┬─────────┬─────────┬─────────┘  │
│        │        │         │         │         │             │
│        ▼        ▼         ▼         ▼         ▼             │
│   Kubernetes Prometheus Grafana  Jaeger  Elasticsearch      │
│   Generator  Generator Generator Generator Generator        │
└─────────────────────────────────────────────────────────────┘
                │
                │ Produces YAML toolset configs
                ▼
        ┌───────────────────┐
        │ ConfigMap         │
        │ (HolmesGPT format)│
        └───────────────────┘
```

---

## HolmesGPT Toolset Format

### Standard Toolset Structure

```yaml
toolset: <toolset-name>        # e.g., "prometheus", "grafana", "kubernetes"
enabled: <true|false>          # Whether toolset is active
config:                        # Toolset-specific configuration
  <key>: <value>               # e.g., url, apiKey, timeout
```

### Example: Prometheus Toolset

```yaml
toolset: prometheus
enabled: true
config:
  url: "http://prometheus-server.monitoring.svc.cluster.local:9090"
  timeout: "30s"
```

---

## Generator Implementations

### Kubernetes Toolset Generator

**Purpose**: Always-included built-in toolset for Kubernetes API access

**Generation Logic**:
```go
func generateKubernetesToolset() string {
    return `toolset: kubernetes
enabled: true
config:
  incluster: true
  namespaces: ["*"]
`
}
```

**Output**:
```yaml
toolset: kubernetes
enabled: true
config:
  incluster: true          # Use in-cluster service account
  namespaces: ["*"]        # Access all namespaces
```

**Rationale**: Kubernetes toolset is always available (no discovery needed)

---

### Prometheus Toolset Generator

**Purpose**: Generate Prometheus toolset configuration from discovered Prometheus service

**Implementation**:
```go
type PrometheusToolsetGenerator struct{}

func (g *PrometheusToolsetGenerator) ServiceType() string {
    return "prometheus"
}

func (g *PrometheusToolsetGenerator) Generate(
    ctx context.Context,
    service toolset.DiscoveredService,
) (string, error) {
    if service.Type != "prometheus" {
        return "", fmt.Errorf("invalid service type: %s", service.Type)
    }

    config := fmt.Sprintf(`toolset: prometheus
enabled: true
config:
  url: "%s"
  timeout: "30s"
  # Prometheus API queries will target this endpoint
  # Example queries: up{}, rate(http_requests_total[5m])
`, service.Endpoint)

    return config, nil
}
```

**Input** (DiscoveredService):
```go
toolset.DiscoveredService{
    Name:      "prometheus-server",
    Namespace: "monitoring",
    Type:      "prometheus",
    Endpoint:  "http://prometheus-server.monitoring.svc.cluster.local:9090",
}
```

**Output** (YAML):
```yaml
toolset: prometheus
enabled: true
config:
  url: "http://prometheus-server.monitoring.svc.cluster.local:9090"
  timeout: "30s"
  # Prometheus API queries will target this endpoint
  # Example queries: up{}, rate(http_requests_total[5m])
```

---

### Grafana Toolset Generator

**Purpose**: Generate Grafana toolset configuration from discovered Grafana service

**Implementation**:
```go
type GrafanaToolsetGenerator struct{}

func (g *GrafanaToolsetGenerator) ServiceType() string {
    return "grafana"
}

func (g *GrafanaToolsetGenerator) Generate(
    ctx context.Context,
    service toolset.DiscoveredService,
) (string, error) {
    if service.Type != "grafana" {
        return "", fmt.Errorf("invalid service type: %s", service.Type)
    }

    config := fmt.Sprintf(`toolset: grafana
enabled: true
config:
  url: "%s"
  apiKey: "${GRAFANA_API_KEY}"  # From Kubernetes Secret
  # Grafana API access for dashboard and panel queries
  # Requires GRAFANA_API_KEY environment variable
`, service.Endpoint)

    return config, nil
}
```

**Output**:
```yaml
toolset: grafana
enabled: true
config:
  url: "http://grafana.monitoring.svc.cluster.local:3000"
  apiKey: "${GRAFANA_API_KEY}"  # From Kubernetes Secret
  # Grafana API access for dashboard and panel queries
  # Requires GRAFANA_API_KEY environment variable
```

**Note**: API key is environment variable reference (resolved by HolmesGPT API at runtime)

---

### Jaeger Toolset Generator

**Purpose**: Generate Jaeger toolset configuration from discovered Jaeger service

**Implementation**:
```go
type JaegerToolsetGenerator struct{}

func (g *JaegerToolsetGenerator) ServiceType() string {
    return "jaeger"
}

func (g *JaegerToolsetGenerator) Generate(
    ctx context.Context,
    service toolset.DiscoveredService,
) (string, error) {
    config := fmt.Sprintf(`toolset: jaeger
enabled: true
config:
  url: "%s"
  query_endpoint: "/api/traces"
  # Jaeger tracing backend for distributed traces
`, service.Endpoint)

    return config, nil
}
```

**Output**:
```yaml
toolset: jaeger
enabled: true
config:
  url: "http://jaeger-query.observability.svc.cluster.local:16686"
  query_endpoint: "/api/traces"
  # Jaeger tracing backend for distributed traces
```

---

## ConfigMap Builder

### Builder Orchestration

**Purpose**: Coordinate all generators to build complete ConfigMap

**Implementation**:
```go
type ToolsetConfigMapBuilder struct {
    generators map[string]ConfigMapGenerator
}

func NewToolsetConfigMapBuilder() *ToolsetConfigMapBuilder {
    return &ToolsetConfigMapBuilder{
        generators: make(map[string]ConfigMapGenerator),
    }
}

func (b *ToolsetConfigMapBuilder) RegisterGenerator(gen ConfigMapGenerator) {
    b.generators[gen.ServiceType()] = gen
}

func (b *ToolsetConfigMapBuilder) BuildConfigMap(
    ctx context.Context,
    services []toolset.DiscoveredService,
    overrides map[string]string,
) (*corev1.ConfigMap, error) {
    configMapData := make(map[string]string)

    // Always include Kubernetes toolset (built-in)
    configMapData["kubernetes-toolset.yaml"] = generateKubernetesToolset()

    // Generate toolset configs for discovered services
    for _, svc := range services {
        generator, ok := b.generators[svc.Type]
        if !ok {
            continue // Skip services without generators
        }

        config, err := generator.Generate(ctx, svc)
        if err != nil {
            return nil, fmt.Errorf("failed to generate %s toolset: %w", svc.Type, err)
        }

        key := fmt.Sprintf("%s-toolset.yaml", svc.Type)
        configMapData[key] = config
    }

    // Merge overrides (admin-configured toolsets)
    for key, value := range overrides {
        if key == "overrides.yaml" {
            configMapData[key] = value // Preserve admin overrides
        }
    }

    // Build ConfigMap
    cm := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "kubernaut-toolset-config",
            Namespace: "kubernaut-system",
            Labels: map[string]string{
                "app.kubernetes.io/name":       "dynamic-toolset",
                "app.kubernetes.io/component":  "toolset-config",
                "app.kubernetes.io/managed-by": "dynamic-toolset-service",
            },
        },
        Data: configMapData,
    }

    return cm, nil
}
```

---

## Generation Flow

### Step-by-Step Process

#### 1. **Discovered Services Input**
```go
services := []toolset.DiscoveredService{
    {Name: "prometheus-server", Type: "prometheus", Endpoint: "http://..."},
    {Name: "grafana", Type: "grafana", Endpoint: "http://..."},
}
```

#### 2. **Generate Individual Toolsets**
```go
for _, svc := range services {
    generator, ok := b.generators[svc.Type]
    config, err := generator.Generate(ctx, svc)
    configMapData[fmt.Sprintf("%s-toolset.yaml", svc.Type)] = config
}
```

#### 3. **Build Complete ConfigMap**
```go
cm := &corev1.ConfigMap{
    ObjectMeta: metav1.ObjectMeta{
        Name: "kubernaut-toolset-config",
        Namespace: "kubernaut-system",
    },
    Data: configMapData,
}
```

#### 4. **Example Output ConfigMap**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernaut-toolset-config
  namespace: kubernaut-system
  labels:
    app.kubernetes.io/name: dynamic-toolset
    app.kubernetes.io/component: toolset-config
data:
  kubernetes-toolset.yaml: |
    toolset: kubernetes
    enabled: true
    config:
      incluster: true
      namespaces: ["*"]

  prometheus-toolset.yaml: |
    toolset: prometheus
    enabled: true
    config:
      url: "http://prometheus-server.monitoring.svc.cluster.local:9090"
      timeout: "30s"

  grafana-toolset.yaml: |
    toolset: grafana
    enabled: true
    config:
      url: "http://grafana.monitoring.svc.cluster.local:3000"
      apiKey: "${GRAFANA_API_KEY}"

  overrides.yaml: |
    # Admin-managed custom toolsets
    custom-elasticsearch:
      enabled: true
      config:
        url: "http://elasticsearch.logging:9200"
```

---

## Override Merging

### Admin Override Pattern

**Purpose**: Allow admins to add custom toolsets or override auto-generated configs

**Mechanism**: Special `overrides.yaml` section in ConfigMap

### Override Merge Algorithm

```go
func (b *ToolsetConfigMapBuilder) mergeOverrides(
    generated map[string]string,
    overrides map[string]string,
) map[string]string {
    merged := make(map[string]string)

    // Copy all generated toolsets
    for key, value := range generated {
        merged[key] = value
    }

    // Add/preserve admin overrides
    if overridesYAML, ok := overrides["overrides.yaml"]; ok {
        merged["overrides.yaml"] = overridesYAML
    }

    return merged
}
```

**Result**: Auto-generated toolsets + admin overrides in single ConfigMap

---

## Environment Variable References

### Security Pattern: Secrets as Environment Variables

**Problem**: API keys and tokens should not be stored in ConfigMap (plaintext)

**Solution**: Reference environment variables, populate from Kubernetes Secrets

### Example: Grafana API Key

**ConfigMap** (toolset config):
```yaml
grafana-toolset.yaml: |
  toolset: grafana
  config:
    url: "http://grafana.monitoring:3000"
    apiKey: "${GRAFANA_API_KEY}"  # Environment variable reference
```

**Kubernetes Secret**:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: holmesgpt-api-secrets
  namespace: kubernaut-system
type: Opaque
stringData:
  GRAFANA_API_KEY: "glsa_EXAMPLE_API_KEY_NOT_REAL_1234567890"
```

**HolmesGPT API Deployment**:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: holmesgpt-api
spec:
  template:
    spec:
      containers:
      - name: holmesgpt
        env:
        - name: GRAFANA_API_KEY
          valueFrom:
            secretKeyRef:
              name: holmesgpt-api-secrets
              key: GRAFANA_API_KEY
```

**Runtime Resolution**: HolmesGPT API reads ConfigMap, replaces `${GRAFANA_API_KEY}` with actual key from environment

---

## Validation

### ConfigMap Size Validation

**Purpose**: Prevent ConfigMap from exceeding Kubernetes 1MB limit

```go
func (b *ToolsetConfigMapBuilder) validateSize(cm *corev1.ConfigMap) error {
    totalSize := 0
    for _, value := range cm.Data {
        totalSize += len(value)
    }

    const maxSize = 900000 // 900KB (safety margin)
    if totalSize > maxSize {
        return fmt.Errorf("ConfigMap size %d bytes exceeds limit %d", totalSize, maxSize)
    }

    return nil
}
```

### YAML Validation

**Purpose**: Ensure generated YAML is syntactically correct

```go
func (b *ToolsetConfigMapBuilder) validateYAML(yamlStr string) error {
    var data map[string]interface{}
    if err := yaml.Unmarshal([]byte(yamlStr), &data); err != nil {
        return fmt.Errorf("invalid YAML: %w", err)
    }
    return nil
}
```

---

## Monitoring & Metrics

### Generation Metrics

```go
// Toolsets generated by type
toolsetsGeneratedTotal.WithLabelValues("prometheus").Inc()

// Generation failures
generationFailuresTotal.WithLabelValues("grafana", "yaml_error").Inc()
```

### Logging

```go
logger.Info("Toolset generated",
    zap.String("toolset_type", service.Type),
    zap.String("service_name", service.Name),
    zap.Int("config_size_bytes", len(config)))
```

---

## Testing Strategy

### Unit Tests

**Focus**: Generator logic and YAML generation

```go
It("generates Prometheus toolset with correct URL", func() {
    generator := generator.NewPrometheusToolsetGenerator()

    service := toolset.DiscoveredService{
        Type:     "prometheus",
        Endpoint: "http://prometheus-server.monitoring:9090",
    }

    config, err := generator.Generate(ctx, service)

    Expect(err).ToNot(HaveOccurred())
    Expect(config).To(ContainSubstring("toolset: prometheus"))
    Expect(config).To(ContainSubstring("url: \"http://prometheus-server.monitoring:9090\""))
})
```

### Integration Tests

**Focus**: Complete ConfigMap generation and Kubernetes write

```go
It("creates ConfigMap with all discovered toolsets", func() {
    builder := generator.NewToolsetConfigMapBuilder()
    builder.RegisterGenerator(generator.NewPrometheusToolsetGenerator())
    builder.RegisterGenerator(generator.NewGrafanaToolsetGenerator())

    services := []toolset.DiscoveredService{
        {Name: "prometheus", Type: "prometheus", Endpoint: "http://..."},
        {Name: "grafana", Type: "grafana", Endpoint: "http://..."},
    }

    cm, err := builder.BuildConfigMap(ctx, services, nil)

    Expect(err).ToNot(HaveOccurred())
    Expect(cm.Data).To(HaveKey("kubernetes-toolset.yaml"))
    Expect(cm.Data).To(HaveKey("prometheus-toolset.yaml"))
    Expect(cm.Data).To(HaveKey("grafana-toolset.yaml"))

    // Write to Kubernetes
    _, err = k8sClient.CoreV1().ConfigMaps("kubernaut-system").Create(ctx, cm, metav1.CreateOptions{})
    Expect(err).ToNot(HaveOccurred())
})
```

---

## Future Enhancements (V2+)

### Dynamic Toolset Templates

**Concept**: Allow admins to provide custom toolset templates

```yaml
# Custom template for Datadog
apiVersion: v1
kind: ConfigMap
metadata:
  name: toolset-template-datadog
data:
  template.yaml: |
    toolset: datadog
    enabled: true
    config:
      api_key: "${DATADOG_API_KEY}"
      app_key: "${DATADOG_APP_KEY}"
      site: "{{.Service.Metadata.datadog_site}}"
```

### Multi-Instance Toolsets

**Concept**: Support multiple Prometheus/Grafana instances

```yaml
prometheus-toolset-monitoring.yaml: |
  toolset: prometheus
  name: monitoring-prometheus
  config:
    url: "http://prometheus-server.monitoring:9090"

prometheus-toolset-production.yaml: |
  toolset: prometheus
  name: production-prometheus
  config:
    url: "http://prometheus-server.production:9090"
```

---

**Document Status**: ✅ Toolset Generation Deep Dive Complete
**Last Updated**: October 10, 2025
**Related**: [implementation.md](./implementation.md), [service-discovery.md](./service-discovery.md), [configmap-reconciliation.md](./configmap-reconciliation.md)

