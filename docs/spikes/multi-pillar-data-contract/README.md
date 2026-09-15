# Spike: Multi-Pillar Data Contract Extensibility

**Status**: Complete; recommendation ready for design approval
**Date**: 2026-09-14
**Owner**: Jordi Gil
**Related capabilities**: Threat Remediation, Cost Optimization, Compliance Remediation, and Supply Chain Security product discovery

## Question

How should pillar-specific data flow through Signal Processing and the downstream remediation services when future capabilities, such as security threat response, cost optimization, compliance remediation, and supply chain security, require data that does not belong to the existing alert model?

The goal is to avoid changing every CRD for every new pillar while retaining validation, versioning, typed service contracts, safe handling of untrusted data, and complete audit reconstruction.

This spike uses synthetic data only. No Red Hat-specific source payloads, templates, or confidential documents are included in this repository.

## Conclusion

Adopt a hybrid data-contract model:

- Keep universal lifecycle and routing fields strongly typed.
- Add an explicit pillar discriminator without changing the existing meaning of `signalType`.
- Carry versioned pillar data through controlled extension points using Kubernetes-native preserved JSON.
- Decode and validate pillar data at the owning service boundary.
- Propagate normalized, typed pillar projections downstream instead of copying the entire raw payload through every CRD.
- Use separate pillar resources when evidence is large, sensitive, independently retained, or updated asynchronously.

Do not make the entire SP CRD or the entire remediation loop unstructured.

### Extensibility invariant

Adding a new pillar should be an additive change at the pillar boundary. It should not require changing unrelated pillar schemas, expanding every shared CRD, or refactoring services that do not consume the new pillar. The expected implementation footprint is a new pillar-owned schema and decoder, explicit registry entry, typed investigation and verification projections, policy inputs, workflow catalog, audit/redaction mapping, and focused tests.

## Evidence Reviewed

- `api/signalprocessing/v1alpha1/signalprocessing_types.go:100-155`
- `api/remediation/v1alpha1/remediationrequest_types.go:335-408`
- `api/aianalysis/v1alpha1/aianalysis_types.go:125-204`
- `api/agentsession/v1alpha1/agentsession_types.go:220-358`
- `api/remediationworkflow/v1alpha1/remediationworkflow_types.go:58-68`
- `config/crd/bases/kubernaut.ai_signalprocessings.yaml:151-210`
- `config/crd/bases/kubernaut.ai_remediationrequests.yaml:117-241`
- `docs/architecture/CRD_SCHEMAS.md:194-213, 664-685, 707-724`
- `docs/architecture/decisions/DD-GATEWAY-010-adapter-naming-convention.md:10-16, 128-141`
- `docs/architecture/proposals/PROPOSAL-EXT-007-pre-investigation-pipeline.md:1-3, 28-30, 150-160`
- `docs/architecture/proposals/PROPOSAL-EXT-002-investigation-prompt-bundles.md:14-20, 24-30, 178-193` (historical strategic precedent; implementation mechanism superseded)
- `docs/roadmap/ROADMAP.md:78`
- `docs/services/test-infrastructure/mock-llm/BUSINESS_REQUIREMENTS.md:872-890`
- External multi-pillar platform analysis provided for this spike (2026-09-10; maintained privately and not copied or linked here)
- `pkg/gateway/processing/crd_creator.go:241-275, 396-424`
- `pkg/remediationorchestrator/creator/signalprocessing.go:57-96`
- `pkg/remediationorchestrator/creator/aianalysis.go:166-214`
- `pkg/aianalysis/handlers/request_builder.go:59-125`
- `pkg/remediationorchestrator/creator/workflowexecution.go:102-176`
- `pkg/remediationorchestrator/creator/effectivenessassessment.go:126-154`
- `pkg/gateway/audit_emission.go:152-225`
- `pkg/datastorage/reconstruction/parser.go:289-309`
- `pkg/datastorage/reconstruction/mapper.go:162-169`

## Current-State Findings

