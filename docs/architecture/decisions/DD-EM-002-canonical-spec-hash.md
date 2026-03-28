# DD-EM-002: Canonical Spec Hash for Pre/Post Remediation Comparison

**Status**: PROPOSED
**Date**: 2026-02-13
**Author**: Architecture Team

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-02-13 | Architecture Team | Initial DD: canonical JSON + SHA-256 algorithm specification, guarantees, non-guarantees, testing requirements |
| 1.1 | 2026-02-14 | Architecture Team | Added Spec Drift Guard: re-hash on each reconcile, spec_drift reason, DS score=0.0 short-circuit |
| 1.2 | 2026-02-24 | Architecture Team | **Issue #188 (DD-EM-003)**: Updated RO consumer description to reference `resolveDualTargets` (renamed from `resolveEffectivenessTarget`). Hash is now explicitly computed from the `RemediationTarget` (the AI-resolved resource), not the single `TargetResource`. |
| 1.3 | 2026-03-04 | Architecture Team | **Issue #396**: ConfigMap-aware composite hashing. Spec hash now incorporates referenced ConfigMap `.data` and `.binaryData` content. Added `ExtractConfigMapRefs`, `ConfigMapDataHash`, `CompositeSpecHash` utilities. Sentinel value for absent/forbidden ConfigMaps. RBAC update for configmaps. |

---

## Context and Problem Statement

The Effectiveness Monitor (EM) compares the target resource's `.spec` before and after remediation to detect whether the workflow's changes are still in effect (configuration drift detection, BR-EM-004). This requires two hashes:

1. **Pre-remediation hash**: Captured by the Remediation Orchestrator (RO) before creating the WorkflowExecution CRD. Emitted in the `remediation.workflow_created` audit event and stored in DataStorage. The hash is captured for the **AI-resolved target resource** (AffectedResource when available from AIAnalysis.Status.RootCauseAnalysis, else RR.Spec.TargetResource), not RR.Spec.TargetResource directly.

2. **Post-remediation hash**: Captured by the EM after the stabilization window passes. Compared against the pre-hash to produce the `effectiveness.hash.computed` audit event.

For the comparison to be meaningful, both hashes **must** be computed using the exact same algorithm, and the algorithm **must** be deterministic regardless of:

- Go's non-deterministic map iteration order
- Slice element reordering (e.g., from strategic merge patches, webhook mutations, or API server serialization differences across versions)
- Cross-process execution (RO and EM are separate binaries with separate memory spaces)

### Why Not Standard `json.Marshal` + SHA-256?

Go's `encoding/json.Marshal` sorts map keys alphabetically (deterministic for maps), but **preserves slice order** exactly as-is. This means:

- If the Kubernetes API server returns `containers: [{name: "b"}, {name: "a"}]` in one call and `containers: [{name: "a"}, {name: "b"}]` in another (possible across API server restarts, strategic merge patches, or webhook mutations), the hashes would differ even though the logical content is identical.
- JSON number representation is stable for the same `float64` value, but the unmarshal-marshal round-trip is itself idempotent.

### Why Not `mitchellh/hashstructure`?

