# Dynamic Toolset Service - BR Test Strategy

**Version**: v1.0
**Created**: October 10, 2025
**Status**: 🎯 Ready for Implementation
**Total BRs**: 20 (estimated)

---

## Executive Summary

**Business Requirements**: The Dynamic Toolset Service automates discovery and configuration of HolmesGPT toolsets for Kubernetes clusters.

**Test Coverage Strategy**:
- **Unit Tests (70%+)**: Service detection logic, ConfigMap generation, health checks
- **Integration Tests (>50%)**: Kubernetes service discovery, ConfigMap reconciliation
- **E2E Tests (<10%)**: Complete discovery flow, HolmesGPT API integration

**Estimated BRs**: 20 business requirements across 4 categories

---

## BR Categories

### Category 1: Service Discovery (BR-TOOLSET-001 to BR-TOOLSET-008)
**Purpose**: Automatically discover services in Kubernetes cluster

| BR | Description | Test Type | Estimated Tests |
|----|-------------|-----------|-----------------|
| BR-TOOLSET-001 | Prometheus service detection | Unit | 5 |
| BR-TOOLSET-002 | Grafana service detection | Unit | 5 |
| BR-TOOLSET-003 | Jaeger service detection | Unit | 4 |
| BR-TOOLSET-004 | Elasticsearch service detection | Unit | 4 |
| BR-TOOLSET-005 | Custom service detection (annotations) | Unit | 6 |
| BR-TOOLSET-006 | Health check validation | Unit + Integration | 8 |
| BR-TOOLSET-007 | Discovery loop (5-minute interval) | Integration | 2 |
| BR-TOOLSET-008 | Multi-namespace discovery | Integration | 3 |

**Total Estimated Tests**: 37 (30 unit + 7 integration)

---

### Category 2: ConfigMap Management (BR-TOOLSET-009 to BR-TOOLSET-013)
**Purpose**: Generate and manage HolmesGPT toolset ConfigMaps

| BR | Description | Test Type | Estimated Tests |
|----|-------------|-----------|-----------------|
| BR-TOOLSET-009 | Kubernetes toolset generation | Unit | 3 |
| BR-TOOLSET-010 | Prometheus toolset generation | Unit | 4 |
| BR-TOOLSET-011 | Grafana toolset generation | Unit | 4 |
| BR-TOOLSET-012 | ConfigMap builder | Unit | 6 |
| BR-TOOLSET-013 | ConfigMap creation/update | Integration | 5 |

**Total Estimated Tests**: 22 (17 unit + 5 integration)

---

### Category 3: Reconciliation (BR-TOOLSET-014 to BR-TOOLSET-017)
**Purpose**: Ensure ConfigMap matches desired state

| BR | Description | Test Type | Estimated Tests |
|----|-------------|-----------|-----------------|
| BR-TOOLSET-014 | Drift detection | Unit + Integration | 6 |
| BR-TOOLSET-015 | ConfigMap reconciliation loop | Integration | 3 |
| BR-TOOLSET-016 | Override preservation | Unit + Integration | 5 |
| BR-TOOLSET-017 | ConfigMap deletion recovery | Integration | 2 |

**Total Estimated Tests**: 16 (6 unit + 10 integration)

---

### Category 4: HTTP API & Observability (BR-TOOLSET-018 to BR-TOOLSET-020)
**Purpose**: Provide manual toolset queries and observability

| BR | Description | Test Type | Estimated Tests |
|----|-------------|-----------|-----------------|
| BR-TOOLSET-018 | HTTP API endpoints (/toolsets, /services, /discover) | Unit + Integration | 8 |
| BR-TOOLSET-019 | Health and readiness probes | Integration | 4 |
| BR-TOOLSET-020 | Prometheus metrics | Unit | 6 |

**Total Estimated Tests**: 18 (12 unit + 6 integration)

---

## Total Test Estimates

| Test Type | Total Tests | Coverage Target |
|-----------|-------------|-----------------|
| **Unit Tests** | 65 | >70% |
| **Integration Tests** | 28 | >50% |
| **E2E Tests** | 2 | <10% |
| **Grand Total** | 95 tests |  |

---

## Detailed BR Test Specifications

### BR-TOOLSET-001: Prometheus Service Detection

**Business Requirement**: Detect Prometheus services in Kubernetes cluster using labels, service name, and port patterns.

**Test Strategy**: Unit tests with mock Kubernetes services

