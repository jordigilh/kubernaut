# Spike: Goose CLI -> ACP MCP Roundtrip

**Date**: May 2026
**Status**: PASS — roundtrip mechanics validated, but the CRD-integration model it fed into is ⚠️ SUPERSEDED by [#1536](https://github.com/jordigilh/kubernaut/issues/1536)
**Proposal**: [PROPOSAL-EXT-004](../../docs/architecture/proposals/PROPOSAL-EXT-004-goose-recipes.md) (superseded)

## Purpose

Validate end-to-end Goose CLI integration with:
- `gcp_vertex_ai` provider (Anthropic Claude via Vertex AI)
- Streamable HTTP MCP server (mock Kubernaut tools)
- Tool call dispatch and structured result extraction

## Prerequisites

- Goose CLI (`block-goose-cli` via Homebrew, v1.35.0+)
- Python 3.10+ with `fastmcp` package
- GCP ADC credentials configured (`gcloud auth application-default login`)
- `GCP_PROJECT_ID` and `GCP_LOCATION` environment variables

## Running

### 1. Start the mock MCP server

```bash
pip install fastmcp
python mcp_server.py
```

Server starts on `http://127.0.0.1:9876/mcp` with tools:
- `kubectl_get` - Returns mock Deployment status
- `kubectl_list_events` - Returns mock OOMKill events
- `submit_result` - Accepts structured RCA result

### 2. Run Goose with MCP tools

```bash
GCP_PROJECT_ID=<your-project> \
GCP_LOCATION=us-east5 \
GOOSE_PROVIDER=gcp_vertex_ai \
GOOSE_MODEL=claude-sonnet-4@20250514 \
goose run \
  --no-profile \
  --no-session \
  --quiet \
  --output-format json \
  --max-turns 4 \
  --with-streamable-http-extension "http://127.0.0.1:9876/mcp" \
  --text "Call kubectl_get with kind=Deployment, name=web-app, namespace=production. Then call submit_result with root_cause='OOMKilled: memory limit exceeded', confidence=0.95, affected_resources=['production/deployment/web-app'], remediation_suggested=true."
```

## Results

| Aspect | Result |
|---|---|
| Provider | `gcp_vertex_ai` with ADC auth |
| Model | `claude-sonnet-4@20250514` via Vertex AI |
| MCP transport | streamable-http |
| Tool discovery | All 3 tools discovered |
| kubectl_get | Called with correct args; response processed |
| submit_result | Called with root_cause, confidence, affected_resources, remediation_suggested |
| Token usage | 2,911 (2,494 input + 417 output) |
| Execution time | 6.7 seconds |
| JSON output | Parseable structured messages |

## Key Findings

1. **Provider name**: `gcp_vertex_ai` (not `google-vertex-ai` or `gcp-vertex`)
2. **Model format**: `claude-sonnet-4@20250514` (with `@`, not `-`)
3. **Authentication**: GCP ADC, no API keys needed
4. **Tool naming**: Goose prefixes tool names with MCP server ID (e.g., `127_0_0_1_9876_mcp__kubectl_get`)
5. **JSON output**: `--output-format json` produces full message history with tool calls/responses

## ACP Server Implications

- Start Goose as subprocess with `--with-streamable-http-extension` pointing to ACP MCP proxy
- ACP proxy intercepts tool calls for budget/audit before forwarding to KA
- `GCP_PROJECT_ID` and `GCP_LOCATION` injected from K8s Secret
- `--max-turns` maps to investigation timeout policy
