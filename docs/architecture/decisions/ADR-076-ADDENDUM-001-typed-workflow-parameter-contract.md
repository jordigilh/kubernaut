# ADR-076 Addendum 001: Typed Workflow Parameter Contract

- **Status**: Approved — implementation complete; CI validation pending
- **Date**: 2026-11-06
- **Version**: 1.0
- **Issue**: [#2575](https://github.com/jordigilh/kubernaut/issues/2575)
- **Parent**: [ADR-076](ADR-076-rar-approval-snapshot-and-terminal-decision-contract.md)
- **Related**: [DD-WORKFLOW-018](DD-WORKFLOW-018-etcd-single-source-of-truth.md), [DD-AUDIT-004](DD-AUDIT-004-RR-RECONSTRUCTION-FIELD-MAPPING.md), [implementation plan](../../tests/2575/IMPLEMENTATION_PLAN.md), [test plan](../../tests/2575/TEST_PLAN.md)

## Context

ADR-076 specifies string-valued execution transport and a frozen approval
snapshot. DD-WORKFLOW-018 establishes the workflow CRD catalog and describes
an execution snapshot. The #2575 contract changes the parameter transport and
schema-copy parts of those records, not their approval-safety guarantees. This
addendum supersedes only those specified details; in particular,
DD-WORKFLOW-018 contains a separate proposed #2570 amendment, not an accepted
claim by this addendum.

Before #2575, `SelectedWorkflow` contained string parameters and
`ParameterSchema`; `WorkflowSnapshot` contained `DeclaredParameterNames`;
`workflowidentity.Verify` returned only an error; the Ansible executor's
`coerceValue` guessed types from strings; and Data Storage audit normalization
could round large numbers through default interface-valued JSON decoding. The
implementation replaces those paths with the shared typed union, an ephemeral
verified workflow view, native Ansible values, and number-safe audit decoding.

**Requirements**: BR-WORKFLOW-004/006, BR-KA-191, BR-ORCH-025/026,
BR-WE-015, BR-AUDIT-005/006 and BR-SECURITY-016. The approved #2575 plans
define scope and test IDs; this approved addendum records their architectural
supersession boundary.

## Decision drivers

Preserve typed values and exact numeric evidence; retain immutable review and
target safety; use one trusted schema authority; fail closed across every
reconcile state; support reconstructable, actor-attributed audit; avoid
ambiguous mixed-version operation.

## Alternatives considered

| Alternative | Advantages | Decision and costs |
|---|---|---|
| Named raw-JSON value map, e.g. `map[string]apiextensionsv1.JSON` | Typed JSON tokens without a public unbounded Go interface map | Rejected after the API-server/CEL failure documented in the spike report. |
| Structural discriminated value union | CEL-visible typed fields, explicit tags and validation | **Selected and implemented**; primitive-only arrays, immutability, code generation and API-server round trips passed the disposable qualification. Numeric lexemes remain separate lossless evidence. |
| Retain `map[string]string` | Minimal change to existing execution APIs | Rejected: loses native cross-CR types and requires schema-directed decoding. |
| Persist `map[string]any` or support both legacy and typed readers | Convenient generic transport or rolling compatibility | Rejected: public unbounded union or ambiguous provenance/unsafe mixed-version execution. Internal LLM DTOs are not persisted schema authorities. |

## Decision

### Authority, values and approval

The trusted `RemediationWorkflow.spec.parameters` is the sole parameter schema
for validation, declared-name filtering, defaults metadata and conversion.
AgentSession, AIAnalysis (AIA), RemediationApprovalRequest (RAR) and
WorkflowExecution (WE) use one normalized typed JSON representation and carry
no persisted parameter-schema snapshot or schema-derived allowlist. Retain
reviewed original and effective values, workflow identity, immutable approval
decision and target locking, collision behavior, and non-schema execution
metadata. Omitted override keys retain their values; absent, null and `{}`
overrides are no-ops; new keys and changes to locked `TARGET_RESOURCE_*` keys
are rejected. No schema default is inserted.

Strings, signed-int64-safe integer tokens, finite floats, booleans and arrays
are typed JSON values. Reject null and object parameter values, malformed JSON,
fractional or exponent integer spellings, float overflow and nonzero underflow.
Arrays contain only primitive string, signed integer, finite float or boolean
elements, including heterogeneous arrays; nested arrays and object elements
are rejected. Array order and zero, false, empty-string and empty-array values survive.
Absent or null selected/original/effective maps fail closed; `{}` is present
and subject to live schema validation. Preserve observable override omission,
null and empty-object distinctions on the wire and in audit evidence. Use
lexicographically sorted keys and compact JSON for deterministic comparison,
without float64 integer conversion or rewriting submitted numeric lexemes.
Retain the existing number-safe `UseNumber` and raw-parameter ingress paths.

### Identity and reconcile invariant

Use the configured trusted namespace, without default or cluster-wide fallback.
Require exactly one workflow with the frozen `status.workflowId`, and recompute
its clean-content-derived ID; if `status.contentHash` is present, it must match
the recomputed hash. No name/version fallback or substitution of live values for
reviewed values is permitted. Missing, ambiguous, foreign, malformed or
changed content fails closed. Successful no-match and configuration or
authorization errors are not retried; transient API/network errors retain at
most three total cancellable attempts. A successful check is point-in-time, not
an atomic deletion barrier. `status.workflowId` is the sole required revision
fence; a missing `status.contentHash` is acceptable. If a future contract
supplies a content hash, verify it against the recomputed hash.

Verify before AgentSession-to-AIA persistence, RO child RAR/WE creation, AW
decision mutation, and every applicable WE state transition, reconcile
completion, audit/status update, or external-resource creation/recreation.
An absent WFE returns without a business transition. If a WFE lacks a finalizer
and verification fails, return/requeue an observable error without finalizer or
status mutation, including no terminal status. If Pending has a finalizer,
verification failure may mark Failed with an observable reason and must create
no resource. Running and terminal processing fail closed before transition or
completion. The sole approved bypass is deletion/finalizer removal limited to
cleanup of the finalizer or owned resources, with no execution or approval
mutation or successful workflow-processing claim; production-path tests must
prove this exception.

### Engine and audit boundaries

At the executor boundary, Job and Tekton receive string values as raw string
contents and non-strings as compact JSON text; filtering uses an ephemeral
view of the verified live schema. Ansible/AWX receives raw typed JSON in
`extra_vars`; remove heuristic coercion. No invalid value silently becomes a
string.

Audit events carry event-specific typed original, submitted override and/or
effective values, as available, plus actor, correlation, workflow identity,
outcome and applicable rationale and error details. Each parameter-bearing
event also carries separate lossless text/byte `parameter_evidence`, preserving
numeric tokens such as `9007199254740993`, `1`, `1.0` and `1e0`; JSONB is not
lexical evidence. Single-create and batch audit persistence must decode numbers
safely and preserve hash/readback consistency. Audit remains best-effort;
enqueue failures are observable and not claimed as durable success.

Reconstruction queries remediation lifecycle events by `correlation_id`,
extracts `workflow_id`, and joins workflow admission create/update events by
that ID to recover the admitted content and hash. Missing admission evidence
produces an explicit incomplete/failure outcome with an operator-visible
reason, never a live-CRD or snapshot fallback. Event ownership is defined in
the [implementation plan](../../tests/2575/IMPLEMENTATION_PLAN.md#39-audit-contract).

### Cutover

This is a coordinated breaking release: quiesce legacy writers/controllers,
inventory active pre-cutover AgentSession, AIA, RAR and WE objects, and block
activation until the inventory is clean. Install typed CRDs/OpenAPI before
enabling typed services. No discriminator, dual reader, drain mechanism,
runtime conversion or silent adoption of legacy strings is allowed. Rollback to
legacy-only software is permitted only before typed objects exist; thereafter
rollback requires a release implementing the same typed contract.

## Consequences and affected components

Schema drift and heuristic coercion are removed, at the cost of extra trusted
API reads, a coordinated release and a point-in-time lookup race. Affected
paths include KA/AgentSession, AIA, RO, AF, AW, WE and its three executors,
shared identity and parameter validation, CRDs and embedded assets, RBAC,
Data Storage OpenAPI/ogen clients, audit emitters and create/batch persistence.
The [wiring manifest](../../tests/2575/IMPLEMENTATION_PLAN.md#7-wiring-manifest)
and [test plan](../../tests/2575/TEST_PLAN.md) define the required production
entry points and integration/E2E proof; unit tests alone do not establish wiring.

Security and compliance: fail-closed identity and schema validation support
AC-4, AC-6 and SI-10; actor-attributed and reconstructable audit supports AU-2,
AU-3 and SOC2 CC7.2, with AU-9/AU-11 assessed against existing protection and
retention controls. Lossless evidence may contain sensitive operator input;
this decision does not claim automatic secret redaction. Protect it under the
existing audit controls and assess applicable control objectives in IT/E2E.

## Representation and code-generation qualification — passed; implementation complete locally

The raw-map candidate and the first recursive-union candidate failed API-server
CRD admission; see the
[spike report](../../tests/2575/SPIKE_REPORT.md). The owner then selected and
approved the named structural discriminated value union with CEL-visible fields.
The complete disposable qualification passed for primitive-only typed arrays,
immutability, generated artifacts, API-server round trips, PostgreSQL
persistence, and lossless `parameter_evidence`. Production integration and
deployed evidence remain separate release gates; this does not authorize
weakened CEL immutability or an unapproved third fallback.

Run in an isolated disposable workspace after authorization, never by modifying
protected records or treating temporary generated artifacts as implementation.
Record commit, Go/controller-gen/ogen/Kubernetes versions, commands, generated
schema excerpts, raw API requests/responses and results. Existing repository
commands `make manifests` and `make generate` update multiple generated and
embedded files; run them only in the isolated workspace and inspect their diff.

1. Define the selected disposable named discriminated union on representative
   CRD fields, including the AgentSession raw selection boundary. Exercise Go
   marshal/unmarshal and deep-copy with absent, null, `{}`, empty values,
   strings, booleans, arrays, `9007199254740993`, `1`, `1.0` and `1e0`.
   Compare raw tokens as well as semantic types; test mutation isolation.
2. Generate CRDs and inspect structural OpenAPI at each actual map-value path;
   validate the schema with a real API server, rather than assuming an existing
   raw JSON field proves map-value behavior. Generate the audit OpenAPI/ogen
   client for a representative typed map and text/byte evidence field; verify
   the generated types and encode/decode without public `map[string]any` or
   float64 number loss. Investigate any generator rejection rather than
   asserting a particular schema marker or generated type in advance.
3. Install disposable CRDs into envtest, create and read representative
   AgentSession, AIA, RAR and WE objects through the API server, and inspect raw
   JSON as well as typed clients. Test omitted, explicit null and `{}` separately
   at selected/original/effective and override boundaries; verify zero-like
   values and type fidelity, no pruning, and the required distinction between
   numeric evidence and any server-normalized semantic value. Include status
   subresource writes where applicable. Exercise invalid value rejection at the
   intended validation boundary; do not assume structural schema alone enforces
   the business grammar.
4. Run the repository generation and generated-artifact drift checks in the
   disposable workspace; inspect CRD bases, embedded copies, deepcopy and
   generated audit client output. Repeat generation to establish idempotence.
   Record any unavailable envtest/API server or generator as **blocked**, not
   pass. Use the approved plans' real persistence and production-wiring tests
   for implementation; this spike does not replace them.

**Pass:** all representative paths preserve required types and presence,
large integers do not round, raw evidence distinguishes all three numeric
spellings, generated schemas are accepted without pruning, generated audit
client encoding is compatible, and generation is repeatable. Exact CRD token
preservation must be demonstrated wherever the design relies on it; if the API
server normalizes a token, the evidence capture path must demonstrably retain
the original before that boundary. **Fail:** any lost presence/type/evidence,
rounding, pruning, unsupported generation, or incompatible client. A failed
candidate blocks implementation; document the result and seek an owner decision
before changing the approved contract. No further fallback is automatically
approved by this document.

## Production verification gate

The owner has accepted this addendum and the policy and cleanup exception above.
Production code and generated artifacts are present and locally validated after
the qualification passed. Follow the approved plans' remaining production
cutover, wiring, security and audit gates; do not modify the parent decisions or
protected #2570/#2601 records as a substitute for this amendment.
