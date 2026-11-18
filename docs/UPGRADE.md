# Kubernaut Upgrade Guides

Versioned upgrade guides covering breaking changes and required migration steps.

| From | To | Guide |
|------|----|-------|
| v1.4 | v1.5 | [UPGRADE_V1.4_TO_V1.5.md](development/release/UPGRADE_V1.4_TO_V1.5.md) |

## RCA confidence rollout (#2586)

Before deploying a build that writes independent RCA confidence, install the CRDs
from that same release **before rolling out its writers or controllers**. This
applies even when backward compatibility is not required: the older AgentSession
schema does not declare `status.result.rcaConfidence`, so the Kubernetes API
server can prune the field on write. Updating the CRD afterward does not recover
values already pruned.

1. Obtain the matching release's AgentSession, AIAnalysis, and
   RemediationApprovalRequest CRDs from `config/crd/bases/` (or the matching
   operator release's embedded operand CRDs). Install them using the supported
   procedure for your deployment and CRD ownership.
2. Before updating any writers or controllers, verify all three installed CRDs
   are established and their served-version schemas contain the RCA confidence
   fields from the matching release. In particular, inspect AgentSession's
   `status.result.rcaConfidence`, both AIAnalysis `rcaConfidence` fields, and
   RemediationApprovalRequest's `spec.rcaConfidence`. Compare the installed
   schemas with the release manifests; an established CRD alone does not prove
   its schema is current. **Stop the rollout** if any schema is missing or the
   CRD update fails. Do not assume operator reconciliation has already updated
   the CRDs on every upgrade path.
3. Roll out the matching writers and controllers only after that verification.
   Investigate any data written during an earlier old-schema window separately;
   the CRD update cannot reconstruct a lost score.

### Operator approval policy

For workflows that reach approval evaluation, user-provided Rego decides
whether low or missing RCA confidence requires approval in autonomous and
interactive flows. Existing terminal/failure routing remains separate. No
default deployed policy is assumed: a permissive policy can allow execution
with a low or missing score. Operators who require review must configure and
test their own policy for both cases, including low RCA confidence paired with
high workflow confidence. `input.rca_confidence` is absent when the score is
missing; an explicit zero is present and distinct. `input.workflow_confidence`
is independent. `input.confidence_threshold` is optional: it is absent when
`rego.confidenceThreshold` is not configured, so policy must handle that case.

The existing `UT-AA-2586-003` cases in
[`pkg/aianalysis/rego_evaluator_test.go`](../pkg/aianalysis/rego_evaluator_test.go)
provide tested policy examples: RCA 0.65 with workflow 0.95 and threshold 0.8
requires review under the sample rule; a separate rule requires review when the
score is missing; a permissive rule allows a missing score. Adapt and test these
examples as part of the operator's own policy rather than treating them as a
deployed default.

## Fleet scope lanes (#2660)

Three scope-surface changes land together. Two remove lanes that had no
reader; the third — cluster-scoped targets — is a behavior change already
approved as the DD-FLEET-015 deferred-target boundary (see below).

### Cluster-scoped targets remain unsupported

BR-SCOPE-001 FR-SCOPE-004 (approved DD-FLEET-015 amendment): a
cluster-scoped remediation target (`Namespace == ""` or a known cluster-scoped
kind such as `Node`/`PersistentVolume`) returns `ScopeUnsupported` with reason
`cluster_scoped_target_unsupported` on both the local and the fleet scope
lanes. Hub `Node`/`PersistentVolume` remediation that previously worked through
a label no longer resolves as managed. This is a documented behavior change, not
a regression: the target boundary is deferred until fleet cluster-scoped support
is designed.

### Both `IsManagedResource` boolean lanes are removed

`scope.Manager.IsManagedResource` and `fleet.FederatedScopeChecker.IsManagedResource`
are deleted. They had no production callers and collapsed the typed outcome
(`invalid`, `unsupported`, `unavailable`) into `false`, which is exactly the
ambiguity DD-FLEET-015 removes. Use `Check` and branch on `ScopeState`.
`fleet.NewFederatedScopeChecker` no longer accepts a local checker (the dead
`local` field is gone with it). The factory selects the local checker only when
Fleet is **disabled**; with Fleet enabled, every scope request must carry a
`ClusterID` — an unattributed request returns `ScopeInvalid`
(`missing_cluster_attribution`) and never falls back to the local checker
(DD-FLEET-015).

### `ScopeResult.Managed` is removed

The field was written by the local manager and the federated checker but read
nowhere. There is **no wire change**: the FMC scope-result decoder already
admits only `state` and `provenance` (DD-FLEET-015), so a future writer of
`managed` would have failed every scope check with a strict-decode error.
Consumers must read `State` (`ScopeVerifiedManaged` / `ScopeVerifiedUnmanaged`)
and `Provenance`.
