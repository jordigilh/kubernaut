# Implementation Plan: Issue #2485 Context-Aware Secret Sanitization

**Status**: Implemented; focused validation complete. Investigator integration execution is blocked by the local envtest prerequisite `/usr/local/kubebuilder/bin/etcd`.  
**Branch**: `fix/2485-context-aware-secret-sanitization`  
**Business requirement**: BR-KA-211, FR-1/FR-2/FR-4  
**Test plan**: [TP-2485-v1.2](TEST_PLAN.md)  
**Design decision**: [DD-KA-2485](../../architecture/decisions/DD-KA-2485-context-aware-secret-sanitization.md)

## 1. Preflight and decision

- The false positive originates in the shared `k8s-secret-data` regex, which
  cannot distinguish `Node.spec.taints[].key` from `Secret.data.key`.
- The production boundary is `executeTool` in
  `internal/kubernautagent/investigator/investigator_tools.go`; the production
  pipeline is constructed by `buildSanitizationPipeline` in
  `cmd/kubernautagent/datastorage.go`.
- The shared Gateway/Notification sanitizer contract must remain unchanged.
- Selected approach: KA-scoped structured YAML/JSON Secret parsing plus isolated
  KA G4 rules. Identity fields are not classified as Secret data and are not
  selectively redacted by base64 shape; a general identity-privacy policy is a
  separate follow-up.

## 2. TDD execution

### RED

Added Ginkgo/Gomega regression scenarios for Kubernetes identifiers, topology,
labels/annotations, Secret/ConfigMap selector paths, projected-volume item
keys, YAML Secret data, SecretList/List, multi-document YAML, alternate YAML
representations, credential-bearing fields, nested Secret references,
production stage order, and `executeTool` delivery.

### GREEN

- Excluded only the unscoped `k8s-secret-data` rule from KA G4.
- Restricted KA plain credential patterns to same-line values.
- Extended `SecretSanitizer` with YAML node traversal for Secret, SecretList,
  generic List, and multi-document inputs.
- Wired K8S-SECRET before G4 and I1.

### REFACTOR

- Centralized YAML mapping traversal helpers.
- Kept shared sanitizer behavior unchanged.
- Used `json.RawMessage` for structured JSON redaction rather than
  `interface{}`-typed intermediate data.
- Added DD/BR/test-plan documentation and production pipeline-order coverage.

## 3. Wiring checkpoint

| Component | Production caller | Evidence |
|---|---|---|
| `SecretSanitizer` | `buildSanitizationPipeline` | `IT-KA-2485-003`, focused `cmd/kubernautagent` suite passes |
| KA G4 rules | `buildSanitizationPipeline` → `executeTool` | `UT-KA-2485-001/004`, `IT-KA-2485-001` |
| Sanitized tool delivery | `executeTool` LLM message boundary | `IT-KA-2485-001/002` (compiled; runtime blocked by envtest etcd) |

## 4. Validation evidence

| Check | Result |
|---|---|
| `go build ./...` | PASS |
| `go test ./... -run '^$' -timeout=30s` | PASS |
| Focused KA/shared suites | PASS |
| `go test ./cmd/kubernautagent -count=1` | PASS |
| Branch-delta `golangci-lint run --new-from-rev=origin/main --timeout=5m` | PASS (0 issues) |
| `git diff --check` | PASS |
| Investigator integration runtime | BLOCKED: missing `/usr/local/kubebuilder/bin/etcd` |
| `make test` | BLOCKED by unrelated parallel-load timing failures in effectivenessmonitor and signalprocessing; those scenarios pass in isolation |
