# Test Plan: Fleet MCP List Responses and Post-RCA Label Persistence

**Test Plan Identifier**: TP-FLEET-MCP-POST-RCA-v1
**Feature**: Parse remote MCP list responses and persist detected labels on every AgentSession result path.
**Version**: 1.0
**Status**: Active
**Business Requirements**: BR-INTEGRATION-1489, BR-AI-056
**Architecture References**: ADR-056, ADR-068

## 1. Purpose and Success Criteria

This plan verifies that remote HPA/PDB discovery works with the response shapes emitted by the MCP Gateway and that detected labels are persisted before every AIAnalysis terminal or continuing transition.

The plan passes only when all P0 scenarios pass, all affected existing tests pass, the production wiring is exercised, and the focused fleet E2E proves the complete remote investigation journey.

## 2. Pyramid Scenarios

### Unit

- `UT-KA-MCP-LIST-001`: Parse a top-level YAML sequence into unstructured objects.
- `UT-KA-MCP-LIST-002`: Parse a top-level JSON array into unstructured objects.
- `UT-KA-MCP-LIST-003`: Parse an `{items: [...]}` envelope.
- `UT-KA-MCP-LIST-004`: Preserve an empty list.
- `UT-KA-MCP-LIST-005`: Reject malformed, scalar, and non-array list responses.
- `UT-AA-056-020`: Persist labels for problem-resolved responses.
- `UT-AA-056-021`: Persist labels for not-actionable responses.
- Existing `UT-AA-056-003`, `UT-AA-056-005`, `UT-AA-056-006`, `UT-AA-056-007`, and `UT-AA-056-008`: cover normal-path mapping, immutability timestamp, absent labels, failed detections, and malformed labels.

### Integration

- `IT-KA-MCP-LIST-001`: Exercise `overlayClientReader.List()` through the production reader with a top-level YAML response.
- Existing overlay-reader tests: prove GVK, namespace, selector, and tool error wiring.
- Existing MCP client tests: prove OAuth2 scope and cluster-prefixed routing.
- Existing AIAnalysis response tests: prove production response processing and Rego consumption of `PostRCAContext`.

### End-to-End

- `E2E-FLEET-017-000`: After the marker Deployment is Available, verify the same resource is readable through the authenticated `resources_get` MCP path before posting the alert; this prevents a false batch-parse failure caused by remote discovery convergence.
- `E2E-FLEET-017-001`: Create remote HPA/PDB resources and retrieve them through the real MCP Gateway.
- `E2E-FLEET-017-002`: Assert `hpaEnabled=true` and `pdbProtected=true`.
- `E2E-FLEET-017-003`: Assert persisted `PostRCAContext.DetectedLabels`.
- `E2E-FLEET-017-004`: Assert remote-cluster provenance and no wrong-cluster data.
- `E2E-FLEET-017-005`: Assert remediation correlation and RCA reconstruction.

## 3. Control Objective Traceability

- FedRAMP `AC-4` and `SC-7`: IT/E2E verify cluster-prefixed reads use the intended remote boundary.
- FedRAMP `AC-6`: existing negative MCP tests verify read-only identities cannot write.
- FedRAMP `IA-5` and `SC-8`: existing OAuth2 and TLS tests verify authenticated transport.
- FedRAMP `AU-3`: AIAnalysis tests verify detected labels and remediation context are persisted.
- FedRAMP `SI-4`: parser and E2E failure assertions verify errors remain observable.
- FedRAMP `SI-10`: parser tests verify malformed responses fail closed.
- OWASP ASVS `V2` and `V4`: existing authentication and authorization tests.
- OWASP ASVS `V5`: list parser malformed-input tests.
- OWASP ASVS `V7`: contextual parser and response-processing errors.
- OWASP ASVS `V9`: existing MCP TLS transport tests.
- OWASP ASVS `V11`: detected labels affect the AIAnalysis business result.
- OWASP ASVS `V13`: MCP list protocol and response-shape tests.

## 4. Pyramid and Wiring Gates

- Unit tests prove parser and response-processing logic.
- Integration tests prove production overlay-reader and response-processor dispatch.
- E2E proves the complete remote alert-to-analysis journey.
- Every new production function has a non-test caller.
- No scenario is skipped or pending.
- Any failed control-objective scenario blocks completion.
