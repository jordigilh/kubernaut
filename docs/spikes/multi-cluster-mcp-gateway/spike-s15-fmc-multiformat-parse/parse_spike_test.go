package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigsyaml "sigs.k8s.io/yaml"
)

// Real captured output from kube-mcp-server v0.0.63 against OCP dev cluster.

const tableDeploymentNamespaced = `NAMESPACE         APIVERSION   KIND         NAME                READY   UP-TO-DATE   AVAILABLE   AGE   CONTAINERS   IMAGES       SELECTOR        LABELS
kubernaut-spike   apps/v1      Deployment   spike-managed-web   0/1     1            0           89s   nginx        nginx:1.27   app=spike-web   app=spike-web,kubernaut.ai/managed=true`

const tableNodeClusterScoped = `APIVERSION   KIND   NAME                               STATUS   ROLES    AGE    VERSION    INTERNAL-IP       EXTERNAL-IP   OS-IMAGE                                                KERNEL-VERSION                  CONTAINER-RUNTIME                             LABELS
v1           Node   dev-worker-0.redhat-internal.com   Ready    worker   3d9h   v1.31.14   192.168.122.228   <none>        Red Hat Enterprise Linux CoreOS 418.94.202606051320-0   5.14.0-427.130.1.el9_4.x86_64   cri-o://1.31.13-10.rhaos4.18.git817a650.el9   beta.kubernetes.io/arch=amd64,beta.kubernetes.io/os=linux,kubernaut.ai/managed=true,kubernetes.io/arch=amd64,kubernetes.io/hostname=dev-worker-0.redhat-internal.com,kubernetes.io/os=linux,node-role.kubernetes.io/worker=,node.openshift.io/os_id=rhcos,topology.topolvm.io/node=dev-worker-0.redhat-internal.com`

const tableServiceNamespaced = `NAMESPACE         APIVERSION   KIND      NAME                TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)   AGE   SELECTOR        LABELS
kubernaut-spike   v1           Service   spike-managed-svc   ClusterIP   172.30.47.244   <none>        80/TCP    89s   app=spike-web   app=spike-svc,kubernaut.ai/managed=true`

const yamlDeploymentList = `- apiVersion: apps/v1
  kind: Deployment
  metadata:
    labels:
      app: spike-web
      kubernaut.ai/managed: "true"
    name: spike-managed-web
    namespace: kubernaut-spike
    resourceVersion: "3512410"
    uid: ef7214f7-6501-4770-9290-9593c9986d86
  spec:
    replicas: 1
  status:
    observedGeneration: 1`

const yamlServiceList = `- apiVersion: v1
  kind: Service
  metadata:
    labels:
      app: spike-svc
      kubernaut.ai/managed: "true"
    name: spike-managed-svc
    namespace: kubernaut-spike
    resourceVersion: "3511015"
    uid: 375db034-8924-4eb5-a791-cb41b9fbf0c9
  spec:
    clusterIP: 172.30.47.244
    type: ClusterIP`

const yamlSingleGet = `apiVersion: apps/v1
kind: Deployment
metadata:
  labels:
    app: spike-web
    kubernaut.ai/managed: "true"
  name: spike-managed-web
  namespace: kubernaut-spike
spec:
  replicas: 1`

// --- Table parser prototype ---

type tableColumn struct {
	name  string
	start int
	end   int // -1 means to end of line
}

func parseTableColumns(header string) []tableColumn {
	var cols []tableColumn
	i := 0
	for i < len(header) {
		// Skip leading whitespace
		for i < len(header) && header[i] == ' ' {
			i++
		}
		if i >= len(header) {
			break
		}
		start := i
		// Find end of column name (non-space run)
		for i < len(header) && header[i] != ' ' {
			// Handle multi-word headers like "UP-TO-DATE" -- they're hyphenated, not spaced
			i++
		}
		name := header[start:i]
		// Find start of next column (skip spaces)
		nextStart := i
		for nextStart < len(header) && header[nextStart] == ' ' {
			nextStart++
		}
		cols = append(cols, tableColumn{name: name, start: start, end: nextStart})
		i = nextStart
	}
	// Last column extends to end of line
	if len(cols) > 0 {
		cols[len(cols)-1].end = -1
	}
	return cols
}

func extractTableField(row string, col tableColumn) string {
	if col.start >= len(row) {
		return ""
	}
	var val string
	if col.end == -1 || col.end > len(row) {
		val = row[col.start:]
	} else {
		val = row[col.start:col.end]
	}
	return strings.TrimSpace(val)
}

