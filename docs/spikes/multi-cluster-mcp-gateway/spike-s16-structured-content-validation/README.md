# Spike S16 — structuredContent Validation (kube-mcp-server PR #1232)

## Goal

Validate that kube-mcp-server now returns `structuredContent` in `CallToolResult`
for `resources_get` and `resources_list`, following the merge of upstream PR #1232.
Determine if the text parsing fallback (`parseMultiFormat`, `parseTableText`,
`looksLikeTable`) can be eliminated.

Supersedes S15's finding that StructuredContent was nil (tracked in kube-mcp-server
Issue #920).

## Context

- kube-mcp-server PR #1232 (merged 2026-06-29) adds `structuredContent` to both
  `resources_get` and `resources_list` via `PrintObjStructured` + `NewToolCallResultFull`
- Kubernaut's `mcpclient` already has `extractStructured()` as a Priority 1 check
  before falling back to text parsing (implemented during S15)
- The MCP Go SDK v1.6.1 (`CallToolResult.StructuredContent`) supports the field
- The existing fleet-e2e Kind cluster (preserved from TLS Dex OIDC validation) was
  reused for live testing

## Test Environment

- **Cluster**: Kind `fleet-e2e` (K8s v1.34, Kuadrant MCP Gateway, preserved cluster)
- **kube-mcp-server**: `ghcr.io/containers/kubernetes-mcp-server:latest` (pulled
  2026-06-29 post-merge, loaded into Kind via `kind load image-archive`)
- **MCP Gateway**: Kuadrant with Istio, NodePort 31975 (`/mcp` Streamable HTTP)
- **SDK**: `github.com/modelcontextprotocol/go-sdk` v1.6.1

## Key Findings

### 1. structuredContent is live and working

Both `resources_get` and `resources_list` return `structuredContent` alongside
unchanged text content. Backward compatible — existing clients that only read
`Content[0].Text` see no difference.

### 2. structuredContent shape depends on `--list-output` flag

**Critical finding**: the `structuredContent` shape for `resources_list` varies
based on the `--list-output` flag:

| Flag | `structuredContent` shape for `resources_list` | Full K8s object? |
|------|-----------------------------------------------|-----------------|
| `--list-output=table` (default) | `{"items": [{"Name":"x","Status":"y","Age":"5m","kind":"Pod","apiVersion":"v1",...}]}` — flat table-row maps with column headers as keys | **No** — missing `metadata`, `spec`, `status` |
| `--list-output=yaml` | `{"items": [{"apiVersion":"v1","kind":"Pod","metadata":{"name":"x","namespace":"y",...},"spec":{...},"status":{...}}]}` — full K8s objects | **Yes** |

**Root cause**: `PrintObjStructured` dispatches to either `table.PrintObjStructured()`
or `yaml.PrintObjStructured()`. The table path uses `tableToStructured()` which
converts K8s Table API rows into flat column-keyed maps. The YAML path returns
`item.DeepCopy().Object` — the full K8s object.

**Impact**: With `--list-output=table` (default), `structuredContent` items use
top-level `Name`/`Namespace` keys (not `metadata.name`/`metadata.namespace`).
`unstructured.Unstructured.GetName()` returns `""` because it reads
`metadata.name`. The FMC syncer, which calls `item.GetName()`, `item.GetNamespace()`,
and `item.GroupVersionKind()`, would silently produce broken scope cache keys.

### 3. `--list-output=yaml` gives full K8s objects for both tools

With `--list-output=yaml`:

- **`resources_get`**: `structuredContent` is `map[string]any` — full K8s object
  (apiVersion, kind, metadata with name/namespace/labels/ownerReferences, spec, status).
  Maps directly to `unstructured.Unstructured.Object`.

- **`resources_list`**: `structuredContent` is `map[string]any{"items": [...]}` where
  each item is a full K8s object. All standard `unstructured.Unstructured` accessors
  work correctly (`GetName()`, `GetNamespace()`, `GetLabels()`, `GroupVersionKind()`,
  `GetOwnerReferences()`).

### 4. Existing `extractStructured` has a shape mismatch

Our `extractStructured()` (client.go:396) does:

```go
items, ok := result.StructuredContent.([]any)
```

But the actual value is `map[string]any{"items": [...]}`. The type assertion
fails silently and the code gracefully falls through to text parsing. No breakage,
but structured data is being ignored.

**Fix needed**: unwrap the `{"items": [...]}` envelope before extracting items.

### 5. `resources_get` has no structuredContent path today

