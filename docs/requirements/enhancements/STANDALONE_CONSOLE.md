# Enhancement Proposal: Standalone Kubernaut Console

**Status**: Proposed; not an implementation approval or release commitment
**Updated**: 2026-10-24
**Tracking**: [#855](https://github.com/jordigilh/kubernaut/issues/855)

## Purpose and Agreed Direction

Evolve the existing standalone console into a fleet-scale operational application
with persistent navigation, global context, dashboards, and list/detail views.
Build on Kubernaut's existing component stack and operational model. RHDH and
Backstage integration are no longer in scope.

Reuse the existing dynamic interactive console: streaming investigation, RCA,
workflow selection, approval, execution progress, and verification. The application
shell should make that experience accessible within a broader operational workspace,
not replace it with a second chat implementation.

Natural-language shortcuts and ordinary navigation, tables, and filters are
complementary. Basic navigation must not require a generative LLM call.

## Proposed Application Views

| View | Intended responsibility |
|---|---|
| Fleet Overview | Scoped metrics, cluster health, and prioritized alert/investigation queues with explicit freshness and unavailable-data states |
| Investigations | Active/historical cases, evidence, RCA, workflow/approval/verification state, and the existing interactive workspace |
| Workflows | Browsable, filterable catalog and execution history |
| Clusters | Per-cluster health, investigations, and capacity supported by authoritative data |
| Policies and Audit | Effective approval policy, applicable rule information, and attributable lifecycle reconstruction |
| Administration | Standalone settings with explicit ownership and read-versus-edit boundaries |

The earlier Fleet Overview, Investigation, and Workflows presentation designs are
historical product references. Their RHDH wrapper is obsolete. No presentation
assets, branding, or private blueprint material are copied into this proposal.

## Presentation and Interaction

1. Provide a consistent sidebar/header, spacing, metric panels, and list/detail
   composition using the existing console component stack.
2. Retain explicit cluster/resource context and permission-scoped, paginated data.
   Preserve the 100+ cluster design target; illustrative mockup counts are not SLAs.
3. Surface work that needs attention: pending review, incomplete verification,
   escalation, and unavailable telemetry. Unknown is not zero or healthy.
4. Reuse existing evidence-bearing approval cards and verification components.
   Workflow completion, verified outcome, and resolution must remain distinguishable.
5. Administration does not imply browser ownership of every setting. Define which
   settings are console-managed, platform-managed, or deployment/GitOps-managed
   before exposing write controls; backend authorization and audit remain authoritative.

## Read-Only Natural-Language Routing

The companion [ModernBERT intent-routing proposal](CONSOLE_INTENT_ROUTING.md)
covers the second UI enhancement: bounded, non-generative recognition for
navigation and filtering. Fine-tune a pretrained ModernBERT encoder to propose
intent categories; extract and validate parameters before dispatching predefined
view/read handlers rather than generating UI or commands.

Evaluate the proposed ModernBERT recognizer against rules/entity matching and a
smaller encoder/classifier baseline. The checkpoint, training data, serving
runtime, deployment footprint, and measured quality require validation before
implementation/deployment approval. No inference SLA or model accuracy is
promised here. Investigation reasoning remains in the existing interactive flow.

## Acceptance Criteria

### Standalone Experience

- [ ] Standalone application navigation replaces host-product/plugin assumptions.
- [ ] Fleet Overview exposes scoped metrics, cluster health, and actionable lists
  with loading, stale, and unavailable states.
- [ ] Supported views have accessible direct navigation, structured filters,
  responsive layouts, and representative 100+ cluster validation.
- [ ] Existing dynamic investigation/approval/verification components are reused
  without changing their backend safety semantics.
- [ ] Administration/settings ownership is defined before write controls are exposed.

### Routing and Implementation Gates

- [ ] Initial NL dispatch is read-only navigation/filtering, with validated
  context-sensitive follow-ups and ordinary-navigation fallback.
- [ ] The proposed ModernBERT recognizer is evaluated against baselines for joint
  intent/parameter/target quality before implementation or deployment approval.
- [ ] Ambiguity, negation, forged scope, unsupported requests, and model failure
  have proving safety and interaction tests.
- [ ] Classification cannot create investigations, submit approvals, execute or
  roll back workflows, or change settings.
- [ ] Before implementation, approve affected BR mapping, API/security design,
  test plan, and wiring manifest. This document does not authorize implementation.

## References

- [Console intent-routing enhancement #856](https://github.com/jordigilh/kubernaut/issues/856)
- [Operations-platform vision #818](https://github.com/jordigilh/kubernaut/issues/818)
- [Historical DD-CONSOLE-001](../../architecture/decisions/DD-CONSOLE-001-intent-driven-navigation-architecture.md)
