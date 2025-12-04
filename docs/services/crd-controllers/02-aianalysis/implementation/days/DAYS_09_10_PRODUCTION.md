# AI Analysis Service - Days 9-10: Production Readiness

**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md)
**Duration**: 16 hours (2 days × 8h)
**Phase**: Production Readiness
**Methodology**: CHECK Phase

---

## 📋 **Overview**

| Day | Focus | Key Deliverables |
|-----|-------|------------------|
| **Day 9** | Documentation | API docs, runbooks, troubleshooting |
| **Day 10** | Final Validation | Production checklist, handoff |

---

## 📅 **Day 9: Documentation (8h)**

### **Objectives**
- API documentation updates
- Production runbooks
- Error handling philosophy
- Troubleshooting guide

### **Hour-by-Hour Breakdown**

#### **Hours 1-2: API Documentation (2h)**

**File**: `docs/services/crd-controllers/02-aianalysis/api-reference.md`

```markdown
# AIAnalysis API Reference

## CRD Spec

### AIAnalysisSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `remediationRequestRef` | `ObjectReference` | ✅ | Reference to parent RemediationRequest |
| `remediationID` | `string` | ✅ | Unique remediation identifier |
| `analysisRequest` | `AnalysisRequest` | ✅ | Analysis request parameters |
| `isRecoveryAttempt` | `bool` | ❌ | Whether this is a recovery attempt |
| `recoveryAttemptNumber` | `int` | ❌ | Current recovery attempt number |
| `previousExecutions` | `[]PreviousExecution` | ❌ | Previous execution history |

### SignalContextInput

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `fingerprint` | `string` | ✅ | SHA256 signal fingerprint |
| `signalType` | `string` | ✅ | Signal type (OOMKilled, CrashLoopBackOff, etc.) |
| `environment` | `string` | ✅ | Environment name |
| `businessPriority` | `string` | ✅ | Business priority (P0-P4) |
| `targetResource` | `TargetResource` | ✅ | Affected resource |
| `enrichmentResults` | `EnrichmentResults` | ❌ | Enrichment data from SignalProcessing |

### AIAnalysisStatus

| Field | Type | Description |
|-------|------|-------------|
| `phase` | `string` | Current phase (pending, validating, investigating, analyzing, recommending, completed, failed) |
| `message` | `string` | Human-readable status message |
| `rootCause` | `string` | Identified root cause |
| `confidence` | `float64` | AI confidence score (0.0-1.0) |
| `selectedWorkflow` | `SelectedWorkflow` | Recommended workflow |
| `approvalRequired` | `bool` | Whether manual approval is needed |
| `approvalDecision` | `string` | Policy decision (auto_approve, manual_approval, reject) |
| `targetInOwnerChain` | `*bool` | Whether target was found in owner chain |
| `warnings` | `[]string` | Non-fatal warnings from analysis |
```

#### **Hours 3-5: Production Runbooks (3h)**

**File**: `docs/services/crd-controllers/02-aianalysis/runbooks/RUNBOOK_01_HIGH_LATENCY.md`

```markdown
# Runbook: AIAnalysis High Latency

## Symptoms
- `aianalysis_phase_duration_seconds{phase="investigating"}` > 60s
- `aianalysis_holmesgpt_latency_seconds` p95 > 30s
- AIAnalysis CRDs stuck in `investigating` phase

## Root Causes

### 1. HolmesGPT-API Slow Response
**Likelihood**: High

**Check**:
```bash
# Check HolmesGPT-API latency
curl -w "@curl-format.txt" http://holmesgpt-api:8080/health

# Check metrics
kubectl exec -n kubernaut deploy/aianalysis -- \
  curl localhost:9090/metrics | grep holmesgpt_latency
```

**Resolution**:
1. Check HolmesGPT-API logs for errors
2. Verify MockLLM server is responsive
3. Scale HolmesGPT-API if needed

### 2. Rego Policy Evaluation Slow
**Likelihood**: Medium

**Check**:
```bash
kubectl exec -n kubernaut deploy/aianalysis -- \
  curl localhost:9090/metrics | grep rego_policy_latency
