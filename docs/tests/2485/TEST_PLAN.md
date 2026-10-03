# Test Plan: Context-Aware Kubernetes Secret Sanitization

> **Template Version**: 2.0 — Hybrid IEEE 829-2008 + Kubernaut

**Test Plan Identifier**: TP-2485-v1.2
**Feature**: Prevent Kubernetes Secret heuristics from corrupting non-secret tool output while preserving Secret redaction
**Version**: 1.2
**Created**: 2026-10-03
**Author**: Kubernaut Team
**Status**: Implemented; focused validation complete. Investigator runtime IT is blocked by missing envtest etcd; repository-wide `make test` has unrelated parallel-load timing failures.
**Branch**: `fix/2485-context-aware-secret-sanitization`

---

## 1. Purpose and success criteria

Issue #2485 exposed that the KA G4 sanitizer treats any YAML line named `key`,
`username`, `token`, `secret`, or `credential` with a base64-shaped value as
Kubernetes Secret data. This can change investigation facts and workflow
parameters before they reach the LLM. The test effort proves that redaction is
context-aware and that the production `executeTool` path preserves useful
Kubernetes facts.

Success requires:

1. Non-Secret Kubernetes identifiers (taints, tolerations, selectors, topology,
   key references, and identity metadata) are not selectively redacted merely
   because their values resemble base64.
2. Every value below `Secret.data` and `Secret.stringData` remains redacted,
   including `data.key` and arbitrary entries such as `tls.crt`, for YAML and
   JSON `Secret`, `SecretList`, generic `List`, and multi-document YAML input.
3. Credential-bearing fields such as `TokenReview.spec.token` remain redacted.
4. Structured Secret-volume references are not corrupted by multiline generic
   credential matching.
5. The actual `executeTool` → sanitization pipeline → LLM message path proves
   the behavior; unit tests alone are insufficient.

Identity fields such as `TokenReview.status.user.username` and
`CertificateSigningRequest.spec.username` are potentially confidential identity
metadata, but are not Kubernetes Secret data. This issue does not silently
define a new identity-privacy policy; it only prevents their inconsistent,
base64-shape-driven treatment. A future explicit identity redaction or
pseudonymization policy requires its own acceptance criteria.

## 2. Authority and controls