#### Test 1.1: Detect by Label
```go
It("detects Prometheus service by 'app=prometheus' label", func() {
    // BUSINESS OUTCOME: Admin deploys Prometheus with standard label
    // Expected: Dynamic Toolset discovers and includes in toolset config

    mockService := &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "prometheus-server",
            Namespace: "monitoring",
            Labels: map[string]string{
                "app": "prometheus",
            },
        },
        Spec: corev1.ServiceSpec{
            Ports: []corev1.ServicePort{
                {Name: "web", Port: 9090},
            },
        },
    }

    detector := discovery.NewPrometheusDetector(logger)
    discovered, err := detector.Detect(ctx, []corev1.Service{*mockService})

    Expect(err).ToNot(HaveOccurred())
    Expect(discovered).To(HaveLen(1))
    Expect(discovered[0].Type).To(Equal("prometheus"))
    Expect(discovered[0].Endpoint).To(ContainSubstring("prometheus-server.monitoring"))
})
```

#### Test 1.2: Detect by Service Name
```go
It("detects Prometheus service by name 'prometheus'", func() {
    mockService := &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "prometheus",
            Namespace: "observability",
        },
        Spec: corev1.ServiceSpec{
            Ports: []corev1.ServicePort{
                {Name: "web", Port: 9090},
            },
        },
    }

    detector := discovery.NewPrometheusDetector(logger)
    discovered, err := detector.Detect(ctx, []corev1.Service{*mockService})

    Expect(err).ToNot(HaveOccurred())
    Expect(discovered).To(HaveLen(1))
})
```

#### Test 1.3: Detect by Port
```go
It("detects Prometheus service by port 9090 named 'web'", func() {
    mockService := &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "metrics-server",
            Namespace: "monitoring",
        },
        Spec: corev1.ServiceSpec{
            Ports: []corev1.ServicePort{
                {Name: "web", Port: 9090},
            },
        },
    }

    detector := discovery.NewPrometheusDetector(logger)
    discovered, err := detector.Detect(ctx, []corev1.Service{*mockService})

    Expect(err).ToNot(HaveOccurred())
    Expect(discovered).To(HaveLen(1))
})
```

#### Test 1.4: Ignore Non-Prometheus Services
```go
It("ignores services that don't match Prometheus patterns", func() {
    mockService := &corev1.Service{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "api-gateway",
            Namespace: "default",
            Labels: map[string]string{
                "app": "gateway",
            },
        },
        Spec: corev1.ServiceSpec{
            Ports: []corev1.ServicePort{
                {Name: "http", Port: 8080},
            },
        },
    }

    detector := discovery.NewPrometheusDetector(logger)
    discovered, err := detector.Detect(ctx, []corev1.Service{*mockService})

    Expect(err).ToNot(HaveOccurred())
    Expect(discovered).To(BeEmpty())
})
```

#### Test 1.5: Multiple Prometheus Instances
```go
It("detects multiple Prometheus instances across namespaces", func() {
    promService1 := makePrometheusService("prometheus-1", "monitoring")
    promService2 := makePrometheusService("prometheus-2", "observability")

    detector := discovery.NewPrometheusDetector(logger)
    discovered, err := detector.Detect(ctx, []corev1.Service{promService1, promService2})

    Expect(err).ToNot(HaveOccurred())
    Expect(discovered).To(HaveLen(2))
})
```

**Test Decision**: Unit tests only (no K8s API needed, mock services sufficient)

---

### BR-TOOLSET-006: Health Check Validation

**Business Requirement**: Validate discovered services are healthy before including in toolset configuration.

**Test Strategy**: Unit + Integration tests

#### Test 6.1: Healthy Prometheus Service (Unit)
```go
It("includes healthy Prometheus service in discovery", func() {
    // BUSINESS OUTCOME: Only operational services are included
    // Expected: Health check passes, service included

    mockHTTPServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/-/healthy" {
            w.WriteHeader(http.StatusOK)
        }
    }))
    defer mockHTTPServer.Close()

    detector := discovery.NewPrometheusDetector(logger)
    err := detector.HealthCheck(ctx, mockHTTPServer.URL)

    Expect(err).ToNot(HaveOccurred())
})
```

