# Test Plan: #2582 schema foundation

## Scope and business requirements

Install the schema required by the fail-closed lifecycle before deploying its
controllers. The foundation adds no controller behavior. BR-AI-009 requires
the persisted AgentSession recovery identity; BR-WE-014 and BR-ORCH-026/036
require the lifecycle status vocabulary. SI-10 requires invalid status values
to remain rejected. Backward compatibility is not a supported requirement.

## Scenarios and acceptance

| ID | Critical | Outcome | Tier |
|---|---|---|---|
| UT-SCHEMA-2582-001 | Yes | Embedded AIAnalysis schema includes the recovery identity. | Unit |
| UT-SCHEMA-2582-002 | Yes | Embedded lifecycle schemas expose the new cancellation/failure values. | Unit |
| IT-SCHEMA-2582-001 | Yes | The API server retains an AgentSession recovery reference in AIAnalysis status. | Integration |
| IT-SCHEMA-2582-002 | Yes | The API server accepts the new lifecycle values and rejects invalid values. | Integration |

All four critical scenarios must pass. Unit scenario coverage is measured as
passing meaningful unit scenarios over the two unit scenarios above; the target
is 2/2. Structural coverage is not used as a substitute.

## Wiring and rollout

| Component | Production entry point | Verification |
|---|---|---|
| Embedded CRDs | Operator `internal/resources.EnsureCRDs` reads `assets.CRDsFS` | Embedded schema tests and real API-server admission/persistence tests |

The operator dependency bump and deployed-operator proof belong to operator
issue #600. Install updated CRDs before deploying the lifecycle controllers;
do not claim cancellation behavior until #2583 is integrated. Run integration
tests through the repository Make targets. No new audit event is introduced.
