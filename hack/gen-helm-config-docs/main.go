// Command gen-helm-config-docs generates docs/generated/helm-values-reference.md
// from charts/kubernaut/values.schema.json (DD-PLATFORM-006 Decision Area 4).
//
// Modeled on the existing hack/crd-ref-docs pattern (Go types -> Markdown):
// this walks the JSON Schema's properties/definitions/$ref/required instead
// of Go AST, emitting one table per top-level service so the generated
// reference never drifts from the schema that Helm actually validates
// against -- unlike a hand-maintained field list in README.md.
//
// The schema-walking core ($ref/allOf resolution) lives in
// hack/internal/helmschema, shared with hack/gen-helm-defaults (PR9,
// DD-PLATFORM-006 Decision Area 14) so both generators stay in lockstep with
// the same resolution semantics.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/jordigilh/kubernaut/hack/internal/helmschema"
)

// Row is one generated Markdown table row: a single leaf configuration
// parameter (dotted path) with its type/description/default/required state.
type Row struct {
	Parameter   string
	Type        string
	Description string
	Default     string
	Required    bool
}

// typeString renders the resolved node's JSON Schema "type" (plus map/array
// shape) as a short human-readable string for the generated table's Type
// column, e.g. "string", "array of string", "map[string]object". defs
// resolves any $ref/allOf on the array's "items" (e.g. tolerations' items
// ref the "toleration" definition rather than declaring "type" inline).
func typeString(n *helmschema.SchemaNode, defs map[string]*helmschema.SchemaNode) string {
	if n.IsMap() {
		return "map[string]object"
	}
	t := decodeTypeField(n.Type)
	if t == "array" {
		item := helmschema.ResolveNode(n.Items, defs)
		itemType := "object"
		if item != nil {
			if it := decodeTypeField(item.Type); it != "" {
				itemType = it
			}
		}
		return "array of " + itemType
	}
	if t == "" {
		return "object"
	}
	return t
}

func decodeTypeField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil && len(list) > 0 {
		return list[0]
	}
	return ""
}

// walk recursively flattens a properties map into leaf Rows, expanding
// $ref/allOf via defs and threading dotted parameter paths (e.g.
// "gateway.config.server.readTimeout") through nested objects.
func walk(prefix string, props map[string]*helmschema.SchemaNode, required []string, defs map[string]*helmschema.SchemaNode) []Row {
	var rows []Row
	reqSet := helmschema.ToSet(required)
	for _, name := range helmschema.SortedKeys(props) {
		node := helmschema.ResolveNode(props[name], defs)
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if node.IsObjectWithProperties() {
			rows = append(rows, walk(path, node.Properties, node.Required, defs)...)
			continue
		}
		rows = append(rows, Row{
			Parameter:   path,
			Type:        typeString(node, defs),
			Description: node.Description,
			Default:     node.DefaultJSON(),
			Required:    reqSet[name],
		})
	}
	return rows
}

// GenerateMarkdown renders one "## <service>" section with its own
// Parameter/Type/Description/Default/Required table per top-level
// values.schema.json property, mirroring README.md's existing per-service
// structure so the two stay visually consistent.
func GenerateMarkdown(root *helmschema.RootSchema) string {
	var sb strings.Builder
	sb.WriteString("# Kubernaut Helm Chart Configuration Reference\n\n")
	sb.WriteString("Auto-generated from `charts/kubernaut/values.schema.json` by " +
		"`hack/gen-helm-config-docs` (`make generate-helm-config-docs`). " +
		"Do not edit by hand -- changes will be overwritten on the next " +
		"generation and CI's drift check (`make generate-helm-config-docs && " +
		"git diff --exit-code -- docs/generated/helm-values-reference.md`) " +
		"will fail on a stale, hand-edited copy.\n\n")

	for _, service := range helmschema.SortedKeys(root.Properties) {
		node := helmschema.ResolveNode(root.Properties[service], root.Definitions)
		sb.WriteString("## " + service + "\n\n")
		rows := walk("", node.Properties, node.Required, root.Definitions)
		if len(rows) == 0 {
			// Leaf top-level scalar (e.g. nameOverride/fullnameOverride,
			// which have no nested properties of their own).
			rows = []Row{{
				Parameter:   service,
				Type:        typeString(node, root.Definitions),
				Description: node.Description,
				Default:     node.DefaultJSON(),
			}}
		}
		sb.WriteString("| Parameter | Type | Description | Default | Required |\n")
		sb.WriteString("|-----------|------|--------------|---------|----------|\n")
		for _, r := range rows {
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | `%s` | %s |\n",
				r.Parameter, escapeCell(r.Type), escapeCell(r.Description), escapeCell(r.Default), requiredCell(r.Required)))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func requiredCell(required bool) string {
	if required {
		return "Yes"
	}
	return "No"
}

// escapeCell prevents a description/type/default value containing a literal
// "|" or newline from corrupting the Markdown table's column alignment.
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func main() {
	schemaPath := flag.String("schema", "charts/kubernaut/values.schema.json", "path to values.schema.json")
	outputPath := flag.String("output", "docs/generated/helm-values-reference.md", "path to write the generated Markdown reference")
	flag.Parse()

	data, err := os.ReadFile(*schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-helm-config-docs: reading schema: %v\n", err)
		os.Exit(1)
	}
	root, err := helmschema.ParseSchema(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-helm-config-docs: %v\n", err)
		os.Exit(1)
	}
	md := GenerateMarkdown(root)
	if err := os.WriteFile(*outputPath, []byte(md), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "gen-helm-config-docs: writing output: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("gen-helm-config-docs: wrote %s\n", *outputPath)
}