#### Test 6.2: Unhealthy Prometheus Service (Unit)
```go
It("excludes unhealthy Prometheus service from discovery", func() {
    mockHTTPServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusServiceUnavailable)
    }))
    defer mockHTTPServer.Close()

    detector := discovery.NewPrometheusDetector(logger)
    err := detector.HealthCheck(ctx, mockHTTPServer.URL)

    Expect(err).To(HaveOccurred())
})
```

#### Test 6.3: Health Check Timeout (Unit)
```go
It("fails health check after timeout", func() {
    mockHTTPServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        time.Sleep(10 * time.Second) // Longer than timeout
    }))
    defer mockHTTPServer.Close()

    detector := discovery.NewPrometheusDetector(logger)
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()

    err := detector.HealthCheck(ctx, mockHTTPServer.URL)

    Expect(err).To(HaveOccurred())
    Expect(err).To(MatchError(ContainSubstring("context deadline exceeded")))
})
```

#### Test 6.4: Real Prometheus Health Check (Integration)
```go
It("validates real Prometheus service health in Kind cluster", func() {
    // BUSINESS OUTCOME: Real health check integration
    // Expected: Deployed Prometheus passes health check

    // Prometheus should be deployed to Kind cluster in test setup
    discoverer := discovery.NewServiceDiscoverer(k8sClient, logger)
    discoverer.RegisterDetector(discovery.NewPrometheusDetector(logger))

    discovered, err := discoverer.DiscoverServices(ctx)

    Expect(err).ToNot(HaveOccurred())
    Expect(discovered).To(ContainElement(
        MatchFields(IgnoreExtras, Fields{
            "Type":    Equal("prometheus"),
            "Healthy": BeTrue(),
        }),
    ))
})
```

**Test Decision**: Mix of unit (mock HTTP) and integration (real services)

---

### BR-TOOLSET-012: ConfigMap Builder

**Business Requirement**: Build complete ConfigMap from discovered services with all toolsets.

**Test Strategy**: Unit tests with mock discovered services

#### Test 12.1: Build ConfigMap with Kubernetes Toolset
```go
It("always includes Kubernetes toolset in ConfigMap", func() {
    builder := generator.NewToolsetConfigMapBuilder()

    cm, err := builder.BuildConfigMap(ctx, []toolset.DiscoveredService{}, nil)

    Expect(err).ToNot(HaveOccurred())
    Expect(cm.Data).To(HaveKey("kubernetes-toolset.yaml"))
    Expect(cm.Data["kubernetes-toolset.yaml"]).To(ContainSubstring("toolset: kubernetes"))
})
```

#### Test 12.2: Build ConfigMap with Prometheus
```go
It("includes Prometheus toolset for discovered Prometheus service", func() {
    builder := generator.NewToolsetConfigMapBuilder()
    builder.RegisterGenerator(generator.NewPrometheusToolsetGenerator())

    promService := toolset.DiscoveredService{
        Name:      "prometheus-server",
        Namespace: "monitoring",
        Type:      "prometheus",
        Endpoint:  "http://prometheus-server.monitoring:9090",
    }

    cm, err := builder.BuildConfigMap(ctx, []toolset.DiscoveredService{promService}, nil)

    Expect(err).ToNot(HaveOccurred())
    Expect(cm.Data).To(HaveKey("prometheus-toolset.yaml"))
    Expect(cm.Data["prometheus-toolset.yaml"]).To(ContainSubstring("url: \"http://prometheus-server.monitoring:9090\""))
})
```

#### Test 12.3: Preserve Admin Overrides
```go
It("preserves admin overrides.yaml section", func() {
    builder := generator.NewToolsetConfigMapBuilder()

    overrides := map[string]string{
        "overrides.yaml": `custom-service:
  enabled: true
  config:
    url: "http://custom-service:8080"`,
    }

    cm, err := builder.BuildConfigMap(ctx, []toolset.DiscoveredService{}, overrides)

    Expect(err).ToNot(HaveOccurred())
    Expect(cm.Data).To(HaveKey("overrides.yaml"))
    Expect(cm.Data["overrides.yaml"]).To(ContainSubstring("custom-service"))
})
```

**Test Decision**: Unit tests only (ConfigMap generation is pure logic)

---

### BR-TOOLSET-014: Drift Detection

**Business Requirement**: Detect when ConfigMap has been manually modified or is out of sync with discovered services.

**Test Strategy**: Unit + Integration tests

