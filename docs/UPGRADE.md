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

### Cluster-scoped targets authorize through their own label

BR-SCOPE-001 FR-SCOPE-004 Option A is restored (issue #2670). A real
cluster-scoped remediation target — a `Node`, a `PersistentVolume`, a labeled
cluster-scoped CRD, or a `Namespace` object — is authorized by the observed
`kubernaut.ai/managed` label on the object itself, resolving `verified_managed`
or `verified_unmanaged` with `resource-label` provenance. The 2-level hierarchy
degenerates to level 1: no namespace exists to inherit, so a present-but-
unlabeled target is `verified_unmanaged` with `none` provenance, and a target
that cannot be observed is `unavailable` (`observation_missing` locally,
`cache_miss` on the fenced fleet lane, `search_observation_unavailable` on direct
ACM). An invalid label value is `invalid`, exactly as for a
namespaced target: since [#2686](https://github.com/jordigilh/kubernaut/issues/2686)
the fenced syncer stages and commits the invalid observation, so the previously
committed authorization is revoked at the next committed repair rather than
serving until TTL. ADR-053 §"Recognized kinds" records the rule.

Supported targets differ by lane, so choose the lane accordingly. The local
Gateway/RO lane and direct ACM can authorize any kind they can read. The fenced
Fleet Metadata Cache observes only `Deployment`, `StatefulSet`, `DaemonSet`,
`Pod`, `Service`, `Node` and `Namespace`: a `PersistentVolume` or a
cluster-scoped CRD target can never resolve as managed there and ends in
retryable `unavailable` (`cache_miss`). A read denied by RBAC is reported as
terminal `unauthorized` (`scope_access_denied`) by a direct client such as the
API Frontend's, whereas a cached Gateway/RO lookup that cannot read the kind ends
in retryable `unavailable` (`scope_lookup_error`).

`ScopeUnsupported` with reason `cluster_scoped_target_unsupported` is now
reserved for a genuinely non-labelable target: `Kind: ClusterScope`, the
reserved pseudo-kind for a cluster-level intent with no backing object to carry
a label (DD-GATEWAY-CLUSTER-SCOPE-SIGNALS). That design remains deferred.

This reverses the behavior shipped in #2637/#2662, where both scope lanes
refused every cluster-scoped target and hub `Node`/`PersistentVolume`
remediation that had worked through a label stopped resolving as managed. The
reason string never crosses the Fleet wire: `ScopeResult` carries only `state`
and `provenance` (DD-FLEET-015), so a Fleet consumer sees the bare state. For a
`ClusterScope` request that specific qualifier still matters: an identity with no
group and no version fails GVK validation before the boundary check and returns
`ScopeInvalid` from the federated checker and the FMC HTTP handler, while one
that carries a version reaches the pseudo-target guard and yields bare
`ScopeUnsupported`.

Read access is unchanged and read-only: the scope check performs a metadata-only
read (a `Get` on `PartialObjectMetadata`, a cache read, or an ACM Search query)
and never mutates the target. ADR-053 §"Recognized kinds" lists every kind the
static classification recognizes and the reads each lane holds.

**Cluster-scoped targets carry no namespace.** At the Gateway, an alert that
labels a namespace for a Node, PV or other cluster-scoped kind is normalized to
an empty target namespace, so the dedup key (which hashes the target namespace),
the persisted `spec.targetResource` and the scope identity agree. This holds for
the kinds the Gateway can classify — the static set plus kinds its API discovery
reports as cluster-scoped. For cluster-scoped kinds it supersedes #1371's "honor
an explicit namespace label" behaviour. The API Frontend still hashes the
namespace it is given, so a cluster-scoped CRD submitted through AF with a
namespace keeps it.

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

## Invalid managed-label observations now revoke (#2686)

On the fenced fleet lane, a target whose `kubernaut.ai/managed` label value becomes
invalid (anything other than `"true"`/`"false"`) used to keep its previously committed
authorization until a later repair replaced the record or its TTL expired (45s default),
because the FMC syncer rejected the invalid observation and failed the whole sync cycle.
The syncer now stages and commits the invalid observation like any other label state
(DD-FLEET-016 "Invalid-label observations" amendment), and the already-shipped read-side
mapping turns it into `ScopeInvalid` (`invalid_resource_label`) immediately — the same
fail-closed rule the local lane applies. Readiness and partition failures are unchanged:
they keep returning `unavailable` (`partition_unavailable`/`partition_invalid`) and are
never flattened into `invalid`. A structurally malformed source payload still fails the
scan as incomplete; the scan commits no observation, so the committed record and its
clean proof remain readable until the next committed repair or their TTL expires.

### `ScopeResult.Managed` is removed

The field was written by the local manager and the federated checker but read
nowhere. There is **no wire change**: the FMC scope-result decoder already
admits only `state` and `provenance` (DD-FLEET-015), so a future writer of
`managed` would have failed every scope check with a strict-decode error.
Consumers must read `State` (`ScopeVerifiedManaged` / `ScopeVerifiedUnmanaged`)
and `Provenance`.
