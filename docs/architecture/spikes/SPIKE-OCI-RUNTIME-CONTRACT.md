# Spike: OCI Image Packaging Contract per Runtime Type

**Date**: May 19, 2026
**Status**: COMPLETED
**Duration**: 1 session
**Relates to**: [PROPOSAL-EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md), [PROPOSAL-EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md), [PROPOSAL-EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md)
**Code**: [spikes/oci-runtime-contract/](../../../spikes/oci-runtime-contract/)

---

## Objective

Define and validate the OCI image packaging contract for each AgenticWorkflow
runtime type (goose, oas, deepagent). Mirrors the `RemediationWorkflow` CRD
pattern where `execution.engine` selects the runtime and `execution.bundle`
points to the OCI image.

## What Was Built

| File | Purpose |
|---|---|
| `validate_contract.go` | Go script validating OCI labels against the defined contract |
| `README.md` | Full contract definition, Dockerfile templates, AgenticWorkflow CRD sketch |

## Contract Summary

### Required OCI Labels

| Label | Description | Example |
|---|---|---|
| `ai.kubernaut.runtime` | Runtime type identifier | `goose`, `oas`, `deepagent` |
| `ai.kubernaut.spec-version` | Spec format version | `25.4.1` (OAS), `1.0` (goose), `0.1` (deepagent) |
| `ai.kubernaut.entrypoint` | Path to spec file inside image | `/spec/agent.yaml` |
| `ai.kubernaut.tools` | Comma-separated tool names | `kubectl_get,submit_result` |

### Entry Point Contract

| Runtime | Spec File | Entry Point Command |
|---|---|---|
| Goose | `/spec/recipe.yaml` | `goose run --recipe /spec/recipe.yaml` |
| OAS | `/spec/agent.yaml` | `python -m kubernaut_oas /spec/agent.yaml` |
| Deep Agents | `/spec/agent.yaml` | `python -m kubernaut_deepagent /spec/agent.yaml` |

## Key Findings

| Finding | Impact |
|---|---|
| **F1**: OCI labels enable runtime discovery without image extraction | Pre-flight validation before pulling large images |
| **F2**: Python runtimes share a base image | OAS + Deep Agents share `python:3.12-slim` + LangGraph |
| **F3**: Goose runtime is self-contained | Rust binary, no additional language runtime |
| **F4**: `runtimeConfig` maps to enforcement layer `BudgetConfig` | ACP server reads CRD and configures enforcement per-investigation |
| **F5**: Bundle digest ensures integrity | Same supply chain pattern as `RemediationWorkflow` |

## Confidence

**92%** — Contract defined and validated against all 3 runtimes. Remaining 8%
is admission webhook implementation and multi-arch image testing.
