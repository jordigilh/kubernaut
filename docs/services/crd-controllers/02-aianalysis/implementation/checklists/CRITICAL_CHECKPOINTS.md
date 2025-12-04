# AI Analysis Service - Critical Checkpoints

**Parent Document**: [IMPLEMENTATION_PLAN_V1.0.md](../IMPLEMENTATION_PLAN_V1.0.md)
**Version**: 1.0

---

## 🚨 **Gateway Checkpoints**

These are blocking checkpoints that must pass before proceeding to the next phase.

### **Checkpoint 1: Day 1 Foundation Gate**

**When**: End of Day 1
**Blocker**: Cannot proceed to Day 2 without passing

| Check | Command | Expected |
|-------|---------|----------|
| Controller builds | `go build ./cmd/aianalysis/...` | Exit 0 |
| Tests pass | `go test ./internal/controller/aianalysis/...` | PASS |
| Lint clean | `golangci-lint run ./internal/controller/aianalysis/...` | Exit 0 |
| Main binary runs | `./bin/aianalysis --help` | Help text |

**Recovery if Failed**:
1. Check import paths
2. Verify CRD types exist
3. Review controller scaffolding against DD-006

---

### **Checkpoint 2: Day 4 Core Complete Gate**

**When**: End of Day 4
**Blocker**: Cannot proceed to metrics without passing

| Check | Command | Expected |
|-------|---------|----------|
| All handlers build | `go build ./pkg/aianalysis/...` | Exit 0 |
| Handler tests pass | `go test ./pkg/aianalysis/phases/...` | PASS |
| Rego engine tests | `go test ./pkg/aianalysis/rego/...` | PASS |
| HolmesGPT client tests | `go test ./pkg/aianalysis/holmesgpt/...` | PASS |

**Recovery if Failed**:
1. Review handler implementations
2. Check Rego policy syntax
3. Verify HolmesGPT client type mappings

---

### **Checkpoint 3: Day 6 Unit Test Gate**

**When**: End of Day 6
**Blocker**: Cannot proceed to integration tests without passing

| Check | Command | Expected |
|-------|---------|----------|
| Coverage ≥70% | `go test -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥70% |
| All tests pass | `go test ./test/unit/aianalysis/... -v` | PASS |
| No lint errors | `golangci-lint run` | Exit 0 |

**Recovery if Failed**:
1. Add missing test cases
2. Review table-driven test coverage
3. Check edge cases

---

### **Checkpoint 4: Day 8 Integration Gate**

**When**: End of Day 8
**Blocker**: Cannot proceed to documentation without passing

| Check | Command | Expected |
|-------|---------|----------|
| KIND cluster up | `kind get clusters` | aianalysis-test |
| CRD installed | `kubectl get crd aianalyses.aianalysis.kubernaut.io` | Found |
| Integration pass | `INTEGRATION_TEST=true go test ./test/integration/aianalysis/...` | PASS |
| E2E pass | `E2E_TEST=true go test ./test/e2e/aianalysis/...` | PASS |

**Recovery if Failed**:
1. Check KIND cluster logs
2. Verify MockLLMServer is running
3. Review CRD installation

---

### **Checkpoint 5: Day 10 Production Gate**

**When**: End of Day 10
**Blocker**: Cannot release without passing

| Check | Command | Expected |
|-------|---------|----------|
| All tests pass | `make test` | Exit 0 |
| Coverage ≥70% | Coverage report | ≥70% |
| Lint clean | `make lint` | Exit 0 |
| Docs complete | Manual review | ✅ |
| Runbooks exist | `ls docs/services/crd-controllers/02-aianalysis/runbooks/` | 3 files |
| Health endpoints | `curl localhost:8081/healthz` | 200 OK |
| Metrics exposed | `curl localhost:9090/metrics \| grep aianalysis` | Found |

**Recovery if Failed**:
1. Review production checklist
2. Fix outstanding issues
3. Extend timeline if needed

---

## 📊 **Checkpoint Status Tracking**

| Checkpoint | Target Date | Status | Actual Date | Notes |
|------------|-------------|--------|-------------|-------|
| Day 1 Foundation | — | ⏳ Pending | — | — |
| Day 4 Core Complete | — | ⏳ Pending | — | — |
| Day 6 Unit Tests | — | ⏳ Pending | — | — |
| Day 8 Integration | — | ⏳ Pending | — | — |
| Day 10 Production | — | ⏳ Pending | — | — |

---

## 🔧 **Common Checkpoint Failures**

### **Failure: Import Cycle**
```
import cycle not allowed
package github.com/jordigilh/kubernaut/pkg/aianalysis/phases
    imports github.com/jordigilh/kubernaut/internal/controller/aianalysis
```

**Fix**: Move shared types to `pkg/shared/types/`

### **Failure: Missing DeepCopy**
```
undefined: aianalysisv1.AIAnalysis.DeepCopy
```

**Fix**: Run `make generate`

### **Failure: CRD Validation**
```
spec.validation.openAPIV3Schema.properties[spec].properties[analysisRequest]: Required value: must have a type
```

**Fix**: Add `// +kubebuilder:validation:Required` markers

### **Failure: Coverage Below 70%**
```
total: (statements) 65.3%
```

**Fix**: Add table-driven tests for edge cases

---

## 📚 **References**

| Document | Purpose |
|----------|---------|
| [TESTING_GUIDELINES.md](../../../../development/business-requirements/TESTING_GUIDELINES.md) | Coverage requirements |
| [APPENDIX_C_CONFIDENCE_METHODOLOGY.md](../appendices/APPENDIX_C_CONFIDENCE_METHODOLOGY.md) | Recovery actions |

