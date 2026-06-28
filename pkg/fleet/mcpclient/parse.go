/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mcpclient

import (
	"encoding/json"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigsyaml "sigs.k8s.io/yaml"
)

// tableColumn represents a detected column in kube-mcp-server table output.
type tableColumn struct {
	name  string
	start int
	end   int // -1 means extends to end of line
}

// parsedTableRow holds metadata extracted from a single table row.
type parsedTableRow struct {
	Namespace  string
	Name       string
	Kind       string
	APIVersion string
	Labels     map[string]string
}

// parseTableColumns extracts column positions from a table header line.
// Each column starts at the first non-space character after a gap and extends
// to the start of the next column. The last column extends to end of line.
func parseTableColumns(header string) []tableColumn {
	var cols []tableColumn
	i := 0
	for i < len(header) {
		for i < len(header) && header[i] == ' ' {
			i++
		}
		if i >= len(header) {
			break
		}
		start := i
		for i < len(header) && header[i] != ' ' {
			i++
		}
		name := header[start:i]
		nextStart := i
		for nextStart < len(header) && header[nextStart] == ' ' {
			nextStart++
		}
		cols = append(cols, tableColumn{name: name, start: start, end: nextStart})
		i = nextStart
	}
	if len(cols) > 0 {
		cols[len(cols)-1].end = -1
	}
	return cols
}

// extractTableField extracts a single field value from a table row using column positions.
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

// findColumn looks up a column by name (case-insensitive).
func findColumn(cols []tableColumn, name string) (tableColumn, bool) {
	for _, c := range cols {
		if strings.EqualFold(c.name, name) {
			return c, true
		}
	}
	return tableColumn{}, false
}

// parseTableText parses kube-mcp-server table text into parsedTableRow structs.
// Expects at least a header line plus one data row.
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

// parseLabels parses a comma-separated label string (e.g. "app=nginx,tier=web")
// into a map. Returns nil for empty or "<none>" input.
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

// tableRowsToUnstructured converts parsed table rows to K8s Unstructured objects.
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

// looksLikeTable returns true if the text appears to be kube-mcp-server table output.
// Checks that the first line contains NAME plus at least one of KIND, APIVERSION, or AGE.
func looksLikeTable(text string) bool {
	lines := strings.SplitN(text, "\n", 2)
	if len(lines) == 0 {
		return false
	}
	header := strings.ToUpper(lines[0])
	return strings.Contains(header, "NAME") &&
		(strings.Contains(header, "KIND") || strings.Contains(header, "APIVERSION") || strings.Contains(header, "AGE"))
}

func parseUnstructured(text string) (*unstructured.Unstructured, error) {
	if text == "" {
		return nil, fmt.Errorf("empty response")
	}

	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal([]byte(text), &obj.Object); err != nil {
		return nil, fmt.Errorf("unmarshaling resource: %w", err)
	}
	return obj, nil
}


// parseMultiFormat attempts to parse text as JSON, YAML, or table format
// using a priority chain: JSON > YAML > table.
func parseMultiFormat(text string) ([]unstructured.Unstructured, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}

	// Priority 1: JSON (K8s list with items, raw array, single object)
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

	// Priority 2: YAML (kube-mcp-server --list-output=yaml)
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

	// Priority 3: Table format (kube-mcp-server --list-output=table, default)
	if looksLikeTable(text) {
		rows, err := parseTableText(text)
		if err != nil {
			return nil, fmt.Errorf("table parse failed: %w", err)
		}
		return tableRowsToUnstructured(rows), nil
	}

	return nil, fmt.Errorf("unable to parse response in any supported format")
}