### Existing provider-data pattern

The current RemediationRequest and SignalProcessing models already separate common fields from provider-specific data. Common fields include fingerprint, source, target type, target resource, labels, annotations, and timestamps. Provider-specific content is stored in `ProviderData`.

The live Go APIs and generated CRDs currently represent `ProviderData` as a JSON string. The architecture documentation describes it as `json.RawMessage`. This is a real contract drift and must be resolved before adding a new pillar payload.

The existing string representation was intentional for issue #96: it avoids unwanted base64 behavior when storing JSON in the CRD. Changing the type requires a migration and serialization spike; it must not be treated as a mechanical type substitution.

### Existing evolving-JSON precedent

AgentSession already uses `*apiextensionsv1.JSON` with `x-kubernetes-preserve-unknown-fields` for evolving fields such as enrichment results, root-cause analysis, selected workflow, and detected labels. Its comments explicitly define these fields as free-form because the KA schema evolves independently of the CRD.

RemediationWorkflow also uses `*apiextensionsv1.JSON` for detected labels and engine configuration. This establishes a project precedent for narrowly scoped extensible fields, not for making complete CRDs unstructured.

### Multi-pillar product precedent

The multi-pillar platform analysis describes Kubernaut's common control plane as the reusable part: signal intake, enrichment, investigation orchestration, workflow selection, approval, deterministic execution, effectiveness assessment, notification, and audit reconstruction. Domain agents, tools, investigation logic, and response catalogs vary by pillar.

The identified pillars are broader than threat and cost:

- Supply Chain Security: SBOM, advisory, exploitability, reachability, patch, backport, and residual-exposure evidence.
- Threat Remediation: runtime and security findings, principals, attack stage, blast radius, containment, and forensic constraints.
- Compliance Remediation: control violations, policy drift, evidence collection, exceptions, compensating controls, and corrective workflows.
- Cost Optimization: utilization, allocation, cost attribution, budgets, forecasts, rightsizing, and savings verification.

The analysis relates Compliance Remediation closely to Threat Remediation, while this spike treats each as an independently selectable pillar for contract extensibility. Product packaging may group them later, but the shared CRD contract must not assume that relationship.

### Signal naming and pillar constraint

`DD-GATEWAY-010` separates external source identity from internal signal classification. The current revision also states that signal types are normalized to the generic value `alert`, while source identifies the adapter or monitoring system.

Therefore, `signalType` should not be repurposed to distinguish `alert`, `threat`, `cost`, `compliance`, or `supply-chain`. A new pillar dimension is required. The existing mock-LLM requirements already use `Pillar` for this concept and name alert remediation, threat remediation, and cost optimization as separate pillars.

The proposed dimensions are:

- `signalSource`: the external producer, such as Prometheus, RHACS, Falco, Koku, Kubecost, OpenCost, policy scanners, or compliance systems.
- `signalType`: the normalized signal shape. Existing alert compatibility must be preserved.
- `targetType`: the infrastructure or platform system being addressed.
- `pillar`: the Kubernaut capability profile selected for this remediation loop, such as `alert-remediation`, `threat-remediation`, `compliance-remediation`, `supply-chain-security`, or `cost-optimization`.
- `schemaVersion`: the version of the pillar-specific contract.

`pillar` is a processing profile, not a replacement for source or signal type. A future design may allow one source signal to produce more than one pillar-specific remediation request, but that fan-out should be explicit rather than hidden in a polymorphic field.

The known and anticipated pillars demonstrate why the shared contract must stay small:

- Threat Remediation needs security findings, principals, attack stage, indicators, blast radius, containment constraints, and forensic requirements.
- Cost Optimization needs utilization, allocation, cost center, budget, pricing period, forecast, savings estimate, and rightsizing constraints.
- Compliance Remediation needs framework and control identifiers, violation details, evidence, exception or compensating-control state, remediation deadlines, and evidence validity.
- Supply Chain Security needs SBOM and advisory identity, exploitability, reachability, affected artifact and workload scope, patch or backport state, and residual exposure.