func findColumn(cols []tableColumn, name string) (tableColumn, bool) {
	for _, c := range cols {
		if strings.EqualFold(c.name, name) {
			return c, true
		}
	}
	return tableColumn{}, false
}

type parsedTableRow struct {
	Namespace  string
	Name       string
	Kind       string
	APIVersion string
	Labels     map[string]string
}

func parseTableText(text string) ([]parsedTableRow, error) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("table must have at least header + 1 data row, got %d lines", len(lines))
	}

	cols := parseTableColumns(lines[0])

	var rows []parsedTableRow
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		row := parsedTableRow{}
		if col, ok := findColumn(cols, "NAMESPACE"); ok {
			row.Namespace = extractTableField(line, col)
		}
		if col, ok := findColumn(cols, "NAME"); ok {
			row.Name = extractTableField(line, col)
		}
		if col, ok := findColumn(cols, "KIND"); ok {
			row.Kind = extractTableField(line, col)
		}
		if col, ok := findColumn(cols, "APIVERSION"); ok {
			row.APIVersion = extractTableField(line, col)
		}
		if col, ok := findColumn(cols, "LABELS"); ok {
			labelsStr := extractTableField(line, col)
			row.Labels = parseLabels(labelsStr)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func parseLabels(s string) map[string]string {
	if s == "" || s == "<none>" {
		return nil
	}
	result := make(map[string]string)
	for _, part := range strings.Split(s, ",") {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) == 2 {
			result[kv[0]] = kv[1]
		} else if len(kv) == 1 {
			result[kv[0]] = ""
		}
	}
	return result
}

func tableRowsToUnstructured(rows []parsedTableRow) []unstructured.Unstructured {
	result := make([]unstructured.Unstructured, 0, len(rows))
	for _, r := range rows {
		obj := unstructured.Unstructured{Object: map[string]any{
			"apiVersion": r.APIVersion,
			"kind":       r.Kind,
			"metadata":   map[string]any{},
		}}
		meta := obj.Object["metadata"].(map[string]any)
		if r.Namespace != "" {
			meta["namespace"] = r.Namespace
		}
		if r.Name != "" {
			meta["name"] = r.Name
		}
		if len(r.Labels) > 0 {
			labels := make(map[string]any, len(r.Labels))
			for k, v := range r.Labels {
				labels[k] = v
			}
			meta["labels"] = labels
		}
		result = append(result, obj)
	}
	return result
}

// --- YAML parser prototype ---

func parseYAMLList(text string) ([]unstructured.Unstructured, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	// Try JSON first (existing behavior)
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err == nil {
		if items, ok := raw["items"].([]any); ok {
			result := make([]unstructured.Unstructured, 0, len(items))
			for _, item := range items {
				if m, ok := item.(map[string]any); ok {
					result = append(result, unstructured.Unstructured{Object: m})
				}
			}
			return result, nil
		}
		return []unstructured.Unstructured{{Object: raw}}, nil
	}

	// Try JSON array
	var jsonItems []map[string]any
	if err := json.Unmarshal([]byte(text), &jsonItems); err == nil {
		result := make([]unstructured.Unstructured, len(jsonItems))
		for i, item := range jsonItems {
			result[i] = unstructured.Unstructured{Object: item}
		}
		return result, nil
	}

	// Try YAML array (kube-mcp-server YAML list format: "- apiVersion: ...")
	var yamlItems []map[string]any
	if err := sigsyaml.Unmarshal([]byte(text), &yamlItems); err == nil && len(yamlItems) > 0 {
		result := make([]unstructured.Unstructured, len(yamlItems))
		for i, item := range yamlItems {
			result[i] = unstructured.Unstructured{Object: item}
		}
		return result, nil
	}

	// Try single YAML object
	var singleObj map[string]any
	if err := sigsyaml.Unmarshal([]byte(text), &singleObj); err == nil && singleObj != nil {
		return []unstructured.Unstructured{{Object: singleObj}}, nil
	}

	return nil, fmt.Errorf("unable to parse response as JSON, YAML list, or YAML object")
}

// --- Multi-format parser (the full priority chain) ---