The [mitchellh/hashstructure](https://github.com/mitchellh/hashstructure) library (archived July 2024) provides struct-level hashing with set semantics for slices. However:

- It produces `uint64` (FNV), not SHA-256 (cryptographic-strength, human-readable)
- It operates on Go structs, not `map[string]interface{}` (which is what unstructured K8s resources provide)
- The repository is archived and no longer maintained

### Why Not Tailscale's `deephash`?

[Tailscale's deephash](https://pkg.go.dev/tailscale.com/util/deephash) is actively maintained but explicitly states: "hashes are only valid within a program's lifetime and shouldn't be stored or transmitted." Since we need to store the pre-hash in DataStorage and compare it from a different process (EM) hours or days later, `deephash` is unsuitable.

---

## Decision

Implement a **canonical JSON normalization + SHA-256** utility in `pkg/shared/hash/` that both the RO and EM use. The algorithm recursively normalizes the input before serialization to guarantee order-independent hashing.

### Algorithm Specification

```
CanonicalSpecHash(spec map[string]interface{}) -> (sha256_hex_string, error)

1. Normalize the input recursively:
   a. map[string]interface{}: sort keys alphabetically, recurse into each value
   b. []interface{}: sort elements by their canonical JSON representation, recurse into each element
   c. All other types (string, float64, bool, nil): pass through unchanged

2. Serialize the normalized structure using encoding/json.Marshal
   (which itself sorts map keys, but we pre-sort for clarity and slice normalization)

3. Compute SHA-256 of the serialized bytes

4. Return the lowercase hex-encoded hash string (64 characters)
```

### Slice Sorting Strategy

Slices are sorted by comparing the canonical JSON representation of each element:

```go
sort.Slice(normalized, func(i, j int) bool {
    ji, _ := json.Marshal(normalized[i])
    jj, _ := json.Marshal(normalized[j])
    return string(ji) < string(jj)
})
```

This ensures:
- Simple values (strings, numbers) sort naturally by their JSON representation
- Objects sort by their full canonical JSON (deterministic because maps are sorted first)
- Mixed-type slices sort by JSON type representation (booleans < numbers < strings < objects, etc.)

### Package Location

```
pkg/shared/hash/
    canonical.go       -- CanonicalSpecHash, normalizeValue (exported + unexported)
    configmap.go       -- ExtractConfigMapRefs, ConfigMapDataHash, CompositeSpecHash (v1.3, #396)
    canonical_test.go  -- (test/unit/shared/hash/canonical_test.go per project convention)
    configmap_test.go  -- (test/unit/shared/hash/configmap_test.go per project convention)
```

Both the RO and EM import `pkg/shared/hash` to compute their respective hashes.

---

## Guarantees

| Guarantee | Description |
|-----------|-------------|
| **Idempotent** | The same logical content always produces the same hash, regardless of how many times it is computed |
| **Map-order independent** | Go's non-deterministic map iteration order does not affect the hash |
| **Slice-order independent** | Element reordering within slices (e.g., containers, volumes, env vars) does not affect the hash |
| **Cross-process portable** | RO and EM instances (separate binaries, separate machines) produce identical hashes for the same resource state |
| **SHA-256 strength** | 256-bit cryptographic hash — collision-resistant for any practical number of resources |
| **Human-readable** | Hex-encoded string (64 chars) — can be logged, stored in audit events, displayed in `kubectl get` output |

---

## Non-Guarantees

| Non-Guarantee | Description | Mitigation |
|---------------|-------------|------------|
| **Server-side defaulting** | If the API server adds new defaulted fields between the pre and post GET (e.g., Kubernetes version upgrade), the hash will change even if no user-visible change occurred | Accept as a known limitation; document in operator runbook. The hash change triggers a "drift detected" assessment, which is conservative but safe. |
| **Field removal** | If a field is removed from the spec (e.g., deprecated field pruning), the hash changes | Same as above — conservative detection |
| **Intentional reordering** | Slice-order independence means the hash cannot detect intentional reordering (e.g., container priority changes via order) | Container priority should be expressed as explicit fields, not positional ordering. This is consistent with Kubernetes best practices. |
| **Floating-point edge cases** | JSON numbers parsed into `float64` may lose precision for integers > 2^53 | Kubernetes specs do not use integers > 2^53 in practice. No mitigation needed. |
| **`resourceVersion` changes** | The resource's `metadata.resourceVersion` changes on every update, but we hash only `.spec`, not metadata | By design — we only hash the `.spec` field |

---

## Consumers

| Consumer | Usage | Phase |
|----------|-------|-------|
| **Remediation Orchestrator** | Computes `CanonicalSpecHash(spec)`, resolves ConfigMap content hashes via `resolveConfigMapHashes`, then produces composite hash via `CompositeSpecHash`. Targets the AI-resolved resource (`resolveDualTargets(rr, ai).Remediation` — DD-EM-003). Hash stored on `RR.Status.PreRemediationSpecHash` and emitted in `remediation.workflow_created` audit event. | RO Analyzing phase |
| **Effectiveness Monitor** | Same composite hash pipeline after stabilization window. Compared against pre-hash from EA spec. Result emitted in `effectiveness.hash.computed` audit event. Also used in drift guard (Step 6.5) for ongoing spec integrity checks. | EM assessment Step 3-4, Step 6.5 |
| **DataStorage** | Stores both hashes in audit events. Returns pre-hash to EM via `queryAuditEvents` API. May use `hash_match` boolean in effectiveness score computation. | Audit storage + query |

---

## Testing Requirements

The canonical hash utility must be thoroughly tested to prevent production edge cases. All tests use the Ginkgo/Gomega BDD framework per project convention.

| Test ID | Scenario | Validates |
|---------|----------|-----------|
| UT-HASH-001 | Map key order independence | Same map content with different key iteration order produces identical hash |
| UT-HASH-002 | Slice order independence | `[a, b]` and `[b, a]` produce the same hash |
| UT-HASH-003 | Nested map + slice normalization | Deeply nested structures normalize correctly |
| UT-HASH-004 | Real K8s Deployment spec | Full Deployment spec round-trip produces stable hash |
| UT-HASH-005 | Real K8s Pod spec with reordered containers | Containers in different order produce same hash |
| UT-HASH-006 | Empty spec, nil spec, empty map | Edge cases handled gracefully |
| UT-HASH-007 | Float precision (`replicas: 3` as float64 vs int) | JSON number representation is stable |
| UT-HASH-008 | Unicode string handling | Non-ASCII characters serialize correctly |
| UT-HASH-009 | Large spec (10KB+) | Performance and correctness at scale |
| UT-HASH-010 | Idempotency (1000 iterations) | Repeated hashing produces identical results |
| UT-HASH-011 | Nested slices of maps (`containers[].volumeMounts[]`) | Multi-level slice normalization |
| UT-HASH-012 | Mixed types in slices (string, number, bool, null, object) | Heterogeneous slices sort correctly |

---

## Hash Format

```
sha256:<64-char-lowercase-hex>
```

Example: `sha256:a1b2c3d4e5f6...` (total 71 characters including prefix)

The `sha256:` prefix provides:
- Forward compatibility if we ever change the algorithm (e.g., `sha512:`, `blake3:`)
- Self-documenting format in audit events and logs
- Consistent with OCI content-addressable storage conventions

---

## Spec Drift Guard (v1.1)

### Problem

Between the time EM computes the post-remediation hash and when it assesses metrics/alerts, another remediation (or external actor) may modify the target resource's `.spec`. If this happens, Prometheus metrics and AlertManager alerts no longer reflect the original remediation's effectiveness. Without a guard, EM would produce a misleading effectiveness score.

**Example scenario:**

```
T0: RR-1 completes remediation → Deployment spec = V1
T1: RO creates EA-1 (post-remediation state = V1)
T2: EM computes PostRemediationSpecHash = hash(V1)
T3: New alert fires → RR-2 starts → modifies Deployment spec → V2
T4: EM assesses EA-1's metrics/alerts → but resource is now V2
    → Prometheus metrics and AlertManager alerts reflect V2, NOT V1
    → Score would be attributed to RR-1 but actually measures RR-2's changes
```

### Decision

The EM reconciler re-hashes the target resource's `.spec` on **every reconcile loop** after the initial `PostRemediationSpecHash` has been computed. If the current hash differs from the stored post-remediation hash, the assessment is immediately completed with `reason=spec_drift`.

### Reconciler Integration (Step 6.5)

After Step 6 (Pending -> Assessing transition) and before Step 7 (component checks):

1. If `ea.Status.Components.HashComputed == true` and `PostRemediationSpecHash != ""`:
   a. Fetch the target resource's `.spec` via the K8s API (same as `getTargetSpec`)
   b. Compute `specHash = CanonicalSpecHash(spec)`. If this fails, log and skip drift check.
   c. Resolve ConfigMap content hashes via `resolveConfigMapHashes(ctx, spec, target)`
   d. Compute `currentHash = CompositeSpecHash(specHash, configMapHashes)`. If this fails, log and skip.
   e. Store `currentHash` in `ea.Status.Components.CurrentSpecHash`
   f. If `currentHash != PostRemediationSpecHash`:
      - Set `SpecIntegrity` condition to `False` with reason `SpecDrifted` (per DD-CRD-002)
      - Complete the EA with `AssessmentReason = spec_drift`
      - Emit `effectiveness.assessment.completed` audit event with `reason: "spec_drift"`
      - **Do NOT assess metrics or alerts** — they would measure the wrong resource state
   g. If hashes match: set `SpecIntegrity` condition to `True` with reason `SpecUnchanged`

### Audit Event

The `effectiveness.assessment.completed` audit event carries `reason: "spec_drift"` using the existing payload schema. No new event type is needed — `spec_drift` is a new value for the existing `reason` field alongside `full`, `partial`, `expired`, `no_execution`, and `metrics_timed_out`.

### DataStorage Scoring Impact

When DS computes the weighted effectiveness score and encounters `assessment_status == "spec_drift"`, it **short-circuits to score = 0.0** without running the weighted component calculation. This is a hard override because:

- Component scores (health, alerts, metrics) may reflect the drifted resource state, not the original remediation
- A nonzero score from health (which may already be assessed) would be misleading
- Spec drift means the remediation was unsuccessful — the system had to intervene again

### EA Status Fields

| Field | Location | Purpose |
|-------|----------|---------|
| `CurrentSpecHash` | `Status.Components.CurrentSpecHash` | Most recent hash of the target resource spec, re-computed each reconcile |
| `AssessmentReason` | `Status.AssessmentReason` | Set to `spec_drift` when drift detected |
| `SpecIntegrity` condition | `Status.Conditions` | `True`/`False` with reason `SpecUnchanged`/`SpecDrifted` (per DD-CRD-002) |

### Implications for HAPI Remediation History

When the HAPI team builds remediation history context for the LLM, a `spec_drift` assessment with score 0.0 provides clear context: "this workflow was applied but the remediated state did not hold — the resource had to be modified again." This helps the AI avoid recommending the same failing workflow for the same target resource.

---

## ConfigMap-Aware Composite Hashing (v1.3 — Issue #396)

### Problem

The original `CanonicalSpecHash` hashes only the resource's `.spec`. However, many workloads reference ConfigMaps that affect runtime behavior (config files, environment variables). If a ConfigMap's content changes without modifying the Deployment's `.spec`, the hash remains unchanged and drift goes undetected.

### Decision

Extend the hashing pipeline to produce a **composite hash** that incorporates both the resource spec hash and the content hashes of all referenced ConfigMaps. The composite hash is a single `sha256:<hex>` digest.

### Algorithm

```
CompositeSpecHash(specHash string, configMapHashes map[string]string) -> (sha256_hex_string, error)

1. If configMapHashes is nil or empty, return specHash unchanged (identity property).
2. Build a composite input string:
   a. Start with "spec:<specHash>"
   b. Sort configMapHashes keys alphabetically
   c. Append "cm:<name>=<hash>" for each entry
   d. Join all parts with newline separator
3. Compute SHA-256 of the composite input string
4. Return "sha256:<64-char-lowercase-hex>"
```

### Supporting Utilities

| Function | Location | Purpose |
|----------|----------|---------|
| `ExtractConfigMapRefs(spec, kind)` | `pkg/shared/hash/configmap.go` | Extracts ConfigMap names from 5 pod spec paths: `volumes[].configMap.name`, `volumes[].projected.sources[].configMap.name`, `containers[].envFrom[].configMapRef.name`, `containers[].env[].valueFrom.configMapKeyRef.name`, and their `initContainers`/`ephemeralContainers` equivalents. Kind-aware: resolves CronJob's nested `spec.jobTemplate.spec.template.spec`. |
| `ConfigMapDataHash(data, binaryData)` | `pkg/shared/hash/configmap.go` | Computes deterministic SHA-256 hash of a ConfigMap's `.data` and `.binaryData`. Binary values are base64-encoded before hashing. Keys are sorted alphabetically. |
| `CompositeSpecHash(specHash, configMapHashes)` | `pkg/shared/hash/configmap.go` | Combines the base spec hash with per-ConfigMap content hashes into a single composite digest. |

### ConfigMap Reference Paths

The following pod spec paths are scanned for ConfigMap references:

1. `spec.template.spec.volumes[].configMap.name`
2. `spec.template.spec.volumes[].projected.sources[].configMap.name`
3. `spec.template.spec.containers[].envFrom[].configMapRef.name`
4. `spec.template.spec.containers[].env[].valueFrom.configMapKeyRef.name`
5. `spec.template.spec.initContainers[]` — same envFrom/env paths as containers
6. `spec.template.spec.ephemeralContainers[]` — same envFrom/env paths as containers

For CronJob resources, the pod spec is located at `spec.jobTemplate.spec.template.spec`.

### Sentinel Value for Absent ConfigMaps

When a referenced ConfigMap cannot be fetched (404 Not Found or 403 Forbidden), a deterministic sentinel value is used instead of skipping the ConfigMap:

```
sentinelData = {"__sentinel__": "__absent:<configmap-name>__"}
sentinelHash = ConfigMapDataHash(sentinelData, nil)
```

This ensures:
- **Deterministic hashes**: The same absent ConfigMap always produces the same sentinel hash
- **Drift detection**: Creating or deleting a ConfigMap is detected as a hash change
- **Namespace isolation**: Each hash computation is scoped to a single namespace; the sentinel is name-dependent but namespace isolation is inherent from the call context

### RBAC Requirements

Both the RO and EM controllers require `get` permission on `configmaps` in target namespaces. This is configured via ClusterRole rules in the Helm charts:

- `charts/kubernaut/templates/remediationorchestrator/remediationorchestrator.yaml`
- `charts/kubernaut/templates/effectivenessmonitor/effectivenessmonitor.yaml`

### Error Handling

Both RO and EM use a **log-and-continue** strategy for ConfigMap fetch errors:
- **404 Not Found / 403 Forbidden**: Use sentinel hash, log at debug level
- **Other errors** (transient network, server errors): Log error, skip the ConfigMap
- **Hash computation errors**: Log error, skip the ConfigMap

This non-fatal approach prevents transient failures from blocking hash capture entirely.

### Identity Property

When a resource has no ConfigMap references (or all references are skipped due to errors), `CompositeSpecHash` returns the base spec hash unchanged. This preserves backward compatibility: resources without ConfigMap references produce the same hash as before v1.3.

---

## Related Decisions

- **ADR-EM-001** (v1.4): Effectiveness Monitor integration architecture, hash comparison in assessment Step 4
- **DD-017** (v2.4): Dual spec hash capture, Level 1 automated assessment scope
- **BR-EM-004**: Spec hash comparison to detect configuration drift
- **DD-HAPI-016**: Remediation history context (uses hash comparison for relevance filtering)

---

## Consequences

### Positive

- Single shared utility eliminates algorithm divergence between RO and EM
- Slice-order independence handles real-world Kubernetes API server behavior
- No external dependencies (standard library only: `crypto/sha256`, `encoding/json`, `sort`)
- Thoroughly tested with 12 dedicated test scenarios covering edge cases
- SHA-256 provides cryptographic-strength collision resistance

### Negative

- Slice-order independence adds ~30 lines of normalization code (vs plain `json.Marshal`)
- Cannot detect intentional reordering as a "change" (acceptable trade-off)
- Recursive normalization has O(n log n) cost for slice sorting (negligible for K8s specs, typically < 10KB)

### Risks

- Server-side defaulting across Kubernetes upgrades may cause false-positive drift detection
  - **Mitigation**: Document in operator runbook. Conservative detection is safer than missing drift.
- If the canonical JSON format changes in a future Go version, existing stored hashes become incomparable
  - **Mitigation**: The `sha256:` prefix enables algorithm versioning. Pin to Go's `encoding/json` behavior.
