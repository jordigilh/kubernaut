// Command gen-helm-defaults generates
// charts/kubernaut/templates/_generated_defaults.tpl from
// charts/kubernaut/values.schema.json (DD-PLATFORM-006 Decision Area 14,
// PR9). The generated file defines a single named template,
// "kubernaut.defaults", containing every schema-declared non-map default as
// a nested YAML tree keyed by top-level service. kubernaut.mergedValues
// (_helpers.tpl) `include`s and `fromYaml`s this template at Helm render
// time, then merges it with the user's own values.yaml/--set overrides
// (user values always win, including explicit false/0/"" -- see
// kubernaut.mergedValues' own doc comment for why mergeOverwrite, not
// merge, is required).
//
// Shares its schema-walking core (hack/internal/helmschema) with
// hack/gen-helm-config-docs, so both generators stay in lockstep with the
// same $ref/allOf resolution semantics.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jordigilh/kubernaut/hack/internal/helmschema"
	"gopkg.in/yaml.v3"
)

const templateHeader = `{{/*
DD-PLATFORM-006 Decision Area 14 (PR9): materialized schema defaults.
Auto-generated from charts/kubernaut/values.schema.json by
hack/gen-helm-defaults (` + "`make generate-helm-defaults`" + `). Do not edit
by hand -- changes will be overwritten on the next generation, and CI's
drift check (` + "`make generate-helm-defaults && git diff --exit-code -- " +
	"charts/kubernaut/templates/_generated_defaults.tpl`" + `) will fail on a
stale, hand-edited copy.

Consumed by kubernaut.mergedValues (_helpers.tpl), which merges these
defaults with the user's own values.yaml/--set overrides, user values always
winning (including explicit false/0/"").
*/}}
{{- define "kubernaut.defaults" -}}
`

// buildDefaultsTree walks the schema's top-level properties (services) and
// returns a nested map keyed by service name, containing only fields that
// declare a non-map schema default. Fields without a default, and map-type
// (arbitrary-key) objects, are omitted entirely -- there is nothing static
// to generate for them, and fabricating a zero-value default for an
// intentionally-undefaulted field would be actively harmful (it would make
// kubernaut.mergedValues "helpfully" supply e.g. a mandatory field's empty
// string instead of leaving it genuinely absent).
func buildDefaultsTree(root *helmschema.RootSchema) map[string]interface{} {
	tree := make(map[string]interface{}, len(root.Properties))
	for _, service := range helmschema.SortedKeys(root.Properties) {
		node := helmschema.ResolveNode(root.Properties[service], root.Definitions)
		if sub := walkDefaults(node, root.Definitions); len(sub) > 0 {
			tree[service] = sub
		}
	}
	return tree
}

// walkDefaults recursively builds a nested map of a single node's
// properties, decoding each leaf's declared JSON default into a Go value
// suitable for YAML re-encoding (json.Unmarshal decodes numbers as
// float64 -- acceptable here since the final output is re-marshaled to
// YAML text, not consumed as a typed Go value directly). Returns nil (not
// an empty map) when the node has no defaultable leaves anywhere in its
// subtree, so callers can omit genuinely-empty branches.
func walkDefaults(node *helmschema.SchemaNode, defs map[string]*helmschema.SchemaNode) map[string]interface{} {
	if node == nil || node.IsMap() {
		return nil
	}
	result := make(map[string]interface{}, len(node.Properties))
	for _, name := range helmschema.SortedKeys(node.Properties) {
		child := helmschema.ResolveNode(node.Properties[name], defs)
		if child.IsObjectWithProperties() {
			if sub := walkDefaults(child, defs); len(sub) > 0 {
				result[name] = sub
			}
			continue
		}
		if !child.HasDefault() {
			continue
		}
		var v interface{}
		if err := json.Unmarshal(child.Default, &v); err != nil {
			// Unreachable in practice: values.schema.json's "default" is
			// itself schema-validated JSON. Skip rather than fail the
			// whole generation run on one malformed field.
			continue
		}
		result[name] = v
	}
	return result
}

// renderTemplate marshals tree to YAML and wraps it in the
// "kubernaut.defaults" named-template define block that
// kubernaut.mergedValues expects to include+fromYaml.
func renderTemplate(tree map[string]interface{}) (string, error) {
	yamlBytes, err := yaml.Marshal(tree)
	if err != nil {
		return "", fmt.Errorf("marshaling defaults tree to YAML: %w", err)
	}
	var sb strings.Builder
	sb.WriteString(templateHeader)
	sb.Write(yamlBytes)
	sb.WriteString("{{- end -}}\n")
	return sb.String(), nil
}

func main() {
	schemaPath := flag.String("schema", "charts/kubernaut/values.schema.json", "path to values.schema.json")
	outputPath := flag.String("output", "charts/kubernaut/templates/_generated_defaults.tpl", "path to write the generated Helm template partial")
	flag.Parse()

	data, err := os.ReadFile(*schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-helm-defaults: reading schema: %v\n", err)
		os.Exit(1)
	}
	root, err := helmschema.ParseSchema(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-helm-defaults: %v\n", err)
		os.Exit(1)
	}
	tree := buildDefaultsTree(root)
	rendered, err := renderTemplate(tree)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-helm-defaults: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outputPath, []byte(rendered), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gen-helm-defaults: writing output: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("gen-helm-defaults: wrote %s\n", *outputPath)
}