// parseMultiFormat is also called from e2e_spike_test.go in this package (via
// fullPriorityChain) with a variable kind argument.
//
//nolint:unparam // fallbackKind is unused by every call site in this file, but removing it would require editing e2e_spike_test.go too, which is out of scope for this edit
func parseMultiFormat(text string, fallbackKind, fallbackAPIVersion string) ([]unstructured.Unstructured, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	// Priority 1: Try JSON (existing behavior)
	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err == nil {
		if items, ok := raw["items"].([]any); ok {
			result := make([]unstructured.Unstructured, 0, len(items))
			for _, item := range items {
				if m, ok := item.(map[string]any); ok {
					result = append(result, unstructured.Unstructured{Object: m})
				}
			}
			return result, nil
		}
		return []unstructured.Unstructured{{Object: raw}}, nil
	}
	var jsonItems []map[string]any
	if err := json.Unmarshal([]byte(text), &jsonItems); err == nil {
		result := make([]unstructured.Unstructured, len(jsonItems))
		for i, item := range jsonItems {
			result[i] = unstructured.Unstructured{Object: item}
		}
		return result, nil
	}

	// Priority 2: Try YAML (kube-mcp-server --list-output=yaml)
	var yamlItems []map[string]any
	if err := sigsyaml.Unmarshal([]byte(text), &yamlItems); err == nil && len(yamlItems) > 0 {
		result := make([]unstructured.Unstructured, len(yamlItems))
		for i, item := range yamlItems {
			result[i] = unstructured.Unstructured{Object: item}
		}
		return result, nil
	}
	var singleObj map[string]any
	if err := sigsyaml.Unmarshal([]byte(text), &singleObj); err == nil && singleObj != nil {
		if _, hasKind := singleObj["kind"]; hasKind {
			return []unstructured.Unstructured{{Object: singleObj}}, nil
		}
	}

	// Priority 3: Try table format (kube-mcp-server --list-output=table, default)
	if looksLikeTable(text) {
		rows, err := parseTableText(text)
		if err != nil {
			return nil, fmt.Errorf("table parse failed: %w", err)
		}
		return tableRowsToUnstructured(rows), nil
	}

	return nil, fmt.Errorf("unable to parse response in any supported format")
}

func looksLikeTable(text string) bool {
	lines := strings.SplitN(text, "\n", 2)
	if len(lines) == 0 {
		return false
	}
	header := strings.ToUpper(lines[0])
	return strings.Contains(header, "NAME") && (strings.Contains(header, "KIND") || strings.Contains(header, "APIVERSION") || strings.Contains(header, "AGE"))
}

// ===================== TESTS =====================

