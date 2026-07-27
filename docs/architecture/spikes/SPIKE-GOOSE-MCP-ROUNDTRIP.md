# Spike: Goose CLI -> ACP MCP Roundtrip

**Date**: May 23, 2026
**Status**: COMPLETED — roundtrip mechanics validated (Goose CLI + MCP works), but the CRD-integration model it fed into ([PROPOSAL-EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md)) is ⚠️ SUPERSEDED by [#1536](https://github.com/jordigilh/kubernaut/issues/1536)
**Duration**: 1 session
**Relates to**: [PROPOSAL-EXT-004](../proposals/PROPOSAL-EXT-004-goose-recipes.md)
**Code**: [goose-mcp-roundtrip/](goose-mcp-roundtrip/)

---

## Objective

Validate end-to-end Goose CLI integration with GCP Vertex AI (`gcp_vertex_ai`
provider) and a streamable HTTP MCP server exposing mock Kubernaut tools.

## What Was Built

| File | Purpose |
|---|---|
| `mcp_server.py` | FastMCP server exposing `kubectl_get`, `kubectl_list_events`, `submit_result` |

## Test Results

| # | Test | Result |
|---|---|---|
| 1 | Goose CLI connects to MCP server | PASS |
| 2 | Tool discovery (all 3 tools found) | PASS |
| 3 | `kubectl_get` called with correct args | PASS |
| 4 | `submit_result` called with structured RCA | PASS |
| 5 | JSON output is parseable | PASS |

## Execution Metrics

| Metric | Value |
|---|---|
| Provider | `gcp_vertex_ai` (GOOSE_PROVIDER) |
| Model | `claude-sonnet-4@20250514` |
| Authentication | GCP ADC (Application Default Credentials) |
| Token usage | 2,911 (2,494 input + 417 output) |
| Execution time | 6.7 seconds |
| Tool calls | 2 (kubectl_get + submit_result) |

## Key Findings

| Finding | Impact |
|---|---|
| **F1**: Provider name is `gcp_vertex_ai` | Not `google-vertex-ai` or `gcp-vertex` |
| **F2**: Model format uses `@` separator | `claude-sonnet-4@20250514`, not `claude-sonnet-4-20250514` |
| **F3**: GCP ADC auth works transparently | No API keys needed; only `GCP_PROJECT_ID` + `GCP_LOCATION` env vars |
| **F4**: Goose prefixes tool names with MCP server ID | e.g., `127_0_0_1_9876_mcp__kubectl_get` |
| **F5**: `--output-format json` produces full message history | Parseable tool calls and responses in structured JSON |
| **F6**: `--max-turns` controls investigation length | Maps to investigation timeout policy in ACP server |

## Environment

| Dependency | Version |
|---|---|
| Goose CLI | v1.35.0 (`block-goose-cli` via Homebrew) |
| Python | 3.14.3 |
| FastMCP | latest |
| GCP Vertex AI | us-east5 |

## Confidence

**96%** — End-to-end roundtrip validated with real Vertex AI. Remaining 4% is
MCP session lifecycle management (reconnection, timeout, graceful shutdown).
