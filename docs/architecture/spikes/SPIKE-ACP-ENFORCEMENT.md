# Spike: ACP Server Enforcement Layer

**Date**: May 19, 2026
**Status**: COMPLETED
**Duration**: 1 session
**Relates to**: [PROPOSAL-EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md), [PROPOSAL-EXT-005](../proposals/PROPOSAL-EXT-005-oracle-agent-spec.md), [PROPOSAL-EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md)
**Code**: [spikes/acp-enforcement/](../../../spikes/acp-enforcement/)

---

## Objective

Prototype the universal ACP server enforcement layer that intercepts tool
calls from any runtime (Goose, OAS/LangGraph, Deep Agents) to enforce
feature parity with KA v1.5: tool call budgets, shadow agent feed, and
audit event emission.

## What Was Built

| File | Purpose |
|---|---|
| `enforcement.go` | Core enforcement layer: budget checking, shadow feed, audit emission |
| `enforcement_test.go` | 9-test suite validating all interception behaviors |
| `go.mod` | Module definition |

## Test Results

| # | Test | Result |
|---|---|---|
| 1 | Tool calls within budget succeed | PASS |
| 2 | Per-tool limit triggers rejection | PASS |
| 3 | Total budget triggers rejection | PASS |
| 4 | Exempt tools bypass budgets (e.g., `todo_*`) | PASS |
| 5 | Audit events emitted for all calls | PASS |
| 6 | Reset clears counters between phases | PASS |
| 7 | Shadow feed receives results in order | PASS |
| 8 | Failing tools record audit failure events | PASS |
| 9 | WrapForRuntime returns all registered handlers | PASS |

## Key Findings

| Finding | Impact |
|---|---|
| **F1**: Decorator pattern scales to all runtimes | `map[string]ToolHandler` is universal across Goose MCP, OAS tool_registry, LangGraph tools |
| **F2**: Budget config is portable from KA | `BudgetConfig` mirrors `AnomalyConfig` exactly |
| **F3**: Shadow feed is pluggable | Decoupled from shadow agent implementation (local canary or remote KA) |
| **F4**: Audit events use existing schema | New `aiagent.runtime.*` prefix distinguishes runtime-mediated calls |

## Integration Points

| KA v1.5 Component | ACP Equivalent | Integration |
|---|---|---|
| `AnomalyDetector` | `EnforcementLayer.checkBudget()` | Same thresholds, same semantics |
| `alignment.SubmitToolStep` | `ShadowFeed` callback | ACP runs shadow evaluator or delegates to KA |
| `audit.StoreBestEffort` | `AuditSink` callback | Routes to Data Storage via OAS client |
| `registry.ToolRegistry` | `WrapForRuntime()` | Returns `map[string]ToolHandler` for runtime injection |

## Confidence

**95%** — All 9 tests pass. Remaining 5% is production hardening (concurrent
access under load, graceful degradation when audit sink is unavailable).
