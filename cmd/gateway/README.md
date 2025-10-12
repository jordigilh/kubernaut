# Gateway Service Binary

The Gateway Service is the entry point for all monitoring alerts and events entering the Kubernaut system.

## Overview

The Gateway Service:
- **Receives alerts** from Prometheus AlertManager, Kubernetes events, and other monitoring sources
- **Normalizes** diverse signal formats into a unified representation
- **Deduplicates** alerts using Redis (5-minute TTL)
- **Detects storms** (rate and pattern-based) to prevent AI overload
- **Classifies environments** using namespace labels and ConfigMaps
- **Assigns priorities** (P0-P3) based on environment and severity
- **Creates RemediationRequest CRDs** for downstream processing
- **Provides observability** via Prometheus metrics and structured logs

## Architecture

```
┌─────────────────┐
│ AlertManager    │
│ (Prometheus)    │
└────────┬────────┘
         │ HTTP POST /api/v1/signals/prometheus
         ▼
┌─────────────────────────────────────────────────────┐
│              Gateway Service                        │
│                                                     │
│  ┌──────────────────────────────────────────────┐ │
│  │  1. Authentication (TokenReview)             │ │
│  │  2. Rate Limiting (per-source IP)            │ │
│  │  3. Adapter (Prometheus/K8s Events)          │ │
│  │  4. Deduplication (Redis, 5-min TTL)         │ │
│  │  5. Storm Detection (rate + pattern)         │ │
│  │  6. Environment Classification               │ │
│  │  7. Priority Assignment                      │ │
│  │  8. RemediationRequest CRD Creation          │ │
│  └──────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────┐
│ RemediationRequest CRD │
│ (in Kubernetes)        │
└────────────────────────┘
```

## Building

```bash
# Build binary
make build-gateway

# Or build manually
go build -o bin/gateway ./cmd/gateway
```

## Running

### Development (Local)

```bash
# Set required environment variables
export GATEWAY_REDIS_ADDR="localhost:6379"
export LOG_LEVEL="debug"

# Run binary
./bin/gateway
```

### Production (Kubernetes)

The Gateway runs as a Kubernetes Deployment with:
- **ServiceAccount**: `gateway-sa` (for TokenReview authentication)
- **RBAC**: Read access to namespaces, ConfigMaps, RemediationRequest CRDs
- **Redis**: Deployed as StatefulSet (persistent deduplication)
- **Service**: ClusterIP with optional Ingress for external AlertManagers

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: gateway
  namespace: kubernaut-system
spec:
  replicas: 2  # High availability
  template:
    spec:
      serviceAccountName: gateway-sa
      containers:
      - name: gateway
        image: kubernaut/gateway:v1.0.0
        ports:
        - containerPort: 8080
          name: http
        - containerPort: 9090
          name: metrics
        env:
        - name: GATEWAY_REDIS_ADDR
          value: "redis-service:6379"
        - name: LOG_LEVEL
          value: "info"
        resources:
          requests:
            memory: "256Mi"
            cpu: "200m"
          limits:
            memory: "512Mi"
            cpu: "500m"
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `GATEWAY_LISTEN_ADDR` | `:8080` | HTTP server listen address |
| `GATEWAY_REDIS_ADDR` | `localhost:6379` | Redis server address |
| `GATEWAY_REDIS_PASSWORD` | `""` | Redis password (if required) |
| `GATEWAY_REDIS_DB` | `0` | Redis database number |
| `GATEWAY_RATE_LIMIT` | `100` | Rate limit (alerts/minute per source) |
| `GATEWAY_RATE_LIMIT_BURST` | `20` | Rate limit burst capacity |
| `GATEWAY_DEDUP_TTL_SECONDS` | `300` | Deduplication TTL (5 minutes) |
| `GATEWAY_STORM_RATE_THRESHOLD` | `10` | Rate storm threshold (alerts/minute) |
| `GATEWAY_STORM_PATTERN_THRESHOLD` | `5` | Pattern storm threshold (similar alerts) |
| `GATEWAY_STORM_WINDOW_SECONDS` | `60` | Storm aggregation window (1 minute) |
| `GATEWAY_ENV_CACHE_TTL_SECONDS` | `30` | Environment classification cache TTL |
| `GATEWAY_ENV_CONFIGMAP_NAMESPACE` | `kubernaut-system` | Environment ConfigMap namespace |
| `GATEWAY_ENV_CONFIGMAP_NAME` | `kubernaut-environment-overrides` | Environment ConfigMap name |
| `LOG_LEVEL` | `info` | Log level: debug, info, warn, error |

### Config File (Optional)

```yaml
# gateway-config.yaml
listen_addr: ":8080"
read_timeout: 30s
write_timeout: 30s
idle_timeout: 120s

rate_limit_requests_per_minute: 100
rate_limit_burst: 20

redis:
  addr: "redis-service:6379"
  password: ""
  db: 0
  pool_size: 100

deduplication_ttl: 5m
storm_rate_threshold: 10
storm_pattern_threshold: 5
storm_aggregation_window: 1m
environment_cache_ttl: 30s

env_configmap_namespace: "kubernaut-system"
env_configmap_name: "kubernaut-environment-overrides"
```

