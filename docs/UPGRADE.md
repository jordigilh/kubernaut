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
