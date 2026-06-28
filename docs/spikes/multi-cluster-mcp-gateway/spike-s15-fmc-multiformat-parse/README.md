# Spike S15 — FMC Multi-Format Response Parsing

## Goal

Validate that the FMC syncer can parse `kube-mcp-server` `resources_list` responses
in **both** the `table` (default since v0.0.40, PR #113) and `yaml` output formats,
extracting the resource metadata needed for Valkey scope cache keys.

Supersedes spike-s7's YAML-only assumption — the default changed from YAML to `table`
to reduce LLM context window usage (Issue #75).

## Context

- `kube-mcp-server` v0.0.40+ defaults to `--list-output=table`
- The FMC syncer needs: GVK + namespace + name (for Valkey keys)
- `labelSelector=kubernaut.ai/managed=true` filters server-side (no client-side label parsing needed)
- The MCP spec's `StructuredContent` field exists in go-sdk v1.6.1 but `kube-mcp-server`
  does not yet emit it for `resources_list` (tracked in their Issue #920)

## Test Environment

- **Cluster**: OCP dev cluster (`api-dev-redhat-internal-com:6443`, OCP 4.18, K8s v1.31.14)
- **kube-mcp-server**: v0.0.63 (built from `main`, commit at time of spike)
- **Resources created**: Deployment, Service (namespaced in `kubernaut-spike`), Node (cluster-scoped)
- **Labels**: `kubernaut.ai/managed=true` on Deployment, Service, and one worker Node

## Key Findings

### 1. Table Format Columns (better than expected)

The table output includes APIVERSION, KIND, NAME, and LABELS columns for **all**
resource types, due to `WithKind: true` and `ShowLabels: true` hardcoded in
`pkg/output/output.go`. This means we can extract all four metadata fields
directly from the table — no need to inject GVK from call context.

**Deployment (namespaced):**
```
NAMESPACE         APIVERSION   KIND         NAME                READY   UP-TO-DATE   AVAILABLE   AGE   CONTAINERS   IMAGES       SELECTOR        LABELS
kubernaut-spike   apps/v1      Deployment   spike-managed-web   0/1     1            0           89s   nginx        nginx:1.27   app=spike-web   app=spike-web,kubernaut.ai/managed=true
```

**Node (cluster-scoped — no NAMESPACE column):**
```
APIVERSION   KIND   NAME                               STATUS   ROLES    AGE    VERSION    INTERNAL-IP       EXTERNAL-IP   OS-IMAGE   ...   LABELS
v1           Node   dev-worker-0.redhat-internal.com   Ready    worker   3d9h   v1.31.14   192.168.122.228   <none>        ...              beta.kubernetes.io/arch=amd64,...,kubernaut.ai/managed=true,...
```

**Service (namespaced):**
```
NAMESPACE         APIVERSION   KIND      NAME                TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)   AGE   SELECTOR        LABELS
kubernaut-spike   v1           Service   spike-managed-svc   ClusterIP   172.30.47.244   <none>        80/TCP    89s   app=spike-web   app=spike-svc,kubernaut.ai/managed=true
```

### 2. StructuredContent is nil

Confirmed against live v0.0.63: `CallToolResult.StructuredContent` is nil for
both `table` and `yaml` modes. The structured output spec says "has no in-tree
adopter yet" (tracked in kube-mcp-server Issue #920).

### 3. YAML List Format

YAML mode returns a YAML array (`- apiVersion: ...`) that `sigs.k8s.io/yaml`
parses directly as `[]map[string]any`. Full objects including spec and status.

### 4. resources_get Always Returns YAML

Single resource `resources_get` returns full YAML regardless of `--list-output`
setting. This is the existing behavior our `parseUnstructured()` handles.

## Parse Priority Chain (validated)

```
1. StructuredContent  → nil today; ready for kube-mcp-server #920
2. JSON unmarshal     → existing behavior, handles JSON list/object responses
3. YAML unmarshal     → handles --list-output=yaml (sigs.k8s.io/yaml, already in go.mod)
4. Table text parse   → handles --list-output=table (default since v0.0.40)
```

Format detection uses `looksLikeTable()` heuristic: first line contains "NAME"
and one of "KIND", "APIVERSION", or "AGE" in uppercase.

## Validation Results

### Unit Tests (12/12 PASS)

| Test | Format | Resource | Result |
|------|--------|----------|--------|
| TestTableParserDeploymentNamespaced | table | Deployment | PASS |
| TestTableParserNodeClusterScoped | table | Node | PASS |
| TestTableParserServiceNamespaced | table | Service | PASS |
| TestYAMLParserList | yaml | Deployment list | PASS |
| TestYAMLParserServiceList | yaml | Service list | PASS |
| TestYAMLParserSingleGet | yaml | Deployment single | PASS |
| TestMultiFormatJSON | json | Pod | PASS |
| TestMultiFormatYAML | yaml | Deployment | PASS |
| TestMultiFormatTable | table | Deployment | PASS |
| TestMultiFormatTableClusterScoped | table | Node | PASS |
| TestMultiFormatEmpty | empty | - | PASS |
| TestLooksLikeTable | heuristic | mixed | PASS |

### E2E Tests Against Live OCP (5/5 PASS)

| Test | Mode | Resource | Result |
|------|------|----------|--------|
| TestE2ETableDeployment | table | Deployment (namespaced) | PASS |
| TestE2ETableNodeClusterScoped | table | Node (cluster-scoped) | PASS |
| TestE2EYAMLDeployment | yaml | Deployment (namespaced) | PASS |
| TestE2EYAMLNodeClusterScoped | yaml | Node (cluster-scoped) | PASS |
| TestE2EStructuredContentNil | table | Service | PASS (nil confirmed) |

## Decision: Do NOT Import kube-mcp-server

Importing `github.com/containers/kubernetes-mcp-server/pkg/output` was considered
for format compatibility guarantees but rejected because:

1. **Pulls in `k8s.io/cli-runtime`** — heavy dependency not in our go.mod
2. **`tableToStructured()` is unexported** — can't call the key function
3. **Server-side types** — `PrintObjStructured()` takes `runtime.Unstructured` (raw K8s API response), not the text string we receive over MCP
4. **Format is simple JSON/text** — `[]map[string]any` on wire, no complex Go types to gain

Instead: parse the documented format directly. Zero new dependencies (`sigs.k8s.io/yaml`
is already in `go.mod`).

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Issue #824 removes `WithKind: true` / `ShowLabels: true` | Lose APIVERSION/KIND/LABELS columns in table | Fall back to call context for GVK; FMC doesn't need labels from table since labelSelector filters server-side |
| kube-mcp-server changes table column order or format | Table parser breaks | `looksLikeTable()` + column-header-based positioning (not index-based) is resilient to column reordering |
| StructuredContent ships in kube-mcp-server #920 | Priority 1 path activates | Already coded as first check — transparent upgrade |

## Confidence Assessment: 95%

- Validated against real OCP cluster with 3 resource types (namespaced + cluster-scoped)
- Both output formats (table + YAML) parse correctly into `unstructured.Unstructured`
- `sigs.k8s.io/yaml` already in project dependencies
- Table parser is column-header-based (resilient to column order changes)
- `looksLikeTable()` heuristic correctly distinguishes all formats tested

## Files

- `parse_spike_test.go` — Parser prototypes + unit tests (12 tests)
- `e2e_spike_test.go` — Live cluster E2E tests (5 tests)
- `main.go` — Raw response capture tool (stdio MCP client → kube-mcp-server)
