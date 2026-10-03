# DD-KA-2485: Context-Aware Kubernetes Secret Sanitization

**Status**: ✅ **Approved and implemented**  
**Date**: 2026-10-03  
**Decision Makers**: Kubernaut Agent Team  
**Business Requirement**: [BR-KA-211](../../requirements/BR-KA-211-llm-input-sanitization.md)  
**Issue**: [#2485](https://github.com/jordigilh/kubernaut/issues/2485)

## Context

The shared `k8s-secret-data` regular expression classified any YAML line named
`username`, `password`, `token`, `key`, `secret`, or `credential` with a
base64-shaped value as Kubernetes Secret data. KA applies that rule to every
successful tool result before it reaches the investigation LLM. Consequently,
valid facts such as `Node.spec.taints[].key: maintenance` were changed to
`[REDACTED]`, and workflow parameters derived from those facts became invalid.

The same leaf name can have different security meanings. `Secret.data.key` is
credential material and must be masked; `SecretKeySelector.key` and a taint key
are identifiers; `TokenReview.status.user.username` is potentially confidential
identity metadata but is not Secret data. A base64-shape heuristic cannot make
those distinctions consistently.

## Alternatives considered

### A — Regex-only indentation/context matching

Require a preceding `data:` line or infer indentation in the existing regular
expression.

**Rejected:** YAML permits nested objects, lists, aliases, block scalars, and
multi-document streams. A regular expression cannot reliably establish the
`Secret.data`/`stringData` context or preserve arbitrary Secret keys such as
`tls.crt`.

### B — Move structured parsing into the shared sanitizer

Make `pkg/shared/sanitization` parse Kubernetes YAML/JSON for all Gateway,
Notification, and KA callers.

**Rejected for this issue:** it broadens the behavior contract of shared log
sanitization and its fallback semantics across multiple services. That change
would require a separate cross-service compatibility review.

### C — KA-scoped structured Secret stage and isolated G4 rules (selected)

Keep the shared library available for ordinary credential patterns, but exclude
the broad `k8s-secret-data` rule from KA's G4 rule set. Extend the existing
`K8S-SECRET` stage to parse Kubernetes JSON and YAML and redact only
`Secret.data`/`Secret.stringData` values. Restrict KA's plain credential rules
to horizontal whitespace so nested YAML mappings are not consumed.

**Selected:** this fixes the affected LLM information-flow boundary, preserves
Gateway/Notification behavior, supports arbitrary Secret data keys, and keeps
the change scoped to the service with the demonstrated production failure.

## Decision

The KA pipeline order is:

```text
K8S-SECRET (structured Secret redaction) → G4 (generic credentials) → I1
```

`SecretSanitizer`:

- recognizes JSON and YAML `Secret`, `SecretList`, generic `List`, and
  multi-document YAML resources;
- replaces every value under `data` and `stringData`, including `data.key` and
  arbitrary keys;
- returns non-Secret, invalid, and unchanged input unchanged;
- uses `yaml.v3`, already a direct project dependency.

`CredentialSanitizer`:

- retains the shared generic credential rules and enhanced Authorization rule;
- excludes only the unscoped `k8s-secret-data` rule for KA;
- uses same-line variants for plain password/API-key/token/secret/credential
  fields, preventing nested YAML mapping corruption;
- does not introduce an identity masking policy. Potentially confidential
  identity fields remain a separate future policy decision.

## Consequences

### Positive

- Valid Kubernetes taint, selector, topology, and key-reference facts are not
  redacted because their values resemble base64.
- Actual Secret data remains protected, including arbitrary and `key` entries.
- Credential fields outside Secret objects remain covered by G4.
- Shared Gateway/Notification sanitizer behavior is unchanged.

### Negative and follow-up

- YAML Secret output may be re-encoded when a redaction occurs; tests assert
  semantic preservation rather than byte identity for changed Secret documents.
- Tool-error messages still bypass the pipeline, as tracked by BR-KA-211 FR-3.
- A general policy for masking or pseudonymizing confidential Kubernetes
  identity metadata remains out of scope.

## Validation

- Unit coverage: `pkg/kubernautagent/tools/sanitization` validates non-Secret
  field preservation, YAML/JSON Secret redaction, List/multi-document handling,
  nested Secret references, and credential-vs-identity classification.
- Integration coverage: `executeTool` tests are added to validate the actual
  LLM-facing message for both a Node taint and YAML Secret output; compilation
  passes, but runtime execution is blocked locally by missing envtest etcd.
- Controls: BR-KA-211 FR-1/FR-2/FR-4, FedRAMP AC-4/SI-10, SOC 2 CC7.2, and
  OWASP ASVS v5.0.0 V1.3.3/V2.2.1/V16.2.5.
