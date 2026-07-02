# Spike S7 — FMC Writer: MCP `list_resources` Response Parsing

## Goal

Validate that the FMC Writer can:
1. Call `resources_list` via the MCP Gateway using `labelSelector=kubernaut.ai/managed=true`
2. Parse the YAML response to extract resource identity (GVK + namespace/name)
3. Write the extracted data to Valkey with the correct key format and TTL

## Key Findings

### K8s MCP Server `resources_list` Tool

- **Tool name**: `resources_list` (per-cluster via MCP Gateway: `{cluster}__resources_list`)
- **Required params**: `apiVersion` (e.g., "apps/v1"), `kind` (e.g., "Deployment")
- **Optional params**: `namespace`, `labelSelector`, `fieldSelector`, `gotemplate`
- **Response format**: Full YAML by default, or projected via `gotemplate`

### Efficient Strategy

Use `labelSelector=kubernaut.ai/managed=true` to pre-filter on the K8s MCP Server side.
This means the FMC Writer only receives managed resources — no client-side filtering needed.

With `gotemplate`, we can further reduce response size:
```
gotemplate: "{{range .items}}{{.metadata.namespace}}/{{.metadata.name}}\n{{end}}"
```

### Response Parsing

The K8s MCP Server returns text content in `CallToolResult.Content[0].Text`.
Format depends on `--list-output` flag:
- `yaml` (default): Full YAML of the resource list
- `table`: kubectl-style table format

For the FMC Writer, we parse the YAML response to extract resource identities.

## Validation Tests

- S7-001: Parse YAML list response and extract resource identities
- S7-002: Use labelSelector to filter managed resources only
- S7-003: Use gotemplate for minimal response (namespace/name only)
- S7-004: End-to-end: MCP call → parse → Valkey write → scope check
- S7-005: Error handling: malformed response, empty list, partial failures

## Conclusion

The `labelSelector` approach is the most efficient path:
- Server-side filtering reduces network payload
- FMC Writer only needs to parse the identity of resources that pass the filter
- With `gotemplate`, we can get just `namespace/name` pairs — minimal parsing overhead
- Fallback: parse full YAML when gotemplate is unavailable
