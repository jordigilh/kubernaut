# Spike: Deep Agents Library Validation

**Date**: May 23, 2026
**Status**: COMPLETED — technique validated; the CRD-level runtime-selection model it targeted is superseded by [#1536](https://github.com/jordigilh/kubernaut/issues/1536)
**Duration**: 1 session
**Relates to**: [PROPOSAL-EXT-006](../proposals/PROPOSAL-EXT-006-deep-agents.md)
**Code**: [pyagentspec-langgraph/05_deepagents_validation.py](pyagentspec-langgraph/05_deepagents_validation.py)

---

## Objective

Validate that LangChain Deep Agents (`create_deep_agent`) supports sub-agent
delegation with scoped tool sets, budget tracking via handler wrappers, and
structured RCA output -- all using real Vertex AI.

## What Was Built

| File | Purpose |
|---|---|
| `05_deepagents_validation.py` | Two tests: sub-agent delegation + budget tracking |

## Test Results

| # | Test | Result |
|---|---|---|
| 1 | Sub-agent delegation (coordinator + k8s-investigator + metrics-investigator) | PASS |
| 2 | `submit_result` called with structured RCA from coordinator | PASS |
| 3 | Budget tracking (per-tool and total counts) | PASS |
| 4 | Tool scoping (sub-agents only see their assigned tools) | PASS |

## Test 1: Sub-agent Delegation

The coordinator agent (`rca-coordinator`) delegated to two specialists:

| Agent | Tools | Behavior |
|---|---|---|
| Coordinator | `submit_result` | Delegated via `task()`, synthesized findings, submitted RCA |
| k8s-investigator | `kubectl_get`, `kubectl_list_events` | Investigated deployment status and events |
| metrics-investigator | `prometheus_query` | Analyzed memory utilization patterns |

The coordinator produced 7 messages at the top level. Sub-agent messages are
internal to the delegation. The coordinator called `submit_result` with a
structured root cause analysis combining both specialists' findings.

## Test 2: Budget Tracking

| Metric | Value |
|---|---|
| Total tool calls | 3 |
| kubectl_get | 1 |
| kubectl_list_events | 1 |
| submit_result | 1 |

Budget tracking works via simple handler wrapping -- the same pattern used
by the ACP enforcement layer (Spike 2).

## Key Findings

| Finding | Impact |
|---|---|
| **F1**: `SubAgent` accepts scoped tool lists | Natural budget isolation per specialist |
| **F2**: Sub-agents inherit model but get isolated tool sets | Coordinator can use different model than specialists |
| **F3**: Tool handlers are sync callables | Budget/audit wrapping is trivial |
| **F4**: `task()` tool carries delegation context | Shadow agent can inspect sub-agent task descriptions |
| **F5**: Coordinator synthesizes sub-agent findings | Natural RCA aggregation point |
| **F6**: ~100 seconds for complex investigation | Budget timeout enforcement is critical for SLO compliance |

## Environment

| Dependency | Version |
|---|---|
| deepagents | 0.6.3 |
| langgraph | 0.6.11 |
| langchain-anthropic | latest |
| anthropic | latest (with vertex extras) |
| Model | claude-sonnet-4@20250514 via Vertex AI |

## Confidence

**96%** — Sub-agent delegation, tool scoping, and budget tracking all validated
with real Vertex AI. Remaining 4% is production hardening (sub-agent timeout,
error propagation from specialists to coordinator, concurrent budget tracking).