`getResource()` currently only uses `ExtractText(result)` + `parseUnstructured()`.
It should prefer `structuredContent` (a direct `map[string]any`) when available,
eliminating the YAML→JSON→Unstructured round-trip.

### 6. Text content (Content[0].Text) is unchanged

`resources_get` still returns YAML text. `resources_list` returns YAML text when
`--list-output=yaml`. Both are present alongside `structuredContent` for backward
compatibility.

## Validation Results

### Run 1: Default `--list-output=table`

All 4 tests returned `structuredContent`, but items were flat table-row maps:

| Test | Tool | structuredContent | Items shape |
|------|------|------------------|-------------|
| Namespace list | `resources_list` | `map[string]any` with `items` | Flat: `{Name, Status, Age, kind, apiVersion}` |
| Namespace/default get | `resources_get` | `map[string]any` | Full K8s object |
| Pod list (kubernaut-system) | `resources_list` | `map[string]any` with `items` | Flat: `{Name, Namespace, Ready, Status, ...}` |
| Service/kube-mcp-server get | `resources_get` | `map[string]any` | Full K8s object |

**Result**: `resources_get` works perfectly. `resources_list` items lack
`metadata.name`/`metadata.namespace` — incompatible with `unstructured.GetName()`.

### Run 2: `--list-output=yaml` (patched deployment)

After patching kube-mcp-server with `--list-output=yaml`:

| Test | Tool | structuredContent | Items shape |
|------|------|------------------|-------------|
| Namespace list | `resources_list` | `map[string]any` with `items` | Full K8s objects with `metadata.name`, `spec`, `status` |
| Namespace/default get | `resources_get` | `map[string]any` | Full K8s object (unchanged) |
| Pod list (kubernaut-system) | `resources_list` | `map[string]any` with `items` (220KB) | Full K8s objects with `metadata`, `spec`, `status` |
| Service/kube-mcp-server get | `resources_get` | `map[string]any` | Full K8s object (unchanged) |

**Result**: All 4 tests return full K8s objects. All `unstructured.Unstructured`
accessors work correctly.

## Decision

### Hard requirement: `--list-output=yaml`

**kube-mcp-server MUST be deployed with `--list-output=yaml`** for Kubernaut's
`mcpclient` to consume `structuredContent` reliably for `resources_list`.

Without this flag, `resources_list` returns flat table-row maps in
`structuredContent` that are incompatible with `unstructured.Unstructured`
accessors used by the FMC syncer and other consumers.

This is a **deployment requirement**, not a code workaround — it ensures the
structured data path produces the same full-object fidelity as `resources_get`.

### Text parsing fallback: retain as degraded mode

The text parsing chain (`parseMultiFormat`, `parseTableText`, `looksLikeTable`)
should be retained as a degraded-mode fallback for environments running older
kube-mcp-server versions that don't emit `structuredContent`. The primary data
path is `structuredContent`; the text path only activates when `structuredContent`
is nil. Log a warning when falling back to text parsing so operators know to
upgrade.

### Code changes required

1. **`extractStructured`**: Handle `map[string]any{"items": [...]}` envelope
   (current code expects `[]any`)
2. **`getResource`**: Prefer `structuredContent` (`map[string]any`) over text
   parsing when available
3. **`fleet_e2e.go`**: Add `--list-output=yaml` to kube-mcp-server deployment args
4. **Helm chart**: Add `--list-output=yaml` to kube-mcp-server deployment template
5. **ADR-068**: Document `--list-output=yaml` as a deployment requirement
6. **Issue #54**: Update with structuredContent status and deployment requirement

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| `--list-output=yaml` increases response size | Pod list (20 pods) was 220KB structured vs 6KB table | Acceptable for programmatic clients; LLMs should use separate MCP sessions with table output |
| Upstream removes `structuredContent` from YAML path | Client falls back to YAML text parsing (same as today) | Text fallback retained; YAML parsing is robust |
| Kuadrant gateway strips `structuredContent` on proxy | Client falls back to text parsing | Validated: Kuadrant passes `structuredContent` through correctly |
| Environments with older kube-mcp-server | `structuredContent` is nil, text fallback activates | Fallback retained with warning log |

## Confidence Assessment: 95%

- Validated against live Kind cluster with Kuadrant MCP Gateway
- Both `resources_get` and `resources_list` return full K8s objects with `--list-output=yaml`
- All `unstructured.Unstructured` accessors work correctly on structured data
- Kuadrant gateway confirmed to pass `structuredContent` through without modification
- Existing text fallback provides safe degradation path

## Files

- `main.go` — Spike validation tool (Streamable HTTP MCP client → MCP Gateway → kube-mcp-server)