- [Issue #2485](https://github.com/jordigilh/kubernaut/issues/2485)
- [BR-KA-211](../../requirements/BR-KA-211-llm-input-sanitization.md), FR-1,
  FR-2, and FR-4
- [DD-KA-005](../../architecture/decisions/DD-KA-005-llm-input-sanitization.md)
- FedRAMP `AC-4` (information-flow enforcement) and `SI-10` (input validation)
- SOC 2 `CC7.2` (monitoring and investigation evidence integrity)
- OWASP ASVS v5.0.0 `V1.3.3` (sanitize before a dangerous context),
  `V2.2.1` (validate input against an expected structure), and `V16.2.5`
  (protection-level handling for sensitive data)

## 3. Risks and mitigations

| ID | Risk | Impact | Affected scenarios | Mitigation |
|---|---|---|---|---|
| R1 | Regex redacts valid Kubernetes identifiers and changes workflow parameters. | High: remediation can target the wrong resource or fail silently. | UT-KA-2485-001, IT-KA-2485-001 | Test representative typed field paths and assert exact identifier preservation through the LLM-facing message. |
| R2 | Narrowing generic matching accidentally exposes credentials in ordinary text. | High: credentials can reach an external LLM. | UT-KA-2485-002/003, IT-KA-2485-002 | Keep same-line credential patterns; redact all Secret data/stringData values structurally; assert original sentinels are absent. |
| R3 | YAML parsing or re-encoding drops documents or changes non-secret content. | Medium/High: investigation context is lost. | UT-KA-2485-002/004 | Cover multi-document/List inputs, return unchanged non-Secret/invalid input, and assert semantic structure and preserved non-secret values. |
| R4 | The fix works in a stage test but production wiring omits/reorders it. | High: regression remains in real MCP output. | IT-KA-2485-001/002 | Exercise the production-equivalent pipeline through `executeTool` and assert the mock LLM receives the sanitized result. |
| R5 | Confidential identity metadata is confused with credential material. | Medium: either unnecessary disclosure or loss of RBAC investigation context. | UT-KA-2485-001 | Keep identity treatment explicit and consistent; do not claim this issue closes a general identity-privacy requirement. |

## 4. Scope

### In scope

- KA `CredentialSanitizer` rule composition.
- KA `SecretSanitizer` JSON and YAML handling.
- Secret, SecretList, generic List, and multi-document Kubernetes objects.
- Non-Secret Kubernetes field preservation and credential-field redaction.
- Production pipeline construction and `executeTool` delivery to the LLM.

### Out of scope

- Changing the shared Gateway/Notification default sanitizer contract.
- Introducing a general username/identity masking policy.
- Tool-error sanitization (existing BR-KA-211 FR-3 gap).
- Kubernetes API authorization or upstream MCP query behavior.

## 5. TDD approach

### RED

- `UT-KA-2485-001`: G4 preserves non-Secret Kubernetes identifiers and
  potentially confidential identity metadata instead of applying a
  base64-shaped heuristic.
- `UT-KA-2485-002`: YAML Secret data/stringData values, including `data.key`
  and arbitrary keys, are redacted.
- `UT-KA-2485-003`: SecretList/List, multi-document YAML, JSON Secret, and
  non-Secret/invalid passthrough behavior are correct.
- `UT-KA-2485-004`: actual token credentials remain redacted and Secret-volume
  structure is preserved.
- `UT-KA-2485-005`: non-Secret and invalid YAML is handled without changing
  ordinary investigation output.
- `UT-KA-2485-006`: topology keys, labels, annotations, Secret/ConfigMap
  selector keys, projected-volume item keys, and additional identity metadata
  remain unchanged.
- `UT-KA-2485-007`: flow-style, quoted-key, anchored, unknown-kind, and JSON
  generic-List representations retain the same context-aware behavior.
- `UT-KA-2485-008`: the structured Secret stage leaves non-Secret JSON
  annotations untouched; the separate shared annotation policy remains scoped
  independently.
- `IT-KA-2485-001`: a Node taint returned by `executeTool` reaches the LLM with
  its real key intact.
- `IT-KA-2485-002`: a YAML/JSON Secret returned by `executeTool` reaches the LLM
  without any original Secret value.
- `IT-KA-2485-003`: the production `buildSanitizationPipeline` wires
  `K8S-SECRET` before `G4` and `I1`.

### GREEN

- Remove the broad `k8s-secret-data` rule from the KA-specific G4 rule set.
- Keep same-line generic credential matching for ordinary text, without
  multiline YAML structure consumption.
- Extend the existing Secret stage with YAML-aware structural redaction.
- Wire the structure-aware stage before generic G4 in the production pipeline.

### REFACTOR

- Centralize YAML document traversal and map-value redaction.
- Keep shared sanitizer behavior unchanged outside KA.
- Update DD/BR implementation comments and test documentation to describe the
  KA/shared boundary and identity classification.
- Run the mandatory build, affected tests, lint, and anti-pattern checks.

## 6. Pyramid and wiring manifest

| Component | Production entry point | Wiring location | IT proof |
|---|---|---|---|
| Context-aware Secret stage | KA sanitization pipeline | `cmd/kubernautagent/datastorage.go` → `buildSanitizationPipeline` | IT-KA-2485-002/003 |
| KA-specific G4 rules | `NewCredentialSanitizer` in pipeline | `pkg/kubernautagent/tools/sanitization/credential.go` | IT-KA-2485-001 |
| Sanitized tool delivery | LLM-directed `executeTool` | `internal/kubernautagent/investigator/investigator_tools.go` | IT-KA-2485-001/002 |

Unit tests prove parser/rule behavior. Integration tests prove the production
dispatch boundary and the actual LLM-facing tool message. No new audit event is
emitted by this change; the security outcome is protection of the existing
tool-output information flow.

The shared JSON `annotations-json` rule remains a separate KA credential-policy
behavior: these regressions prove that the Kubernetes Secret-data rule does not
classify annotation map values as Secret data, without changing the existing
whole-annotation scrub used by the shared library.

## 7. BR/control coverage matrix

| Requirement/control | Business assertion | Test scenarios | Priority |
|---|---|---|---|
| BR-KA-211 FR-1 / FedRAMP AC-4 | Tool output crossing into the external LLM is sanitized without changing valid investigation facts. | UT-001, IT-001 | P0 |
| BR-KA-211 FR-2 / FedRAMP SI-10 | Secret structure is recognized before applying Secret-data redaction. | UT-002/003, IT-002 | P0 |
| BR-KA-211 FR-4 / SOC 2 CC7.2 | Sanitization does not corrupt evidence needed to reconstruct the investigation. | UT-001/004, IT-001 | P0 |
| ASVS `v5.0.0-1.3.3` | Untrusted tool content is sanitized before the LLM context boundary. | IT-001/002 | P0 |
| ASVS `v5.0.0-2.2.1` | YAML/JSON is checked against the expected Kubernetes Secret/List structure before structural redaction. | UT-002/003 | P0 |
| ASVS `v5.0.0-16.2.5` | Credential material is masked; potentially sensitive identity metadata is not inconsistently handled by a secret heuristic. | UT-001/002/004, IT-002 | P0 |

## 8. Environmental constraints and pass criteria

- Tests use Ginkgo/Gomega; standard `testing.T` remains only in existing suite
  bootstrap files and permitted benchmark/fuzz infrastructure.
- Focused unit tests must pass with `go test ./pkg/kubernautagent/tools/sanitization ./pkg/shared/sanitization -count=1`.
- The investigator integration suite requires `/usr/local/kubebuilder/bin/etcd`;
  if unavailable, the IT tier is recorded as blocked rather than weakened.
- PASS requires all P0 unit tests, all available P0 integration tests, no
  affected-suite regressions, and successful `go build ./...` plus lint.

## 9. Execution results

- RED failures reproduced the original false positives and YAML Secret gap.
- GREEN focused suites pass for `pkg/kubernautagent/tools/sanitization`,
  `pkg/shared/sanitization`, and `cmd/kubernautagent`.
- Repository build and compile-only test gates pass; branch-delta lint reports
  zero issues.
- The investigator suite compiles, but runtime execution cannot start because
  `/usr/local/kubebuilder/bin/etcd` is unavailable.
- `make test` exited non-zero due unrelated timing-sensitive failures in
  effectivenessmonitor and signalprocessing under its 12-process parallel
  runner. The affected scenarios pass when isolated; no files in those
  packages are changed by this branch.
