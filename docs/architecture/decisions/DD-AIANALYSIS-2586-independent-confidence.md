# DD-AIANALYSIS-2586: Independent RCA confidence in approval policy

## Decision

For BR-AI-012 and BR-AI-028, preserve RCA confidence independently from workflow-selection confidence through the AgentSession and AIAnalysis boundaries. An absent RCA score is unknown, not zero. Pass the scores and the existing operator-configured confidence threshold to user-provided Rego. The policy exclusively decides approval for selected workflows in both interactive and autonomous flows; it may permit a missing score. The existing workflow-selection floor and non-workflow terminal routing are unchanged. No default deployed policy is assumed.

Approval review and audit records distinguish the scores and preserve presence. The policy evaluation audit describes the evaluator result, including degraded evaluation, and the approval decision audit describes the resulting decision. Console presentation is tracked by kubernaut-console #180.

KA's durable `aiagent.rca.complete` and `aiagent.response.complete` events carry optional `response_data.rca_confidence` independently of workflow selection confidence. The legacy required `confidence` is retained for compatibility and must not be used to infer RCA presence. These events support AU-3 and SOC2 CC7.2 reconstruction (DD-AUDIT-003, DD-AUDIT-005).

The Rego input includes numeric `workflow_confidence`, optional numeric `rca_confidence`, and optional `confidence_threshold`. An absent RCA key means unknown; a value of zero is present. The threshold comes from the existing `rego.confidenceThreshold` operator configuration when set. The legacy `confidence` key also denotes workflow confidence. User policy is responsible for deciding whether and how these inputs affect approval, including when a score or threshold is absent.

## Alternatives

* Controller-imposed RCA guard: rejected because it overrides the user's policy decision.
* Mandatory bundled RCA policy: rejected because policies are user supplied and no default deployment is guaranteed.
* Policy-owned decision with explicit inputs: selected because it preserves operator control and makes uncertainty available without silently changing routing.

## Control objectives

FedRAMP AU-3 requires meaningful decision records; SI-10 requires preservation and validation of the bounded input contract. SOC2 CC7.2 requires correlated investigation and decision reconstruction. OWASP ASVS input validation and authorization objectives are addressed by typed boundaries and policy-owned authorization; control assessment requires the scoped integration and end-to-end evidence, not this document alone.
