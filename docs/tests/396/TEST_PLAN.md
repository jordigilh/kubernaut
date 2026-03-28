# Test Plan: Include Mounted ConfigMap Content in RO/EM Spec Hash Computation

**Feature**: Extend the canonical spec hash (DD-EM-002) to include ConfigMap data referenced by the target resource's pod spec, enabling drift detection for ConfigMap content changes
**Version**: 1.0
**Created**: 2026-03-04
**Author**: AI Assistant
**Status**: Ready for Execution
**Branch**: `development/v1.2`

**Authority**:
- BR-EM-004: Spec hash comparison to detect configuration drift
- DD-EM-002: Canonical Spec Hash for Pre/Post Remediation Comparison
- [#396](https://github.com/jordigilh/kubernaut/issues/396): Include mounted ConfigMap content in RO/EM spec hash computation

**Cross-References**:
- [Testing Strategy](../../../.cursor/rules/03-testing-strategy.mdc)
- [Test Case Specification Template](../../testing/TEST_CASE_SPECIFICATION_TEMPLATE.md)
- [Integration/E2E No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md)
- [DD-EM-002: Canonical Spec Hash](../../architecture/decisions/DD-EM-002-canonical-spec-hash.md)
- GitHub Issue: [#396](https://github.com/jordigilh/kubernaut/issues/396)

---

## 1. Scope

### In Scope

- **Shared hash utility** (`pkg/shared/hash/configmap.go`): New `ExtractConfigMapRefs`, `ConfigMapDataHash`, `CompositeSpecHash` functions for extracting ConfigMap references from unstructured specs, hashing ConfigMap data deterministically, and producing a single composite digest
- **EM hash computer** (`pkg/effectivenessmonitor/hash/hash.go`): Extend `SpecHashInput` with optional `ConfigMapData` field; `Computer.Compute` uses composite hash when ConfigMap data is present
- **RO pre-remediation hash** (`internal/controller/remediationorchestrator/reconciler.go`): `CapturePreRemediationHash` resolves ConfigMap references from the target spec, fetches ConfigMap data from the K8s API, and includes it in the hash
- **EM post-remediation hash** (`internal/controller/effectivenessmonitor/reconciler.go`): `assessHash`, `getTargetSpec`, and drift guard (Step 6.5) include ConfigMap data in hash computation
- **Helm RBAC** (`charts/kubernaut/templates/`): Add `configmaps` to RO and EM ClusterRoles for cross-namespace ConfigMap reads

### Out of Scope

- **Secrets**: Excluded from hash computation for security reasons (secrets should not be logged or hashed in audit trails)
- **E2E tests**: Hash computation is infrastructure-level logic with no new CRDs or API endpoints; UT + IT provide sufficient defense-in-depth
- **Backward compatibility**: First assessment post-upgrade shows a one-time hash mismatch (accepted design decision -- no versioning or fallback logic)
- **ConfigMap caching**: Accept N extra K8s API GETs per hash computation for v1.2; caching deferred to future optimization
- **DD-EM-002 document update**: Will be updated separately (v1.3) after implementation

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| Single composite digest stored, composite structure logged | Avoids storing variable-length data; debug log provides forensic visibility without storage risk |
| No backward compatibility | Eliminates hash versioning, mixed-version comparisons, and fallback logic. One-time drift on upgrade is acceptable and expected. |
| Absent ConfigMap sentinel `__absent:<configmap-name>__` | Deterministic hash for 404 (Not Found) and 403 (Forbidden) errors. Detects transitions when ConfigMap appears or disappears. |
| RBAC graceful degradation on 403 | Treat as absent (sentinel + warning log) instead of failing the hash computation. Allows partial RBAC deployments. |
| Include `.binaryData` in hash | ConfigMaps have both `.data` (string) and `.binaryData` (bytes). Base64-encode binary values for deterministic serialization. Prevents silent drift blindness for binary ConfigMap content. |
| `CanonicalSpecHash` unchanged | New `CompositeSpecHash` wraps it. Existing tests and consumers unaffected. |
| `SpecHashInput.ConfigMapData` is optional | When nil/empty, `Computer.Compute` falls back to spec-only hash. Existing callers work without modification. |
| All 5 ConfigMap reference paths covered | `volumes[].configMap`, `volumes[].projected.sources[].configMap`, `containers[].envFrom[].configMapRef`, `containers[].env[].valueFrom.configMapKeyRef`, + same for `initContainers[]` |
| Kind-aware pod template extraction | Deployment/StatefulSet/DaemonSet/ReplicaSet/Job use `spec.template.spec`; CronJob uses `spec.jobTemplate.spec.template.spec`; Pod uses `spec` directly |

---

## 2. Coverage Policy

### Per-Tier Testable Code Coverage (>=80%)

Authority: `03-testing-strategy.mdc` -- Per-Tier Testable Code Coverage.

- **Unit**: >=80% of unit-testable code (ConfigMap ref extraction, data hashing, composite hashing, EM computer extension, RO hash capture with fake client)
- **Integration**: >=80% of integration-testable code (EM reconciler hash computation with envtest, drift guard, ConfigMap content change detection)

### 2-Tier Minimum

Every business requirement gap is covered by Unit + Integration tiers:
- **Unit tests** validate pure logic correctness (extraction paths, hash determinism, deduplication, sentinel behavior, backward compat) and RO hash capture with fake K8s client
- **Integration tests** validate EM reconciler behavior with envtest: composite hash storage in EA status, ConfigMap content change detection, drift guard with ConfigMap mutations

### Business Outcome Quality Bar

Tests validate business outcomes -- "drift detection correctly identifies ConfigMap content changes between pre and post remediation" and "hash remains stable when ConfigMap content is unchanged" -- not just code path coverage. Each test asserts what the operator or effectiveness assessment observes.

---

## 3. Testable Code Inventory

### Unit-Testable Code (pure logic, no I/O)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `pkg/shared/hash/configmap.go` (NEW) | `ExtractConfigMapRefs`, `ConfigMapDataHash`, `CompositeSpecHash` | ~95 |
| `pkg/effectivenessmonitor/hash/hash.go` | `SpecHashInput.ConfigMapData` field, `computer.Compute` composite path | ~15 |

### Integration-Testable Code (I/O, wiring, cross-component)

| File | Functions/Methods | Lines (approx) |
|------|-------------------|-----------------|
| `internal/controller/remediationorchestrator/reconciler.go` | `CapturePreRemediationHash` with ConfigMap resolution | ~30 |
| `internal/controller/effectivenessmonitor/reconciler.go` | `assessHash`, `getTargetSpec`, drift guard (Step 6.5) with ConfigMap data | ~30 |

### Helm RBAC (declarative YAML, no tests)

| File | Change | Lines (approx) |
|------|--------|-----------------|
| `charts/kubernaut/templates/remediationorchestrator/remediationorchestrator.yaml` | Add `configmaps` to ClusterRole `resources` | ~2 |
| `charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml` | Add `configmaps` to ClusterRole `resources` | ~2 |

---

## 4. BR Coverage Matrix

| BR ID | Description | Priority | Tier | Test ID | Status |
|-------|-------------|----------|------|---------|--------|
| BR-EM-004 | Extract ConfigMap names from volumes[].configMap in Deployment spec | P0 | Unit | UT-SH-396-001 | Pending |
| BR-EM-004 | Extract ConfigMap names from initContainers[].envFrom[].configMapRef | P0 | Unit | UT-SH-396-002 | Pending |
| BR-EM-004 | Extract ConfigMap names from volumes[].projected.sources[].configMap | P0 | Unit | UT-SH-396-003 | Pending |
| BR-EM-004 | Extract ConfigMap names from containers[].envFrom[].configMapRef | P0 | Unit | UT-SH-396-004 | Pending |
| BR-EM-004 | Extract ConfigMap names from containers[].env[].valueFrom.configMapKeyRef | P0 | Unit | UT-SH-396-005 | Pending |
| BR-EM-004 | Deduplicate and sort ConfigMap names across all reference paths | P0 | Unit | UT-SH-396-006 | Pending |
| BR-EM-004 | Empty slice when no ConfigMap references in spec | P0 | Unit | UT-SH-396-007 | Pending |
| BR-EM-004 | Malformed spec returns empty slice without panic | P1 | Unit | UT-SH-396-008 | Pending |
| BR-EM-004 | Empty slice for non-workload kind (HPA, Service) | P0 | Unit | UT-SH-396-009 | Pending |
| BR-EM-004 | Pod kind extracts from spec directly (no .template) | P0 | Unit | UT-SH-396-010 | Pending |
| BR-EM-004 | CronJob kind extracts from spec.jobTemplate.spec.template.spec | P0 | Unit | UT-SH-396-011 | Pending |
| BR-EM-004 | ConfigMapDataHash deterministic for sorted .data key-value pairs | P0 | Unit | UT-SH-396-012 | Pending |
| BR-EM-004 | ConfigMapDataHash includes .binaryData (base64-encoded) | P0 | Unit | UT-SH-396-013 | Pending |
| BR-EM-004 | ConfigMapDataHash key-order independent | P0 | Unit | UT-SH-396-014 | Pending |
| BR-EM-004 | ConfigMapDataHash empty data produces deterministic hash | P0 | Unit | UT-SH-396-015 | Pending |
| BR-EM-004 | ConfigMapDataHash absent sentinel distinct from empty data | P0 | Unit | UT-SH-396-016 | Pending |
| BR-EM-004 | CompositeSpecHash with no ConfigMaps equals CanonicalSpecHash | P0 | Unit | UT-SH-396-017 | Pending |
| BR-EM-004 | CompositeSpecHash with ConfigMap data differs from spec-only | P0 | Unit | UT-SH-396-018 | Pending |
| BR-EM-004 | CompositeSpecHash deterministic | P0 | Unit | UT-SH-396-019 | Pending |
| BR-EM-004 | CompositeSpecHash detects ConfigMap appearance (absent -> present) | P0 | Unit | UT-SH-396-020 | Pending |
| BR-EM-004 | CompositeSpecHash detects ConfigMap data change | P0 | Unit | UT-SH-396-021 | Pending |
| BR-EM-004 | CompositeSpecHash name-sorted compositing is order-independent | P0 | Unit | UT-SH-396-022 | Pending |
| BR-EM-004 | EM Computer.Compute with ConfigMapData produces composite hash | P0 | Unit | UT-EM-396-001 | Pending |
| BR-EM-004 | EM Computer.Compute without ConfigMapData produces spec-only hash | P0 | Unit | UT-EM-396-002 | Pending |
| BR-EM-004 | EM Computer.Compute Match=true for identical composite hashes | P0 | Unit | UT-EM-396-003 | Pending |
| BR-EM-004 | EM Computer.Compute Match=false for different ConfigMap data | P0 | Unit | UT-EM-396-004 | Pending |
| BR-EM-004 | RO includes ConfigMap content in hash for Deployment with volume | P0 | Unit | UT-RO-396-001 | Pending |
| BR-EM-004 | RO uses absent sentinel when ConfigMap not found (404) | P0 | Unit | UT-RO-396-002 | Pending |
| BR-EM-004 | RO produces spec-only hash when no ConfigMap refs | P0 | Unit | UT-RO-396-003 | Pending |
| BR-EM-004 | RO treats 403 Forbidden as absent (sentinel + warning) | P0 | Unit | UT-RO-396-004 | Pending |
| BR-EM-004 | RO CronJob target resolves nested pod template ConfigMap refs | P0 | Unit | UT-RO-396-005 | Pending |
| BR-EM-004 | EM composite hash matches pre-hash when unchanged | P0 | Integration | IT-EM-396-001 | Pending |
| BR-EM-004 | EM detects ConfigMap content change via hash mismatch | P0 | Integration | IT-EM-396-002 | Pending |
| BR-EM-004 | EM drift guard detects ConfigMap change as spec_drift | P0 | Integration | IT-EM-396-003 | Pending |
| BR-EM-004 | EM absent ConfigMap sentinel produces stable hash | P0 | Integration | IT-EM-396-004 | Pending |

### Status Legend

- Pending: Specification complete, implementation not started
- RED: Failing test written (TDD RED phase)
- GREEN: Minimal implementation passes (TDD GREEN phase)
- REFACTORED: Code cleaned up (TDD REFACTOR phase)
- Pass: Implemented and passing

---

## 5. Test Scenarios

### Test ID Naming Convention

Format: `{TIER}-{SERVICE}-{BR_NUMBER}-{SEQUENCE}`

- **TIER**: `UT` (Unit), `IT` (Integration)
- **SERVICE**: `SH` (Shared Hash), `EM` (Effectiveness Monitor), `RO` (Remediation Orchestrator)
- **BR_NUMBER**: 396 (issue number)
- **SEQUENCE**: Zero-padded 3-digit (001, 002, ...)

### Tier 1: Unit Tests -- ConfigMap Reference Extraction (11 tests)

**Testable code scope**: `pkg/shared/hash/configmap.go` -- `ExtractConfigMapRefs` (~40 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-SH-396-001` | ConfigMap names extracted from `volumes[].configMap.name` in Deployment spec | Pending |
| `UT-SH-396-002` | ConfigMap names extracted from `initContainers[].envFrom[].configMapRef.name` | Pending |
| `UT-SH-396-003` | ConfigMap names extracted from `volumes[].projected.sources[].configMap.name` | Pending |
| `UT-SH-396-004` | ConfigMap names extracted from `containers[].envFrom[].configMapRef.name` | Pending |
| `UT-SH-396-005` | ConfigMap names extracted from `containers[].env[].valueFrom.configMapKeyRef.name` | Pending |
| `UT-SH-396-006` | Duplicate ConfigMap names across paths deduplicated and sorted | Pending |
| `UT-SH-396-007` | Empty slice returned when spec has no ConfigMap references | Pending |
| `UT-SH-396-008` | Malformed spec (unexpected types) returns empty slice without panic | Pending |
| `UT-SH-396-009` | Empty slice for non-workload kind (HPA, Service -- no pod template) | Pending |
| `UT-SH-396-010` | Pod kind extracts from `spec` directly (no `.template` nesting) | Pending |
| `UT-SH-396-011` | CronJob kind extracts from `spec.jobTemplate.spec.template.spec` | Pending |

**File**: `test/unit/shared/hash/configmap_test.go` (new file, Ginkgo/Gomega BDD)

**Pattern**: Call `ExtractConfigMapRefs(spec, kind)` with synthetic `map[string]interface{}` fixtures. Assert returned `[]string` contents and ordering. Same approach as existing `canonical_test.go`.

**Fixture for UT-SH-396-001** (Deployment with configMap volume):
```go
spec := map[string]interface{}{
    "template": map[string]interface{}{
        "spec": map[string]interface{}{
            "volumes": []interface{}{
                map[string]interface{}{
                    "name": "app-config",
                    "configMap": map[string]interface{}{
                        "name": "my-app-config",
                    },
                },
            },
            "containers": []interface{}{
                map[string]interface{}{"name": "app", "image": "nginx:1.25"},
            },
        },
    },
}
refs := ExtractConfigMapRefs(spec, "Deployment")
// Expect: ["my-app-config"]
```

**Fixture for UT-SH-396-003** (projected volume):
```go
spec := map[string]interface{}{
    "template": map[string]interface{}{
        "spec": map[string]interface{}{
            "volumes": []interface{}{
                map[string]interface{}{
                    "name": "combined",
                    "projected": map[string]interface{}{
                        "sources": []interface{}{
                            map[string]interface{}{
                                "configMap": map[string]interface{}{
                                    "name": "app-config",
                                },
                            },
                            map[string]interface{}{
                                "configMap": map[string]interface{}{
                                    "name": "logging-config",
                                },
                            },
                        },
                    },
                },
            },
            "containers": []interface{}{
                map[string]interface{}{"name": "app", "image": "nginx:1.25"},
            },
        },
    },
}
refs := ExtractConfigMapRefs(spec, "Deployment")
// Expect: ["app-config", "logging-config"]
```

**Fixture for UT-SH-396-011** (CronJob):
```go
spec := map[string]interface{}{
    "jobTemplate": map[string]interface{}{
        "spec": map[string]interface{}{
            "template": map[string]interface{}{
                "spec": map[string]interface{}{
                    "volumes": []interface{}{
                        map[string]interface{}{
                            "name": "job-config",
                            "configMap": map[string]interface{}{
                                "name": "cron-config",
                            },
                        },
                    },
                    "containers": []interface{}{
                        map[string]interface{}{"name": "worker", "image": "worker:1.0"},
                    },
                },
            },
        },
    },
}
refs := ExtractConfigMapRefs(spec, "CronJob")
// Expect: ["cron-config"]
```

### Tier 1: Unit Tests -- ConfigMap Data Hashing (5 tests)

**Testable code scope**: `pkg/shared/hash/configmap.go` -- `ConfigMapDataHash` (~20 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-SH-396-012` | Deterministic sha256 for sorted `.data` key-value pairs | Pending |
| `UT-SH-396-013` | `.binaryData` included in hash (base64-encoded for determinism) | Pending |
| `UT-SH-396-014` | Key-order independent (same data, different map iteration -> same hash) | Pending |
| `UT-SH-396-015` | Empty data map produces deterministic hash | Pending |
| `UT-SH-396-016` | Absent sentinel produces deterministic hash distinct from empty data | Pending |

**File**: `test/unit/shared/hash/configmap_test.go` (same file, separate `Describe` block)

**Pattern**: Call `ConfigMapDataHash(data, binaryData)` with synthetic maps. Assert `sha256:` prefix, 71-char length, determinism, and distinctness.

**Fixture for UT-SH-396-013** (binaryData):
```go
data := map[string]string{"config.yaml": "key: value"}
binaryData := map[string][]byte{"cert.pem": []byte("binary-cert-content")}
h1, err := ConfigMapDataHash(data, binaryData)
// Expect: sha256: prefixed, 71 chars
// Expect: differs from ConfigMapDataHash(data, nil)
```

**Fixture for UT-SH-396-016** (absent sentinel vs empty):
```go
absentData := map[string]string{"__sentinel__": "__absent:my-config__"}
emptyData := map[string]string{}
hAbsent, _ := ConfigMapDataHash(absentData, nil)
hEmpty, _ := ConfigMapDataHash(emptyData, nil)
// Expect: hAbsent != hEmpty
```

### Tier 1: Unit Tests -- Composite Hash Computation (6 tests)

**Testable code scope**: `pkg/shared/hash/configmap.go` -- `CompositeSpecHash` (~25 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-SH-396-017` | No ConfigMaps -> equals `CanonicalSpecHash` (backward compat) | Pending |
| `UT-SH-396-018` | With ConfigMap data -> differs from spec-only hash | Pending |
| `UT-SH-396-019` | Deterministic (same spec + same ConfigMap data -> same composite) | Pending |
| `UT-SH-396-020` | ConfigMap appears (absent sentinel -> real data) -> different hash | Pending |
| `UT-SH-396-021` | ConfigMap data changes -> different hash | Pending |
| `UT-SH-396-022` | Multiple ConfigMaps -- name-sorted compositing is order-independent | Pending |

**File**: `test/unit/shared/hash/configmap_test.go` (same file, separate `Describe` block)

**Pattern**: Call `CompositeSpecHash(specHash, configMapHashes)`. Assert format, determinism, and semantic distinctness.

**Fixture for UT-SH-396-017** (backward compat):
```go
specHash := "sha256:abcdef..." // pre-computed from CanonicalSpecHash
composite, err := CompositeSpecHash(specHash, nil)
// Expect: composite == specHash (no ConfigMaps means identity)
```

**Fixture for UT-SH-396-022** (order independence):
```go
hashes := map[string]string{
    "config-a": "sha256:aaa...",
    "config-b": "sha256:bbb...",
}
h1, _ := CompositeSpecHash(specHash, hashes)

reversedHashes := map[string]string{
    "config-b": "sha256:bbb...",
    "config-a": "sha256:aaa...",
}
h2, _ := CompositeSpecHash(specHash, reversedHashes)
// Expect: h1 == h2 (name-sorted compositing)
```

### Tier 1: Unit Tests -- EM Hash Computer Extension (4 tests)

**Testable code scope**: `pkg/effectivenessmonitor/hash/hash.go` -- `SpecHashInput.ConfigMapData`, `computer.Compute` composite path (~15 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-EM-396-001` | Compute with ConfigMapData produces composite hash | Pending |
| `UT-EM-396-002` | Compute without ConfigMapData produces spec-only hash (backward compat) | Pending |
| `UT-EM-396-003` | Match=true when pre and post composite hashes are identical | Pending |
| `UT-EM-396-004` | Match=false when ConfigMap data changed between pre and post | Pending |

**File**: `test/unit/effectivenessmonitor/hash_test.go` (extend existing, new `Describe` block)

**Pattern**: Follow existing `UT-EM-SH-*` structure. Construct `SpecHashInput` with/without `ConfigMapData`, call `computer.Compute`, assert `ComputeResult` fields.

**Fixture for UT-EM-396-001**:
```go
input := hash.SpecHashInput{
    Spec: map[string]interface{}{"replicas": float64(3)},
    ConfigMapData: map[string]map[string]string{
        "my-config": {"config.yaml": "key: value"},
    },
}
result := computer.Compute(input)
// Expect: result.Hash starts with "sha256:", len 71
// Expect: result.Hash != specOnlyHash (computed without ConfigMapData)
```

**Fixture for UT-EM-396-002** (backward compat):
```go
spec := map[string]interface{}{"replicas": float64(3)}
withoutCM := computer.Compute(hash.SpecHashInput{Spec: spec})
withNilCM := computer.Compute(hash.SpecHashInput{Spec: spec, ConfigMapData: nil})
withEmptyCM := computer.Compute(hash.SpecHashInput{Spec: spec, ConfigMapData: map[string]map[string]string{}})
// Expect: withoutCM.Hash == withNilCM.Hash == withEmptyCM.Hash
```

### Tier 1: Unit Tests -- RO Hash Capture with ConfigMaps (4 tests)

**Testable code scope**: `internal/controller/remediationorchestrator/reconciler.go` -- `CapturePreRemediationHash` with ConfigMap resolution (~30 lines, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `UT-RO-396-001` | Includes ConfigMap content in hash for Deployment with configMap volume | Pending |
| `UT-RO-396-002` | Uses absent sentinel when referenced ConfigMap not found (404) | Pending |
| `UT-RO-396-003` | Produces spec-only hash when target has no ConfigMap refs | Pending |
| `UT-RO-396-004` | Treats 403 Forbidden on ConfigMap fetch as absent (sentinel + warning) | Pending |
| `UT-RO-396-005` | CronJob target with nested pod template resolves ConfigMap refs | Pending |

**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go` (extend existing, new `Describe` block)

**Pattern**: Follow existing `CapturePreRemediationHash` test structure. Use `fake.NewClientBuilder().WithScheme(scheme).WithObjects(...)` with Deployment + ConfigMap objects. For 403 test, use client interceptor to return Forbidden error on ConfigMap `Get`.

**Fixture for UT-RO-396-001** (Deployment + ConfigMap):
```go
deploy := &appsv1.Deployment{
    // ... standard metadata ...
    Spec: appsv1.DeploymentSpec{
        // ...
        Template: corev1.PodTemplateSpec{
            Spec: corev1.PodSpec{
                Volumes: []corev1.Volume{{
                    Name: "app-config",
                    VolumeSource: corev1.VolumeSource{
                        ConfigMap: &corev1.ConfigMapVolumeSource{
                            LocalObjectReference: corev1.LocalObjectReference{
                                Name: "my-app-config",
                            },
                        },
                    },
                }},
                Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}},
            },
        },
    },
}
cm := &corev1.ConfigMap{
    ObjectMeta: metav1.ObjectMeta{Name: "my-app-config", Namespace: "default"},
    Data: map[string]string{"config.yaml": "key: value"},
}
fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(deploy, cm).Build()
hash, err := controller.CapturePreRemediationHash(ctx, fakeClient, restMapper, "Deployment", "test-app", "default")
// Expect: hash starts with "sha256:", len 71
// Expect: hash != specOnlyHash (hash without ConfigMap data)
```

**Fixture for UT-RO-396-002** (absent ConfigMap):
```go
// Deployment references "missing-config" ConfigMap, but ConfigMap not in fake client
hash, err := controller.CapturePreRemediationHash(ctx, fakeClient, restMapper, "Deployment", "test-app", "default")
// Expect: no error (graceful degradation)
// Expect: hash is valid sha256, includes absent sentinel in computation
// Expect: hash != specOnlyHash (sentinel changes the composite)
```

### Tier 2: Integration Tests -- EM Hash with ConfigMaps (4 tests)

**Testable code scope**: `internal/controller/effectivenessmonitor/reconciler.go` -- `assessHash`, `getTargetSpec`, drift guard (~30 lines wiring, target >=80%)

| ID | Business Outcome Under Test | Phase |
|----|----------------------------|-------|
| `IT-EM-396-001` | Composite hash matches pre-hash when spec + ConfigMaps unchanged | Pending |
| `IT-EM-396-002` | ConfigMap content change detected via hash mismatch (Match=false) | Pending |
| `IT-EM-396-003` | Drift guard detects ConfigMap content change as spec_drift | Pending |
| `IT-EM-396-004` | Absent ConfigMap sentinel produces stable hash (optional ConfigMap) | Pending |

**File**: `test/integration/effectivenessmonitor/hash_configmap_integration_test.go` (new file)

**Pattern**: Follow existing `hash_integration_test.go` structure. Uses envtest (`k8sClient`, `k8sManager`). Creates real Deployments, ConfigMaps, and EAs.

### Tier Skip Rationale

- **E2E**: Hash computation is infrastructure-level logic with no new CRDs or API endpoints. UT (fake client) + IT (envtest) provide defense-in-depth for both pure logic and reconciler wiring. RBAC Helm chart changes are declarative YAML, verified during deployment.

---

## 6. Test Cases (Detail)

### UT-SH-396-001: Extract ConfigMap names from volumes[].configMap.name

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A Deployment spec with one volume referencing configMap named "my-app-config"
**When**: `ExtractConfigMapRefs(spec, "Deployment")` is called
**Then**: Returns `["my-app-config"]`

**Acceptance Criteria**:
- Returned slice contains exactly one element
- Element matches the ConfigMap name from the volume definition
- Order is deterministic (sorted)

---

### UT-SH-396-002: Extract ConfigMap names from initContainers envFrom

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A Deployment spec with one initContainer using `envFrom[].configMapRef.name: "init-config"`
**When**: `ExtractConfigMapRefs(spec, "Deployment")` is called
**Then**: Returns `["init-config"]`

**Acceptance Criteria**:
- initContainers path is traversed alongside containers
- Returned name matches the configMapRef name

---

### UT-SH-396-003: Extract ConfigMap names from projected volume sources

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A Deployment spec with a projected volume containing two configMap sources ("app-config", "logging-config")
**When**: `ExtractConfigMapRefs(spec, "Deployment")` is called
**Then**: Returns `["app-config", "logging-config"]` (sorted)

**Acceptance Criteria**:
- Both ConfigMap names from projected sources are extracted
- Secret sources in the same projected volume are ignored
- Results are sorted alphabetically

---

### UT-SH-396-004: Extract ConfigMap names from containers envFrom

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A Deployment spec with one container using `envFrom[].configMapRef.name: "env-config"`
**When**: `ExtractConfigMapRefs(spec, "Deployment")` is called
**Then**: Returns `["env-config"]`

**Acceptance Criteria**:
- envFrom configMapRef path is correctly traversed
- Returned name matches the configMapRef name

---

### UT-SH-396-005: Extract ConfigMap names from containers env valueFrom

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A Deployment spec with one container env var using `valueFrom.configMapKeyRef.name: "feature-flags"`
**When**: `ExtractConfigMapRefs(spec, "Deployment")` is called
**Then**: Returns `["feature-flags"]`

**Acceptance Criteria**:
- env[].valueFrom.configMapKeyRef path is correctly traversed
- Returned name matches the configMapKeyRef name

---

### UT-SH-396-006: Deduplicate and sort across all paths

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A Deployment spec with "shared-config" referenced in both a volume AND containers[].envFrom
**When**: `ExtractConfigMapRefs(spec, "Deployment")` is called
**Then**: Returns `["shared-config"]` (deduplicated, single entry)

**Acceptance Criteria**:
- Duplicate ConfigMap names across different reference paths are collapsed
- Result is sorted alphabetically
- No duplicate entries in the returned slice

---

### UT-SH-396-007: Empty slice when no ConfigMap references

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A Deployment spec with containers but no volumes, no envFrom, no env valueFrom referencing ConfigMaps
**When**: `ExtractConfigMapRefs(spec, "Deployment")` is called
**Then**: Returns `[]` (empty slice)

**Acceptance Criteria**:
- Empty slice (not nil) is returned
- No error or panic

---

### UT-SH-396-008: Malformed spec returns empty slice without panic

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A spec where `volumes` is a string instead of `[]interface{}`, and `containers` is missing
**When**: `ExtractConfigMapRefs(spec, "Deployment")` is called
**Then**: Returns `[]` (empty slice, no panic)

**Acceptance Criteria**:
- No panic on type assertion failure
- Defensive two-return-value type assertions handle unexpected types
- Returns empty slice gracefully

---

### UT-SH-396-009: Empty slice for non-workload kind

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: An HPA spec (no pod template)
**When**: `ExtractConfigMapRefs(spec, "HorizontalPodAutoscaler")` is called
**Then**: Returns `[]` (empty slice)

**Acceptance Criteria**:
- Non-workload kinds that have no pod template return empty
- No error or panic

---

### UT-SH-396-010: Pod kind extracts from spec directly

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A Pod spec (not wrapped in `.template.spec`) with a configMap volume "pod-config"
**When**: `ExtractConfigMapRefs(spec, "Pod")` is called
**Then**: Returns `["pod-config"]`

**Acceptance Criteria**:
- Pod kind reads volumes/containers from spec directly (not spec.template.spec)
- ConfigMap name extracted correctly

---

### UT-SH-396-011: CronJob kind extracts from nested jobTemplate

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A CronJob spec with pod template at `spec.jobTemplate.spec.template.spec` containing a configMap volume "cron-config"
**When**: `ExtractConfigMapRefs(spec, "CronJob")` is called
**Then**: Returns `["cron-config"]`

**Acceptance Criteria**:
- CronJob path `jobTemplate.spec.template.spec` is correctly traversed
- ConfigMap name extracted from the deeply nested pod spec

---

### UT-SH-396-012: ConfigMapDataHash deterministic for .data

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: ConfigMap `.data` with keys "config.yaml" and "settings.json"
**When**: `ConfigMapDataHash(data, nil)` is called twice
**Then**: Both calls produce identical `sha256:` prefixed hash

**Acceptance Criteria**:
- Hash has `sha256:` prefix, 71 characters total
- Two calls with same input produce same output

---

### UT-SH-396-013: BinaryData included in hash

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: ConfigMap with both `.data` {"key": "value"} and `.binaryData` {"cert.pem": <bytes>}
**When**: `ConfigMapDataHash(data, binaryData)` is called
**Then**: Hash differs from `ConfigMapDataHash(data, nil)` (binaryData contributes to hash)

**Acceptance Criteria**:
- Hash includes binaryData contribution
- Base64 encoding of binary values is deterministic
- Hash differs when binaryData is present vs absent

---

### UT-SH-396-014: ConfigMapDataHash key-order independent

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: Two maps with same key-value pairs in different insertion order
**When**: `ConfigMapDataHash` is called on each
**Then**: Both produce the same hash

**Acceptance Criteria**:
- Go map iteration order does not affect hash
- Keys are sorted before serialization

---

### UT-SH-396-015: ConfigMapDataHash empty data

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: Empty `.data` map and nil `.binaryData`
**When**: `ConfigMapDataHash(map[string]string{}, nil)` is called
**Then**: Produces valid `sha256:` hash

**Acceptance Criteria**:
- Hash has correct format
- Deterministic across multiple calls

---

### UT-SH-396-016: Absent sentinel distinct from empty data

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: Absent sentinel data `{"__sentinel__": "__absent:my-config__"}` and empty data `{}`
**When**: `ConfigMapDataHash` is called on each
**Then**: Hashes are different

**Acceptance Criteria**:
- Sentinel value produces a distinct hash from empty ConfigMap
- Enables detection of ConfigMap appearance/disappearance transitions

---

### UT-SH-396-017: CompositeSpecHash with no ConfigMaps equals CanonicalSpecHash

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A spec hash and nil/empty configMapHashes
**When**: `CompositeSpecHash(specHash, nil)` is called
**Then**: Returns the original specHash unchanged

**Acceptance Criteria**:
- Identity behavior: no ConfigMaps means the composite hash equals the spec-only hash
- Backward compatible with existing hash consumers

---

### UT-SH-396-018: CompositeSpecHash with ConfigMap data differs from spec-only

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: A spec hash and one ConfigMap hash {"my-config": "sha256:abc..."}
**When**: `CompositeSpecHash(specHash, configMapHashes)` is called
**Then**: Returns a hash different from specHash

**Acceptance Criteria**:
- ConfigMap data changes the composite hash
- Result is a valid `sha256:` prefixed hash, 71 chars

---

### UT-SH-396-019: CompositeSpecHash deterministic

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: Same spec hash and same configMapHashes
**When**: `CompositeSpecHash` is called twice
**Then**: Both calls produce the same hash

**Acceptance Criteria**:
- Deterministic output for identical inputs

---

### UT-SH-396-020: CompositeSpecHash detects ConfigMap appearance

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: Pre-hash with ConfigMap sentinel (absent), post-hash with real ConfigMap data
**When**: Both are compared
**Then**: Hashes differ

**Acceptance Criteria**:
- Transition from absent to present ConfigMap is detectable via hash change

---

### UT-SH-396-021: CompositeSpecHash detects ConfigMap data change

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: Same spec hash, but ConfigMap hash changes (data modified)
**When**: `CompositeSpecHash` is called with each
**Then**: Composite hashes differ

**Acceptance Criteria**:
- ConfigMap content change produces different composite hash
- Spec hash alone cannot mask the change

---

### UT-SH-396-022: CompositeSpecHash name-sorted order independence

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/shared/hash/configmap_test.go`

**Given**: Two configMapHashes maps with same entries but different insertion order: {"a": h1, "b": h2} and {"b": h2, "a": h1}
**When**: `CompositeSpecHash` is called on each
**Then**: Both produce the same hash

**Acceptance Criteria**:
- ConfigMap names are sorted before compositing
- Go map iteration order does not affect result

---

### UT-EM-396-001: Compute with ConfigMapData produces composite hash

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/hash_test.go`

**Given**: `SpecHashInput` with Spec and ConfigMapData populated
**When**: `computer.Compute(input)` is called
**Then**: `ComputeResult.Hash` is a composite hash (differs from spec-only)

**Acceptance Criteria**:
- Hash has `sha256:` prefix, 71 chars
- Hash differs from Compute without ConfigMapData
- `Component.Assessed` is true

---

### UT-EM-396-002: Compute without ConfigMapData (backward compat)

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/hash_test.go`

**Given**: `SpecHashInput` with Spec only, ConfigMapData is nil
**When**: `computer.Compute(input)` is called
**Then**: `ComputeResult.Hash` equals the spec-only hash (same as pre-#396 behavior)

**Acceptance Criteria**:
- ConfigMapData=nil produces same hash as ConfigMapData=empty map
- Both equal the pre-existing spec-only hash
- Existing tests are not broken

---

### UT-EM-396-003: Match=true for identical composite hashes

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/hash_test.go`

**Given**: Pre-hash computed with Spec+ConfigMapData, post-hash with same Spec+ConfigMapData
**When**: `computer.Compute` with PreHash set to the pre-computed composite hash
**Then**: `Match` is `true`

**Acceptance Criteria**:
- `*result.Match == true`
- PreHash is preserved in the result
- Composite hash comparison works correctly

---

### UT-EM-396-004: Match=false for different ConfigMap data

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/effectivenessmonitor/hash_test.go`

**Given**: Pre-hash computed with ConfigMapData v1, post-hash with ConfigMapData v2 (content changed)
**When**: `computer.Compute` with PreHash set to the v1 composite hash
**Then**: `Match` is `false`

**Acceptance Criteria**:
- `*result.Match == false`
- ConfigMap content change is detectable through the Computer interface

---

### UT-RO-396-001: RO includes ConfigMap content in hash

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

**Given**: A Deployment with a configMap volume "my-app-config" and a ConfigMap "my-app-config" with data {"config.yaml": "key: value"} in the fake client
**When**: `CapturePreRemediationHash(ctx, fakeClient, restMapper, "Deployment", "test-app", "default")` is called
**Then**: Returns a hash that differs from the spec-only hash

**Acceptance Criteria**:
- Hash is valid `sha256:` format, 71 chars
- Hash differs from calling `CapturePreRemediationHash` on the same Deployment without ConfigMap data in the volume
- ConfigMap content is included in the hash computation

---

### UT-RO-396-002: RO uses absent sentinel for missing ConfigMap

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

**Given**: A Deployment referencing ConfigMap "missing-config" in its volume, but ConfigMap does not exist in the fake client
**When**: `CapturePreRemediationHash` is called
**Then**: Returns a valid hash (no error), hash includes absent sentinel

**Acceptance Criteria**:
- No error returned (graceful degradation)
- Hash is valid `sha256:` format
- Hash differs from spec-only hash (sentinel is part of composite)
- Hash differs from the hash with the real ConfigMap data present

---

### UT-RO-396-003: RO produces spec-only hash with no ConfigMap refs

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

**Given**: A Deployment with no volumes, no envFrom, no env valueFrom referencing ConfigMaps
**When**: `CapturePreRemediationHash` is called
**Then**: Returns the same hash as pre-#396 behavior (spec-only)

**Acceptance Criteria**:
- Hash matches the existing canonical spec hash (backward compat)
- No ConfigMap API calls made (nothing to resolve)

---

### UT-RO-396-004: RO treats 403 Forbidden as absent

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

**Given**: A Deployment referencing ConfigMap "rbac-denied-config", fake client interceptor returns Forbidden on ConfigMap Get
**When**: `CapturePreRemediationHash` is called
**Then**: Returns a valid hash (no error), uses absent sentinel, logs warning

**Acceptance Criteria**:
- No error returned (graceful degradation)
- Hash includes absent sentinel (same as 404 case)
- RBAC failure does not block hash computation

---

### UT-RO-396-005: RO CronJob target resolves ConfigMap refs

**BR**: BR-EM-004
**Type**: Unit
**File**: `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

**Given**: A CronJob with a configMap volume at `spec.jobTemplate.spec.template.spec.volumes[]` and the ConfigMap exists in the fake client
**When**: `CapturePreRemediationHash(ctx, fakeClient, restMapper, "CronJob", "my-cronjob", "default")` is called
**Then**: Returns a composite hash including ConfigMap content

**Acceptance Criteria**:
- CronJob's nested pod template path is correctly traversed
- Hash includes ConfigMap content (differs from spec-only hash)

---

### IT-EM-396-001: Composite hash matches pre-hash when unchanged

**BR**: BR-EM-004
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/hash_configmap_integration_test.go`

**Given**: A Deployment with a configMap volume, ConfigMap exists in the cluster, EA created with PreRemediationSpecHash set to the composite hash of the unchanged state
**When**: EA reconciles to Completed
**Then**: `PostRemediationSpecHash` matches `PreRemediationSpecHash` (Match=true)

**Acceptance Criteria**:
- EA completes with `HashComputed=true`
- `PostRemediationSpecHash` equals the pre-hash (no drift)
- `CurrentSpecHash` equals `PostRemediationSpecHash`
- ConfigMap content was included in both pre and post hash computation

---

### IT-EM-396-002: ConfigMap content change detected

**BR**: BR-EM-004
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/hash_configmap_integration_test.go`

**Given**: A Deployment with configMap volume, EA created with PreRemediationSpecHash from spec+ConfigMap-v1, then ConfigMap updated to v2 before EM computes post-hash
**When**: EA reconciles to Completed
**Then**: `PostRemediationSpecHash` differs from `PreRemediationSpecHash` (Match=false)

**Acceptance Criteria**:
- EA completes with `HashComputed=true`
- Pre and post hashes differ (ConfigMap content change detected)
- Deployment spec itself did not change -- only ConfigMap data

---

### IT-EM-396-003: Drift guard detects ConfigMap change as spec_drift

**BR**: BR-EM-004
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/hash_configmap_integration_test.go`

**Given**: EA completed initial assessment with PostRemediationSpecHash, then ConfigMap content is mutated while EA is in Assessing phase
**When**: Drift guard (Step 6.5) re-hashes on next reconcile
**Then**: EA completes with `AssessmentReason=spec_drift`

**Acceptance Criteria**:
- Drift guard uses the same composite hash utility as the initial hash
- ConfigMap mutation triggers `spec_drift` even though Deployment spec is unchanged
- `SpecIntegrity` condition set to `False` with reason `SpecDrifted`

---

### IT-EM-396-004: Absent ConfigMap sentinel hash stability

**BR**: BR-EM-004
**Type**: Integration
**File**: `test/integration/effectivenessmonitor/hash_configmap_integration_test.go`

**Given**: A Deployment references a ConfigMap that does not exist (optional ConfigMap), EA created with PreRemediationSpecHash computed with absent sentinel
**When**: EA reconciles and ConfigMap still does not exist
**Then**: `PostRemediationSpecHash` matches `PreRemediationSpecHash` (stable)

**Acceptance Criteria**:
- Absent ConfigMap produces deterministic sentinel in both pre and post hash
- Hashes match (no false-positive drift)
- If ConfigMap later appears, subsequent assessment would detect the change (covered by UT-SH-396-020)

---

## 7. Test Infrastructure

### Unit Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: External dependencies only (K8s API via `fake.NewClientBuilder()` for RO tests, client interceptor for 403 simulation)
- **Location**: `test/unit/shared/hash/configmap_test.go`, `test/unit/effectivenessmonitor/hash_test.go`, `test/unit/remediationorchestrator/pre_remediation_hash_test.go`

### Integration Tests

- **Framework**: Ginkgo/Gomega BDD (mandatory)
- **Mocks**: ZERO mocks (see [No-Mocks Policy](../../testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md))
- **Infrastructure**: envtest (`k8sClient`, `k8sManager`) with real K8s API server, real ConfigMap CRUD, real EA reconciliation
- **Location**: `test/integration/effectivenessmonitor/hash_configmap_integration_test.go`

---

## 8. Risk Mitigations

| Risk | Mitigation | Test Coverage |
|------|-----------|---------------|
| R1: Missing ConfigMap reference paths | All 5 paths covered: `volumes[].configMap`, `volumes[].projected.sources[].configMap`, `containers[].envFrom[].configMapRef`, `containers[].env[].valueFrom.configMapKeyRef`, + same for `initContainers[]` | UT-SH-396-001 through 005 |
| R2: Kind-to-PodSpec path mapping errors | Three path shapes tested: Deployment (`.template.spec`), Pod (direct `spec`), CronJob (`.jobTemplate.spec.template.spec`) | UT-SH-396-010, 011, UT-RO-396-005 |
| R3: EM drift guard inconsistency | IT-EM-396-003 explicitly tests drift guard with ConfigMap changes. Implementation must refactor drift guard to reuse composite hash utility. | IT-EM-396-003 |
| R4: Unstructured type assertion panics | Malformed spec test validates defensive type assertions return empty slice | UT-SH-396-008 |
| R5: Race between spec and ConfigMap fetch | Accepted for v1.2. Document in DD-EM-002 v1.3 non-guarantees. | N/A (documentation) |
| R6: Existing test breakage | `CanonicalSpecHash` unchanged. `ConfigMapData` field is optional with nil/empty fallback. | UT-EM-396-002 |
| R7: One-time drift storm on upgrade | Accepted. No backward compat needed. First assessment post-upgrade shows hash mismatch. | N/A (by design) |

---

## 9. Execution

```bash
# Unit tests -- shared hash ConfigMap utilities
go test ./test/unit/shared/hash/... -ginkgo.focus="ConfigMap"

# Unit tests -- EM hash computer with ConfigMap
go test ./test/unit/effectivenessmonitor/... -ginkgo.focus="UT-EM-396"

# Unit tests -- RO pre-remediation hash with ConfigMaps
go test ./test/unit/remediationorchestrator/... -ginkgo.focus="UT-RO-396"

# Integration tests -- EM hash with ConfigMaps
go test ./test/integration/effectivenessmonitor/... -ginkgo.focus="IT-EM-396"

# All unit tests
make test

# All integration tests for EM
make test-integration-effectivenessmonitor
```

---

## 10. Changelog

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | 2026-03-04 | Initial test plan: 34 tests (30 UT + 4 IT), 7 risk mitigations, ConfigMap reference extraction (5 paths + projected), data hashing with binaryData, composite hash, EM computer extension, RO hash capture, EM integration with drift guard |