These sets belong in neither universal SignalProcessing, AIAnalysis, WorkflowExecution, nor EffectivenessAssessment fields. They are pillar contexts carried through the extension boundary and projected only where a service needs them.

### Signal Processing boundary

The rejected pre-investigation proposal confirms that SP remains responsible for deterministic normalization, enrichment, and consolidation. Cross-domain root-cause reasoning belongs to the downstream investigation agent.

SP may therefore decode and validate a pillar payload and provide deterministic pillar context. For threat remediation this means security evidence normalization and deterministic context enrichment. For cost optimization this could mean allocation, utilization, budget, pricing-period, and forecast context. For compliance this could mean control and evidence normalization. For supply chain security this could mean advisory, artifact, reachability, and exposure normalization. SP should not become the owner of threat investigation, FinOps reasoning, compliance reasoning, supply chain reasoning, LLM triage, or cross-domain reasoning as a side effect of adding a data extension.

### Immutable-spec constraint

RemediationRequest spec data is immutable after creation. This is appropriate for the original signal and its provenance, but it means asynchronously arriving evidence or evolving investigation context should not be modeled as mutations to the original signal payload. Such data belongs in status, a child resource, or an independently retained evidence store with references and correlation identifiers. This matters for threat telemetry, supply-chain evidence, compliance evidence, and cost data that may arrive asynchronously or over a measurement period.

### Propagation trace and readiness result

The current production path was traced using the existing common fields and provider payload:

1. Gateway creates `RemediationRequest`, writes the source payload to the immutable `ProviderData` string, and emits a correlated `gateway.signal.received` event.
2. Remediation Orchestrator copies the common signal fields and `ProviderData` into `SignalProcessing`.
3. SignalProcessing produces normalized severity, signal classification, priority, environment, and enrichment status.
4. Remediation Orchestrator builds `AIAnalysis.Spec.AnalysisRequest.SignalContext` from the normalized SP status and enrichment results. The current builder does not carry pillar identity or pillar data.
5. AIAnalysis builds `AgentSession.Spec` from that context. The current builder carries enrichment and annotations but no pillar projection. Its Rego policy input also has no pillar dimension.
6. WorkflowExecution receives the selected workflow snapshot, target, parameters, and execution metadata. It should receive the governed action plan, not the original pillar payload.
7. EffectivenessAssessment receives the RR correlation ID, signal/remediation targets, and timing configuration. It needs a pillar-specific verification plan or evidence reference when generic alert/hash checks are insufficient.
8. Data Storage reconstructs `ProviderData` from audit events, including AI analysis provider-response summaries. A future pillar envelope therefore needs explicit audit fields or a correlated child-resource reference; it must not be hidden only in an opaque transcript.

Readiness result: the common lifecycle is already wired, but pillar context currently drops at the SP-to-AIAnalysis boundary. The minimum additive implementation is a validated pillar projection on the AIAnalysis/AgentSession path, a pillar-aware policy input, and pillar-aware verification/audit references. WFE and unrelated services should remain unchanged unless they consume a specific pillar projection.

## Prototype Results

A temporary Go probe used synthetic data containing a pillar, schema version, known fields, and an unknown future field. The probe was deleted after execution.

Both `apiextensionsv1.JSON` and `runtime.RawExtension` successfully unmarshaled and re-marshaled the nested object without losing the unknown future field:

```text
apiextensionsv1.JSON round-trip: {"pillar":"cost-optimization","schemaVersion":"v1","data":{"costCenter":"platform","forecastPeriod":"monthly","unknownFutureField":{"enabled":true}}}
runtime.RawExtension round-trip: {"pillar":"cost-optimization","schemaVersion":"v1","data":{"costCenter":"platform","forecastPeriod":"monthly","unknownFutureField":{"enabled":true}}}
```

The probe proves preservation, not correctness. Neither type provides automatic domain dispatch, semantic validation, authorization, redaction, or safe handling of an unknown schema version. Those behaviors must be implemented explicitly at the domain boundary.