```

**Resolution**:
1. Check policy complexity
2. Review ConfigMap for syntax errors
3. Restart controller to reload policy

### 3. Kubernetes API Throttling
**Likelihood**: Low

**Check**:
```bash
kubectl get --raw /apis/apiregistration.k8s.io/v1/apiservices | jq '.items[].status'
```

**Resolution**:
1. Check API server logs
2. Review rate limits
3. Contact cluster admin

## Escalation
- **L1**: Check runbook steps
- **L2**: Review HolmesGPT-API team
- **L3**: Platform team
```

**File**: `docs/services/crd-controllers/02-aianalysis/runbooks/RUNBOOK_02_APPROVAL_FAILURES.md`

```markdown
# Runbook: AIAnalysis Approval Failures

## Symptoms
- `aianalysis_approval_decisions_total{decision="reject"}` increasing
- AIAnalysis CRDs failing in `analyzing` phase
- Unexpected manual approval requirements

## Root Causes

### 1. Rego Policy Syntax Error
**Likelihood**: High

**Check**:
```bash
# Check controller logs for Rego errors
kubectl logs -n kubernaut deploy/aianalysis | grep -i rego

# Check ConfigMap
kubectl get configmap -n kubernaut aianalysis-rego-policy -o yaml
```

**Resolution**:
1. Fix Rego syntax
2. Apply corrected ConfigMap
3. Wait for hot-reload (5s interval)

### 2. Low AI Confidence
**Likelihood**: Medium

**Check**:
```bash
kubectl get aianalysis -n <ns> <name> -o jsonpath='{.status.confidence}'
```

**Resolution**:
1. Review HolmesGPT-API response
2. Check if signal type is supported
3. Verify enrichment data quality

### 3. Policy Threshold Misconfiguration
**Likelihood**: Medium

**Check**:
```bash
# Review policy thresholds
kubectl get configmap -n kubernaut aianalysis-rego-policy -o yaml | \
  grep -A5 "confidence_threshold"
```

**Resolution**:
1. Adjust confidence thresholds
2. Update environment-specific rules
3. Test with dry-run

## Escalation
- **L1**: Check runbook steps
- **L2**: Review Rego policy with team
- **L3**: Policy design review
```

#### **Hours 6-8: Troubleshooting Guide (3h)**

**File**: `docs/services/crd-controllers/02-aianalysis/troubleshooting.md`

```markdown
# AIAnalysis Troubleshooting Guide

## Quick Diagnostics

### Check Controller Health
```bash
# Pod status
kubectl get pods -n kubernaut -l app=aianalysis

# Health endpoints
kubectl exec -n kubernaut deploy/aianalysis -- curl localhost:8081/healthz
kubectl exec -n kubernaut deploy/aianalysis -- curl localhost:8081/readyz

# Recent logs
kubectl logs -n kubernaut deploy/aianalysis --tail=100
```

### Check AIAnalysis Status
```bash
# List all AIAnalysis
kubectl get aianalysis -A

# Describe specific AIAnalysis
kubectl describe aianalysis -n <ns> <name>

# Get status JSON
kubectl get aianalysis -n <ns> <name> -o jsonpath='{.status}' | jq
```

### Check Metrics
```bash
kubectl exec -n kubernaut deploy/aianalysis -- curl localhost:9090/metrics | grep -E "^aianalysis_"
```

## Common Issues

### Issue: AIAnalysis Stuck in Validating
**Cause**: Missing required fields in SignalContext
**Fix**: Check `fingerprint`, `signalType`, `environment`, `businessPriority`, `targetResource`

### Issue: AIAnalysis Stuck in Investigating
**Cause**: HolmesGPT-API unreachable or slow
**Fix**: Check HolmesGPT-API health, network connectivity

### Issue: Invalid FailedDetections Error
**Cause**: Unknown field name in `failedDetections` array
**Fix**: Use only valid fields: gitOpsManaged, pdbProtected, hpaEnabled, stateful, helmManaged, networkIsolated, serviceMesh

### Issue: Unexpected Manual Approval Required
**Cause**: Confidence below threshold or policy mismatch
**Fix**: Review Rego policy, check confidence score
```

