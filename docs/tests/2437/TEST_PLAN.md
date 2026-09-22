# Test Plan: Demo Model-Aware Reasoning Configuration

**Test Plan Identifier**: TP-2437-v1.0
**Feature**: Configure and document model-aware reasoning for the automated fleet demo
**Version**: 1.0
**Created**: 2026-09-19
**Author**: Kubernaut Team
**Status**: Active
**Branch**: `fix/fleet-e2e-logger-init`

## 1. Purpose and Business Outcome

This plan validates issue #2437 and BR-PLATFORM-014. The automated demo must infer safe
reasoning defaults for first-party OpenAI models, require explicit opt-in for arbitrary
OpenAI-compatible endpoints, and render one consistent profile consumed by API Frontend
and Kubernaut Agent. The final manual E2E run must prove the configured demo completes a
real remediation journey.

## 2. Authority and Scope

- [BR-PLATFORM-014](../../requirements/BR-PLATFORM-014-demo-environment-cert-lifecycle.md)
- [DD-LLM-005](../../architecture/decisions/DD-LLM-005-model-aware-reasoning-support.md)
- Issue #2437
- `hack/setup-demo-infra/main.go`
- `test/infrastructure/demo_helm.go`
- `charts/kubernaut/tests/llm_profiles_test.yaml`

In scope: Make-to-CLI flag wiring, model-family inference, explicit overrides, Helm profile
rendering, AF/KA configuration parity, documentation, and the fleet demo remediation journey.
Out of scope: provider-specific model quality benchmarking and the OpenAI Responses API.

## 3. Pyramid Coverage

| Tier | Test IDs | Proof |
|---|---|---|
| Unit | `UT-INFRA-FLEETDEMO-052` through `UT-INFRA-FLEETDEMO-066` | Resolver, CLI parsing, topology defaults, and validation behavior for GPT 5.6+ version-based defaults across variants, older GPT-5, o1/o3/o4, non-reasoning models, custom endpoints, overrides, and SI-10 invalid input |
| Integration | `IT-PLATFORM-LLM-2437-001`, `IT-PLATFORM-LLM-2437-002` | Helm renders the resolved reasoning block into KA and AF production ConfigMaps |
| E2E | `E2E-PLATFORM-2437-001` | `make setup-fleet-demo-infra` installs the configured fleet demo; a crashloop scenario is investigated and remediated through the real fleet path |

## 4. Wiring Manifest

| Component | Production entry point | Wiring location | Test |
|---|---|---|---|
| Demo reasoning flags | `make setup-fleet-demo-infra` / `make setup-local-demo-infra` | `Makefile` → `hack/setup-demo-infra/main.go` | `UT-INFRA-FLEETDEMO-052` through `061` |
| Reasoning profile rendering | Helm install from `InstallDemoHelmChart` | `test/infrastructure/demo_helm.go` → `global.llmProfiles.primary.reasoning` | `IT-PLATFORM-LLM-2437-001/002` |
| AF/KA reasoning consumption | Deployed AF and KA clients | `charts/kubernaut/templates/*` → runtime ConfigMaps | `E2E-PLATFORM-2437-001` |

## 5. E2E Procedure

1. Run the fleet demo setup with a valid LLM credential and reasoning configuration.
2. Verify the printed cluster, Console, and kubeconfig outputs.
3. Inspect AF and KA runtime ConfigMaps and confirm the reasoning profile is present.
4. Run the `crashloop` scenario with `--fleet --alert-only`.
5. Use Console to investigate the alert, approve the proposed workflow, and verify the
   workload is remediated on the spoke cluster.
6. Record the RemediationRequest, AIAnalysis completion, WorkflowExecution completion,
   and final workload recovery as the E2E evidence.

## 6. Pass Criteria

- All unit and Helm integration scenarios pass.
- `go build ./...`, lint, and project test targets pass.
- The generated Helm documentation drift check passes.
- The fleet demo installs successfully and the crashloop remediation completes.
- No credentials appear in Helm arguments, logs, summaries, or committed files.