func TestTableParserDeploymentNamespaced(t *testing.T) {
	rows, err := parseTableText(tableDeploymentNamespaced)
	if err != nil {
		t.Fatalf("parseTableText: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	assertEqual(t, "Namespace", r.Namespace, "kubernaut-spike")
	assertEqual(t, "Name", r.Name, "spike-managed-web")
	assertEqual(t, "Kind", r.Kind, "Deployment")
	assertEqual(t, "APIVersion", r.APIVersion, "apps/v1")
	assertLabel(t, r.Labels, "kubernaut.ai/managed", "true")
	assertLabel(t, r.Labels, "app", "spike-web")

	objs := tableRowsToUnstructured(rows)
	if len(objs) != 1 {
		t.Fatalf("expected 1 unstructured, got %d", len(objs))
	}
	assertEqual(t, "obj.Name", objs[0].GetName(), "spike-managed-web")
	assertEqual(t, "obj.Namespace", objs[0].GetNamespace(), "kubernaut-spike")
	assertEqual(t, "obj.Kind", objs[0].GetKind(), "Deployment")
	assertEqual(t, "obj.APIVersion", objs[0].GetAPIVersion(), "apps/v1")
}

func TestTableParserNodeClusterScoped(t *testing.T) {
	rows, err := parseTableText(tableNodeClusterScoped)
	if err != nil {
		t.Fatalf("parseTableText: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	assertEqual(t, "Namespace", r.Namespace, "")
	assertEqual(t, "Name", r.Name, "dev-worker-0.redhat-internal.com")
	assertEqual(t, "Kind", r.Kind, "Node")
	assertEqual(t, "APIVersion", r.APIVersion, "v1")
	assertLabel(t, r.Labels, "kubernaut.ai/managed", "true")
	assertLabel(t, r.Labels, "kubernetes.io/hostname", "dev-worker-0.redhat-internal.com")

	objs := tableRowsToUnstructured(rows)
	assertEqual(t, "obj.Name", objs[0].GetName(), "dev-worker-0.redhat-internal.com")
	assertEqual(t, "obj.Namespace", objs[0].GetNamespace(), "")
	assertEqual(t, "obj.Kind", objs[0].GetKind(), "Node")
}

func TestTableParserServiceNamespaced(t *testing.T) {
	rows, err := parseTableText(tableServiceNamespaced)
	if err != nil {
		t.Fatalf("parseTableText: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	assertEqual(t, "Namespace", r.Namespace, "kubernaut-spike")
	assertEqual(t, "Name", r.Name, "spike-managed-svc")
	assertEqual(t, "Kind", r.Kind, "Service")
	assertEqual(t, "APIVersion", r.APIVersion, "v1")
}

func TestYAMLParserList(t *testing.T) {
	items, err := parseYAMLList(yamlDeploymentList)
	if err != nil {
		t.Fatalf("parseYAMLList: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertEqual(t, "Name", items[0].GetName(), "spike-managed-web")
	assertEqual(t, "Namespace", items[0].GetNamespace(), "kubernaut-spike")
	assertEqual(t, "Kind", items[0].GetKind(), "Deployment")
	assertEqual(t, "APIVersion", items[0].GetAPIVersion(), "apps/v1")
	labels := items[0].GetLabels()
	assertLabel(t, labels, "kubernaut.ai/managed", "true")
}

func TestYAMLParserServiceList(t *testing.T) {
	items, err := parseYAMLList(yamlServiceList)
	if err != nil {
		t.Fatalf("parseYAMLList: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertEqual(t, "Name", items[0].GetName(), "spike-managed-svc")
	assertEqual(t, "Namespace", items[0].GetNamespace(), "kubernaut-spike")
	assertEqual(t, "Kind", items[0].GetKind(), "Service")
}

func TestYAMLParserSingleGet(t *testing.T) {
	items, err := parseYAMLList(yamlSingleGet)
	if err != nil {
		t.Fatalf("parseYAMLList: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertEqual(t, "Name", items[0].GetName(), "spike-managed-web")
	assertEqual(t, "Kind", items[0].GetKind(), "Deployment")
}

func TestMultiFormatJSON(t *testing.T) {
	jsonText := `{"items":[{"apiVersion":"v1","kind":"Pod","metadata":{"name":"test","namespace":"default"}}]}`
	items, err := parseMultiFormat(jsonText, "Pod", "v1")
	if err != nil {
		t.Fatalf("parseMultiFormat JSON: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertEqual(t, "Name", items[0].GetName(), "test")
}

func TestMultiFormatYAML(t *testing.T) {
	items, err := parseMultiFormat(yamlDeploymentList, "Deployment", "apps/v1")
	if err != nil {
		t.Fatalf("parseMultiFormat YAML: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertEqual(t, "Name", items[0].GetName(), "spike-managed-web")
	assertEqual(t, "APIVersion", items[0].GetAPIVersion(), "apps/v1")
}

func TestMultiFormatTable(t *testing.T) {
	items, err := parseMultiFormat(tableDeploymentNamespaced, "Deployment", "apps/v1")
	if err != nil {
		t.Fatalf("parseMultiFormat Table: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertEqual(t, "Name", items[0].GetName(), "spike-managed-web")
	assertEqual(t, "Namespace", items[0].GetNamespace(), "kubernaut-spike")
	assertEqual(t, "Kind", items[0].GetKind(), "Deployment")
	assertEqual(t, "APIVersion", items[0].GetAPIVersion(), "apps/v1")
}

func TestMultiFormatTableClusterScoped(t *testing.T) {
	items, err := parseMultiFormat(tableNodeClusterScoped, "Node", "v1")
	if err != nil {
		t.Fatalf("parseMultiFormat Table cluster-scoped: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	assertEqual(t, "Name", items[0].GetName(), "dev-worker-0.redhat-internal.com")
	assertEqual(t, "Namespace", items[0].GetNamespace(), "")
	assertEqual(t, "Kind", items[0].GetKind(), "Node")
}

func TestMultiFormatEmpty(t *testing.T) {
	items, err := parseMultiFormat("", "Pod", "v1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Fatalf("expected nil, got %d items", len(items))
	}
}

func TestLooksLikeTable(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{tableDeploymentNamespaced, true},
		{tableNodeClusterScoped, true},
		{yamlDeploymentList, false},
		{yamlSingleGet, false},
		{`{"items":[]}`, false},
		{``, false},
	}
	for i, tt := range tests {
		if got := looksLikeTable(tt.input); got != tt.want {
			t.Errorf("test %d: looksLikeTable = %v, want %v (input starts with: %q)", i, got, tt.want, tt.input[:min(40, len(tt.input))])
		}
	}
}

// --- Helpers ---

func assertEqual(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %q, want %q", field, got, want)
	}
}

func assertLabel(t *testing.T, labels map[string]string, key, want string) {
	t.Helper()
	got, ok := labels[key]
	if !ok {
		t.Errorf("label %q not found in %v", key, labels)
		return
	}
	if got != want {
		t.Errorf("label %q: got %q, want %q", key, got, want)
	}
}