### Serialization readiness decision

- Keep the current `ProviderData string` and `OriginalPayload string` contracts unchanged for the first pillar implementation. Existing Gateway, reconstruction, and integration tests depend on parseable JSON text without a base64 layer.
- Add the new pillar envelope as an optional, additive field rather than mechanically changing `ProviderData` to `json.RawMessage` or `apiextensionsv1.JSON`.
- The envelope should contain an explicit pillar, a pillar-local kind, a schema version, and preserved JSON data. The final top-level name (`signalClass` versus `pillarData`) remains a design-review decision, but it must be one stable extension point rather than one new universal field per pillar.
- Treat `ProviderData` as legacy/source evidence and the new envelope as normalized pillar context. Do not create two competing normalized representations or silently prefer one based on consumer behavior.
- The first implementation must add decoder/version tests and audit/redaction tests before any deprecation or migration of `ProviderData` is considered.

## Spike Validation

- A temporary Go probe passed JSON and YAML round trips for the existing `ProviderData` string contract, preserving parseable JSON and the unknown future field.
- The same probe passed `apiextensionsv1.JSON` and `runtime.RawExtension` round trips for `threat-remediation`, `cost-optimization`, `compliance-remediation`, and `supply-chain-security` synthetic envelopes.
- Existing BDD coverage passed with `go test ./pkg/gateway/processing -count=1 -ginkgo.focus='ProviderData|Issue #96'`.
- Existing orchestrator creator coverage passed with `go test ./pkg/remediationorchestrator/creator -count=1`.
- The propagation trace was verified against the current Gateway, Remediation Orchestrator, SignalProcessing, AIAnalysis, AgentSession, WorkflowExecution, EffectivenessAssessment, audit, and reconstruction code paths listed above.

The probe validates serialization behavior only. It does not validate Kubernetes API-server admission, generated CRD OpenAPI/CEL behavior, decoder semantics, or a new pillar field because production types and CRDs remain unchanged by this spike.

## Readiness Decision

The architecture is ready for a design decision review, but implementation is not yet authorized by this spike. Before changing APIs or CRDs, the implementation plan must settle the final envelope name and placement, run an envtest or API-server schema check, define the legacy `ProviderData` compatibility boundary, and specify the typed projections and audit events for the first pillar. No separate architecture is required for Supply Chain Security or Compliance Remediation; they should plug into the same extension and projection model.

## Alternatives

### Alternative A: Fully typed discriminated union

Example shape:

```yaml
pillar: cost-optimization
signalType: alert
pillarData:
  costCenter: ...
  forecastPeriod: ...
```

Advantages:

- Strong compile-time contracts.
- Excellent CRD validation and generated client support.
- Clear API and `kubectl` behavior.

Costs:

- Every new domain changes shared CRDs and generated code.
- All consumers become coupled to every domain.
- Large unions become difficult to version and maintain.

Assessment: appropriate for universal fields and stable domain projections; not suitable as the only extensibility mechanism.

### Alternative B: Fully unstructured CRDs

Example shape:

```yaml
spec:
  data: {}
```

Advantages:

- New domains do not require immediate CRD shape changes.
- Arbitrary payloads can be transported.

Costs:

- Weak admission validation and poor API discoverability.
- Runtime failures replace schema-time failures.
- Generated clients and typed service contracts lose value.
- Security review becomes harder because arbitrary fields cross trust boundaries.
- CEL and server-side validation cannot safely reason about preserved unknown fields.
- The project would spread `any`/map-based handling and duplicate parsing logic.

Assessment: reject as the default architecture.

### Alternative C: Typed common envelope with versioned pillar extension

Conceptual shape:

```yaml
signalClass:
  pillar: cost-optimization
  kind: cost-observation
  schemaVersion: v1
common:
  fingerprint: ...
  source: ...
  targetResource: ...
pillarData:
  ...
```

Advantages:

- Stable universal contract.
- New pillars can evolve without adding a field for every pillar to every CRD.
- Pillar ownership and versioning are explicit.
- Preserved JSON can bridge independently evolving schemas.
- Downstream services can receive typed projections rather than raw payloads.

Costs:

- Requires an explicit decoder registry keyed by pillar and schema version.
- Pillar payload validation moves to runtime/controller code.
- Preserved JSON cannot be used directly in CEL expressions.
- Each domain still needs its own fixtures, schema tests, redaction rules, and migration path.

Assessment: recommended.

### Alternative D: Separate domain resource

Example: a `SecuritySignal`, `ThreatContext`, `ComplianceContext`, `SupplyChainContext`, or `CostContext` resource referenced by SignalProcessing and AIAnalysis.

Advantages:

- Security owns its schema and lifecycle independently.
- Large or sensitive evidence does not inflate every remediation CRD.
- Evidence can be updated, retained, redacted, or access-controlled independently.

Costs:

- More objects, watches, reads, and failure modes.
- Reference consistency and garbage collection require design.
- Audit reconstruction needs explicit resource and correlation events.
- The first implementation is heavier than an inline extension.

Assessment: use as a complement to Alternative C when data is large, sensitive, asynchronous, or independently retained.

## Recommended Contract

The recommended model has three layers.

### Layer 1: Universal typed envelope

These fields are stable and should remain typed across domains:

- Signal fingerprint and incident correlation identity.
- Signal source.
- Signal type.
- Selected pillar or capability profile.
- Schema version.
- Target type and target resource.
- Cluster identity.
- Received and firing timestamps.
- Normalized severity and routing metadata.

The recommended product term is `pillar`, aligned with the existing mock-LLM extensibility requirement. It may be nested under a `signalClass` object if the API needs a grouped discriminator. `signalType` should retain its existing signal-shape semantics. The final wire shape requires a design decision after checking all API and metrics consumers.

### Layer 2: Pillar extension at the owning boundary

The pillar payload should be represented by a narrowly scoped preserved JSON field, preferably using the existing `apiextensionsv1.JSON` convention. A conceptual Go shape is:

```go
type PillarData struct {
	Pillar        string                `json:"pillar"`
	Kind          string                `json:"kind"`
	SchemaVersion string                `json:"schemaVersion"`
	Data          *apiextensionsv1.JSON `json:"data,omitempty"`
}
```

The exact shared type and CRD placement are not approved by this spike. The type must be designed so the pillar and version are validated before decoding `Data`.

### Layer 3: Typed service projections

SP, KA, RO, WFE, EM, and notification should not all receive the same raw pillar payload.

- SP owns source decoding, deterministic normalization, and pillar enrichment.
- KA receives a typed or validated pillar-specific investigation context projection, such as `ThreatInvestigationContext`, `ComplianceInvestigationContext`, `SupplyChainInvestigationContext`, or `CostOptimizationContext`.
- RO receives the recommendation, policy decision, approval state, and workflow reference needed for orchestration.
- WFE receives the governed action plan and execution parameters, not the original pillar evidence by default.
- EM receives a typed pillar-specific verification plan or a reference to the evidence required for verification.
- Notification receives a redacted operator-facing summary and references to the correlated record.

This prevents every future domain from forcing every downstream CRD to carry every field.

## Validation and Security Rules

- Validate the pillar discriminator and schema version before decoding payload data.
- Reject unknown required versions; do not silently interpret them as the current version.
- Preserve unknown optional fields for forward compatibility, but do not trust them without pillar validation.
- Treat provider payloads, security findings, cost observations, compliance evidence, supply chain records, and optimization recommendations as untrusted input.
- Redact secrets and sensitive data before data reaches the LLM, logs, notifications, or persisted audit payloads.
- Keep raw source evidence at the narrowest retention and access boundary that satisfies reconstruction requirements.
- Do not expose preserved JSON fields to policy expressions unless they are converted into validated, typed policy inputs.
- Record the pillar, schema version, policy version, and decoder result in the audit trail.
- Use fail-closed behavior for malformed payloads, unsupported versions, contradictory evidence, and unavailable validation.