---

## 📅 **Day 10: Production Readiness (8h)**

### **Objectives**
- Complete production checklist
- Final validation
- Handoff documentation
- Confidence assessment

### **Production Readiness Checklist**

#### **Code Quality**
- [ ] All code compiles without warnings
- [ ] golangci-lint passes with zero errors
- [ ] No TODO/FIXME comments in production paths
- [ ] Error handling follows philosophy document

#### **Testing**
- [ ] Unit test coverage ≥ 70%
- [ ] All integration tests pass
- [ ] E2E tests validate critical paths
- [ ] No flaky tests

#### **CRD Controller**
- [ ] Finalizer cleanup works correctly
- [ ] Status updates are idempotent
- [ ] Phase transitions are logged
- [ ] Events are recorded appropriately

#### **Observability**
- [ ] All metrics exposed at `/metrics`
- [ ] Structured logging (logr) used consistently
- [ ] Health endpoints respond correctly
- [ ] Audit events recorded

#### **Configuration**
- [ ] No hardcoded values
- [ ] Environment variables documented
- [ ] ConfigMap references work
- [ ] RBAC permissions minimal and documented

#### **Integration**
- [ ] HolmesGPT-API integration tested
- [ ] Rego policy hot-reload works
- [ ] Cross-CRD coordination validated
- [ ] Recovery flow tested

#### **Documentation**
- [ ] API reference complete
- [ ] 3 runbooks created
- [ ] Troubleshooting guide complete
- [ ] Error handling philosophy documented

#### **Security**
- [ ] RBAC roles minimal
- [ ] No sensitive data in logs
- [ ] Network policies reviewed
- [ ] Secrets handling validated

### **Final Validation Commands**

```bash
# Build validation
go build ./cmd/aianalysis/...
golangci-lint run ./internal/controller/aianalysis/... ./pkg/aianalysis/...

# Test validation
go test ./test/unit/aianalysis/... -v -coverprofile=coverage.out
go tool cover -func=coverage.out | grep total

# Integration test (requires KIND)
INTEGRATION_TEST=true go test ./test/integration/aianalysis/... -v -timeout 10m

# E2E test (requires full stack)
E2E_TEST=true go test ./test/e2e/aianalysis/... -v -timeout 15m

# Verify CRD
kubectl apply --dry-run=server -f config/crd/bases/aianalysis.kubernaut.io_aianalyses.yaml

# Check metrics
curl -s localhost:9090/metrics | grep -c "^aianalysis_"
```

### **Day 10 EOD Checklist**
- [ ] Production readiness checklist 100% complete
- [ ] All tests passing
- [ ] Documentation complete
- [ ] Handoff notes written
- [ ] Confidence assessment: 95%+
- [ ] Sign-off obtained

---

## 📊 **Final Confidence Assessment**

See [APPENDIX_C_CONFIDENCE_METHODOLOGY.md](../appendices/APPENDIX_C_CONFIDENCE_METHODOLOGY.md) for Day 10 template.

### **Target**: 95%+

| Category | Points | Evidence |
|----------|--------|----------|
| Base Score | 50% | Starting point |
| Build Success | +10% | Clean build |
| Lint Compliance | +5% | Zero errors |
| Unit Test Coverage | +15% | ≥70% |
| Integration Tests | +10% | All pass |
| E2E Tests | +5% | Critical paths |
| Documentation | +5% | Complete |
| Pattern Compliance | +10% | All DDs followed |
| **Subtotal** | 110% | (capped at 100%) |
| Risk Deductions | -2% | Minor tech debt |
| **Final** | **98%** | |

---

## 📚 **References**

| Document | Purpose |
|----------|---------|
| [Production Readiness Checklist](../../implementation-checklist.md) | Full checklist |
| [APPENDIX_C_CONFIDENCE_METHODOLOGY.md](../appendices/APPENDIX_C_CONFIDENCE_METHODOLOGY.md) | Assessment method |
| [testing-strategy.md](../../testing-strategy.md) | Test requirements |