#### Test 14.1: Detect Missing Key (Unit)
```go
It("detects when toolset key is missing from current ConfigMap", func() {
    reconciler := reconciler.NewConfigMapReconciler(k8sClient, logger)

    current := &corev1.ConfigMap{
        Data: map[string]string{
            "kubernetes-toolset.yaml": "...",
        },
    }

    desired := &corev1.ConfigMap{
        Data: map[string]string{
            "kubernetes-toolset.yaml": "...",
            "prometheus-toolset.yaml": "...",
        },
    }

    hasDrift, driftKeys := reconciler.DetectDrift(current, desired)

    Expect(hasDrift).To(BeTrue())
    Expect(driftKeys).To(ContainElement("missing:prometheus-toolset.yaml"))
})
```

#### Test 14.2: Detect Modified Value (Unit)
```go
It("detects when toolset configuration has been modified", func() {
    reconciler := reconciler.NewConfigMapReconciler(k8sClient, logger)

    current := &corev1.ConfigMap{
        Data: map[string]string{
            "prometheus-toolset.yaml": "url: \"http://old-url:9090\"",
        },
    }

    desired := &corev1.ConfigMap{
        Data: map[string]string{
            "prometheus-toolset.yaml": "url: \"http://new-url:9090\"",
        },
    }

    hasDrift, driftKeys := reconciler.DetectDrift(current, desired)

    Expect(hasDrift).To(BeTrue())
    Expect(driftKeys).To(ContainElement("modified:prometheus-toolset.yaml"))
})
```

#### Test 14.3: Reconcile Drift (Integration)
```go
It("reconciles drifted ConfigMap back to desired state", func() {
    // BUSINESS OUTCOME: Manual ConfigMap edits are overwritten by reconciler
    // Expected: ConfigMap updated to match desired state

    // Create ConfigMap with drift
    driftedCM := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "kubernaut-toolset-config",
            Namespace: "kubernaut-system",
        },
        Data: map[string]string{
            "kubernetes-toolset.yaml": "wrong config",
        },
    }
    k8sClient.CoreV1().ConfigMaps("kubernaut-system").Create(ctx, driftedCM, metav1.CreateOptions{})

    // Run reconciliation
    reconciler := reconciler.NewConfigMapReconciler(k8sClient, logger)
    err := reconciler.Reconcile(ctx, desiredState)

    Expect(err).ToNot(HaveOccurred())

    // Verify ConfigMap updated
    updatedCM, err := k8sClient.CoreV1().ConfigMaps("kubernaut-system").
        Get(ctx, "kubernaut-toolset-config", metav1.GetOptions{})
    Expect(err).ToNot(HaveOccurred())
    Expect(updatedCM.Data["kubernetes-toolset.yaml"]).To(Equal(desiredState.Data["kubernetes-toolset.yaml"]))
})
```

**Test Decision**: Mix of unit (drift detection logic) and integration (real reconciliation)

---

## Test Implementation Order

### Phase 1: Unit Tests (Week 1, Days 1-2)
**Priority**: Detectors and ConfigMap generation
**Tests**: 65 unit tests

1. Prometheus detector (5 tests)
2. Grafana detector (5 tests)
3. Jaeger detector (4 tests)
4. Elasticsearch detector (4 tests)
5. Custom detector (6 tests)
6. ConfigMap generators (17 tests)
7. Health checks (8 unit tests)

### Phase 2: Integration Tests (Week 1, Days 3-4)
**Priority**: Kubernetes integration
**Tests**: 28 integration tests

1. Service discovery in Kind (8 tests)
2. ConfigMap creation/update (5 tests)
3. Reconciliation (10 tests)
4. HTTP API (5 tests)

### Phase 3: E2E Tests (Week 1, Day 5)
**Priority**: End-to-end validation
**Tests**: 2 E2E tests

1. Complete discovery flow
2. HolmesGPT API toolset consumption

---

## Success Criteria

### Unit Test Coverage: >70%
- All service detectors
- ConfigMap generation
- Drift detection logic
- Health check logic

### Integration Test Coverage: >50%
- Real Kubernetes service discovery
- ConfigMap reconciliation
- HTTP API endpoints

### E2E Test Coverage: <10%
- Complete discovery and toolset generation flow
- HolmesGPT API integration

---

**Document Status**: ✅ BR Test Strategy Complete
**Last Updated**: October 10, 2025
**Estimated Total Tests**: 95 (65 unit + 28 integration + 2 E2E)
**Confidence**: 90% (Very High)