Load config file:
```bash
export GATEWAY_CONFIG_FILE="gateway-config.yaml"
./bin/gateway
```

## Endpoints

### Signal Ingestion

- **POST** `/api/v1/signals/prometheus` - Prometheus AlertManager webhook
- **POST** `/api/v1/signals/kubernetes` - Kubernetes Event webhook (future)

### Health & Observability

- **GET** `/health` - Health check (returns 200 OK)
- **GET** `/healthz` - Alias for `/health`
- **GET** `/metrics` - Prometheus metrics (on port 9090 by default)

### Metrics

The Gateway exposes 15+ Prometheus metrics:

| Metric | Type | Description |
|---|---|---|
| `gateway_signals_received_total` | Counter | Total signals received by adapter |
| `gateway_signals_processed_total` | Counter | Total signals processed successfully |
| `gateway_remediationrequests_created_total` | Counter | Total RemediationRequest CRDs created |
| `gateway_deduplication_hits_total` | Counter | Total deduplicated signals |
| `gateway_storm_detected_total` | Counter | Total storms detected |
| `gateway_rate_limit_exceeded_total` | Counter | Total rate limit violations |
| `gateway_signal_processing_duration_seconds` | Histogram | Signal processing latency |

## Testing

### Unit Tests

```bash
# Run Gateway unit tests
make test-gateway

# Or manually
go test ./pkg/gateway/...
```

### Integration Tests

```bash
# Setup test environment (Kind + Redis)
make test-gateway-setup

# Run integration tests
make test-gateway-integration

# Cleanup
make test-gateway-cleanup
```

## Business Requirements Covered

- **BR-GATEWAY-001**: Alert ingestion from Prometheus AlertManager
- **BR-GATEWAY-002**: Kubernetes Event integration (adapter ready)
- **BR-GATEWAY-004**: Rate limiting (100 alerts/min per source)
- **BR-GATEWAY-010**: Deduplication (5-minute TTL, Redis-based)
- **BR-GATEWAY-015**: Storm detection (rate-based)
- **BR-GATEWAY-016**: Storm aggregation (1-minute windows)
- **BR-GATEWAY-023**: RemediationRequest CRD creation
- **BR-GATEWAY-051**: Environment classification (namespace labels)
- **BR-GATEWAY-052**: Environment classification (ConfigMap overrides)
- **BR-GATEWAY-053**: Priority assignment (P0-P3)

## Troubleshooting

### Redis Connection Issues

```bash
# Check Redis connectivity
redis-cli -h localhost -p 6379 ping

# View Gateway logs
kubectl logs -f deployment/gateway -n kubernaut-system

# Check Redis keys
redis-cli -h localhost -p 6379 keys "alert:*"
```

### Authentication Failures

```bash
# Verify ServiceAccount has correct permissions
kubectl auth can-i create remediationrequests.remediation.kubernaut.io \
  --as system:serviceaccount:kubernaut-system:gateway-sa

# Check TokenReview API
kubectl get --raw /apis/authentication.k8s.io/v1/tokenreviews
```

### Rate Limiting

```bash
# Check rate limit metrics
curl http://localhost:9090/metrics | grep rate_limit

# Temporarily increase limits (not recommended for production)
export GATEWAY_RATE_LIMIT=1000
export GATEWAY_RATE_LIMIT_BURST=100
```

### Storm Detection False Positives

```bash
# Increase thresholds if legitimate burst traffic is being aggregated
export GATEWAY_STORM_RATE_THRESHOLD=50
export GATEWAY_STORM_PATTERN_THRESHOLD=20
```

## Production Recommendations

1. **High Availability**: Run 2+ replicas with anti-affinity
2. **Resource Limits**: 256Mi-512Mi memory, 200m-500m CPU
3. **Redis**: Use StatefulSet with persistent volume
4. **Monitoring**: Alert on `gateway_remediationrequests_creation_failures_total`
5. **Rate Limiting**: Tune based on alert volume (100-500 alerts/min typical)
6. **Storm Detection**: Monitor `gateway_storm_detected_total` for patterns
7. **Logging**: Use `info` level in production, `debug` for troubleshooting

## Security

- **Authentication**: All requests require valid Kubernetes ServiceAccount token
- **Authorization**: TokenReview API validates caller permissions
- **Network**: Deploy behind Ingress with TLS for external AlertManagers
- **Redis**: Use password authentication and TLS in production
- **Rate Limiting**: Prevents DoS attacks (100 alerts/min per source)

## Performance

- **Throughput**: 1000+ alerts/second (with 100-connection Redis pool)
- **Latency**: <50ms p95 (including Redis, K8s API, CRD creation)
- **Memory**: ~256Mi baseline, +1MB per 1000 active deduplication keys
- **Redis**: ~1KB per deduplicated alert (5-minute TTL)

## Links

- [Gateway Service Documentation](../../docs/services/stateless/gateway-service/)
- [Integration Tests](../../test/integration/gateway/)
- [API Specification](../../docs/services/stateless/gateway-service/api-specification.md)
- [CRD Schema](../../api/remediation/v1alpha1/remediationrequest_types.go)

