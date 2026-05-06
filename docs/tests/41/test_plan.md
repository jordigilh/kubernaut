# Test Plan: SLO Definitions, Alerting Rules, and Histogram Bucket Alignment

**Test Plan Identifier:** TP-AF-041
**Issue:** [#41](https://github.com/jordigilh/kubernaut-apifrontend/issues/41)
**Version:** 1.0
**Date:** 2026-05-06
**Status:** Draft

---

## 1. Introduction

This test plan validates the SLO definitions documentation, PrometheusRule alerting manifests, and histogram bucket alignment with SLO thresholds for the kubernaut API Frontend.

### 1.1 Scope

- `docs/slo/SLO_DEFINITIONS.md` documenting targets, metrics, and measurement methods
- `deploy/prometheus-rules.yaml` with alerting rules derived from SLOs
- Verification that `prometheus.DefBuckets` align with SLO thresholds
- YAML syntax validation of PrometheusRule manifest

### 1.2 References

- Issue #41: SLO definitions for tool call latency, SSE, and authentication
- `internal/metrics/metrics.go` — histogram definitions using `prometheus.DefBuckets`
- kubernaut `deploy/manifests/monitoring.yaml` — reference PrometheusRule
- `prometheus.DefBuckets` = {.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

### 1.3 Definitions

| Term | Definition |
|------|-----------|
| SLO | Service Level Objective — target performance/availability threshold |
| PrometheusRule | Kubernetes CRD (`monitoring.coreos.com/v1`) defining alerting rules |
| DefBuckets | Default Prometheus histogram boundaries |
| Burn rate | Rate of SLO budget consumption |

---

## 2. Test Items

| Item | Location | Source |
|------|----------|--------|
| SLO_DEFINITIONS.md | `docs/slo/` | New |
| prometheus-rules.yaml | `deploy/` | New |
| Histogram bucket documentation | SLO_DEFINITIONS.md | New |

---

## 3. Features to Be Tested

### 3.1 Business Acceptance Criteria

| ID | Criterion | Testable |
|----|-----------|----------|
| BAC-1 | SLO targets documented | Yes (file review) |
| BAC-2 | Prometheus metrics defined for each SLO | Yes (cross-ref with metrics.go) |
| BAC-3 | Histogram buckets aligned with SLO thresholds | Yes (DefBuckets analysis) |
| BAC-4 | Alerting rule definitions ready for Prometheus/Alertmanager | Yes (YAML validation) |

### 3.2 Features by Tier

#### Tier 1: SLO Documentation

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-41.1 | SLO_DEFINITIONS.md exists | File present with all SLO targets |
| F-41.2 | Each SLO has target, metric, and measurement | Table complete |
| F-41.3 | SLOs reference actual af_* metric names | Names match metrics.go |

#### Tier 2: Alerting Rules

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-41.4 | PrometheusRule YAML is valid K8s manifest | apiVersion, kind, metadata, spec present |
| F-41.5 | Availability alert: up == 0 for 5m | Critical severity |
| F-41.6 | Latency alert: p95 > 500ms for 2m | Warning severity |
| F-41.7 | Error rate alert: 5xx > 1% for 2m | Critical severity |
| F-41.8 | Auth failure spike: > 10% for 2m | Critical severity |

#### Tier 3: Bucket Alignment

| ID | Feature | Acceptance Criteria |
|----|---------|-------------------|
| F-41.9 | DefBuckets cover p50 SLO (100ms) | Bucket at 0.1 exists |
| F-41.10 | DefBuckets cover p95 SLO (500ms) | Bucket at 0.5 exists |
| F-41.11 | DefBuckets cover p99 SLO (1s) | Bucket at 1.0 exists |
| F-41.12 | Alignment documented in SLO_DEFINITIONS.md | Table maps SLO to bucket |

---

## 4. Features Not Tested

| Feature | Reason |
|---------|--------|
| Alert firing in live Prometheus | Requires running cluster |
| SLO budget burn rate alerting | Deferred to operational maturity |
| Grafana dashboard | Separate deliverable |
| SLO adjustment after measurement | Deferred per issue (step 4) |

---

## 5. Approach

### 5.1 Test Methodology

This issue is primarily documentation and configuration. Validation approach:
- **YAML syntax**: `kubectl --dry-run=client -f deploy/prometheus-rules.yaml` or manual review
- **Cross-reference**: Script/test that verifies metric names in rules match `internal/metrics/metrics.go`
- **Bucket alignment**: Documented analysis (no code change needed)

### 5.2 Test Levels

| Level | Scope | Tools |
|-------|-------|-------|
| Static | YAML validation, cross-reference | Manual review, grep |
| Unit | Metric name consistency | Go test verifying registered metric names |

---

## 6. Test Cases

### 6.1 SLO Documentation (3 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-041-001 | SLO_DEFINITIONS.md contains all 7 SLO targets from issue | P0 | BAC-1 |
| UT-AF-041-002 | Each SLO row has target percentile, threshold, and metric name | P0 | BAC-1 |
| UT-AF-041-003 | Metric names in SLO doc match af_* names in metrics.go | P0 | BAC-2 |

### 6.2 Alerting Rules (5 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-041-004 | prometheus-rules.yaml has apiVersion monitoring.coreos.com/v1 | P0 | BAC-4 |
| UT-AF-041-005 | Availability alert: expr uses up{job="apifrontend"} == 0 | P0 | BAC-4 |
| UT-AF-041-006 | Latency alert: expr uses histogram_quantile(0.95, rate(af_http_request_duration_seconds_bucket[5m])) | P0 | BAC-4 |
| UT-AF-041-007 | Error rate alert: expr uses sum(rate(af_http_requests_total{status=~"5.."}[5m])) | P0 | BAC-4 |
| UT-AF-041-008 | All alerts have severity label and annotations | P0 | BAC-4 |

### 6.3 Bucket Alignment (3 tests)

| ID | Description | Priority | BAC |
|----|-------------|----------|-----|
| UT-AF-041-009 | DefBuckets contain boundary at or below p50 SLO threshold | P0 | BAC-3 |
| UT-AF-041-010 | DefBuckets contain boundary at or below p95 SLO threshold | P0 | BAC-3 |
| UT-AF-041-011 | DefBuckets contain boundary at or below p99 SLO threshold | P0 | BAC-3 |

---

## 7. Pass/Fail Criteria

### 7.1 Pass

- SLO_DEFINITIONS.md complete with all targets
- PrometheusRule YAML valid (parseable, correct structure)
- All metric names cross-reference correctly
- DefBuckets alignment documented and verified

### 7.2 Fail

- Missing SLO target for any metric
- PrometheusRule has syntax errors
- Metric name mismatch between rules and code
- No bucket at SLO boundary (would require bucket change)

---

## 8. Test Deliverables

| Deliverable | Location |
|-------------|----------|
| SLO definitions | `docs/slo/SLO_DEFINITIONS.md` |
| Alerting rules | `deploy/prometheus-rules.yaml` |
| This test plan | `docs/tests/41/test_plan.md` |

---

## 9. Traceability Matrix

| BAC | Test Cases | Count |
|-----|-----------|-------|
| BAC-1 | UT-AF-041-001, 002 | 2 |
| BAC-2 | UT-AF-041-003 | 1 |
| BAC-3 | UT-AF-041-009, 010, 011 | 3 |
| BAC-4 | UT-AF-041-004 to 008 | 5 |