## Service Impact

The first pillar implementation should not update every CRD at once.

- Gateway and RemediationRequest need the smallest common routing addition and a source-contract decision.
- SignalProcessing needs a pillar decoder and a validated pillar-context output.
- AIAnalysis and AgentSession need a projection capable of carrying pillar-specific investigation context and results, plus a pillar-aware policy input.
- Remediation Orchestrator needs policy and approval semantics for each pillar's action classes.
- WorkflowExecution needs only the governed action data required to execute a selected workflow.
- Effectiveness Monitor needs a pillar-specific verification plan and result model if existing alert/hash checks are insufficient.
- Audit and notification need event and redaction mappings, but should not become generic raw-payload stores.

## Follow-up Work

The following items remain before implementation:

- Decide the final `pillar`/`signalClass` wire shape and enumerate initial values.
- Resolve the `ProviderData` string versus `json.RawMessage` documentation and API drift.
- Select `apiextensionsv1.JSON` versus a separate child resource for the first pillar context.
- Define sanitized threat, cost, compliance, and supply chain payload schemas and decoder contracts in the private blueprint repository.
- Define the additive compatibility and eventual deprecation plan for legacy `ProviderData`; no first-pillar migration is assumed.
- Map each candidate field to its owning service and retention boundary.
- Validate policy, approval, denial, and degraded-mode transitions with a representative workflow.
- Define the typed investigation and verification projections.
- Produce the formal BRs, wiring manifest, threat model, and test plan after the product scope is approved.

## Decision Gate

This spike recommends Alternative C, supplemented by Alternative D for large or sensitive evidence. It does not authorize CRD changes.

Before implementation, Product Management and the affected service owners should approve:

- The first source and response journeys.
- The discriminator vocabulary.
- The common envelope and pillar extension boundary.
- The first pillar context schemas and validation owner.
- The projection and reference strategy for downstream services.
- The security, audit, retention, and redaction rules.

After that approval, create a design decision record and formal business requirements. Do not implement a broad unstructured CRD as a shortcut.

## Related Documentation

- [Threat Remediation Product Discovery](../../requirements/enhancements/THREAT_REMEDIATION_PRODUCT_DISCOVERY.md)
- [Cost Optimization roadmap item](../../roadmap/ROADMAP.md)
- [Pillar abstraction requirement](../../services/test-infrastructure/mock-llm/BUSINESS_REQUIREMENTS.md)
- [Signal source and signal type naming](../../architecture/decisions/DD-GATEWAY-010-adapter-naming-convention.md)
- [ADR-075: Multi-Pillar Data Contract Extensibility](../../architecture/decisions/ADR-075-multi-pillar-data-contract-extensibility.md)
- [DD-CONTRACT-003: Pillar Extension Envelope](../../architecture/decisions/DD-CONTRACT-003-pillar-extension-envelope.md)
- [DD-CONTRACT-004: ProviderData Compatibility Boundary](../../architecture/decisions/DD-CONTRACT-004-provider-data-compatibility.md)
- [DD-CONTRACT-005: Multi-Pillar Propagation Boundaries](../../architecture/decisions/DD-CONTRACT-005-multi-pillar-propagation-boundaries.md)
- [DD-CONTRACT-006: Pillar Safety and Audit Governance](../../architecture/decisions/DD-CONTRACT-006-pillar-safety-audit-governance.md)
- [CRD schema reference](../../architecture/CRD_SCHEMAS.md)
- [Rejected pre-investigation pipeline proposal](../../architecture/proposals/PROPOSAL-EXT-007-pre-investigation-pipeline.md)
- [AgentSession API types](../../../api/agentsession/v1alpha1/agentsession_types.go)
- [SignalProcessing API types](../../../api/signalprocessing/v1alpha1/signalprocessing_types.go)
