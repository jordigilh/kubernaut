// Command gen-helm-config-docs generates docs/generated/helm-values-reference.md
// from charts/kubernaut/values.schema.json (DD-PLATFORM-006 Decision Area 4).
//
// Modeled on the existing hack/crd-ref-docs pattern (Go types -> Markdown):
// this walks the JSON Schema's properties/definitions/$ref/required instead
// of Go AST, emitting one table per top-level service so the generated
// reference never drifts from the schema that Helm actually validates
// against -- unlike a hand-maintained field list in README.md.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
)

// SchemaNode is a JSON Schema (draft-07 subset) node. Fields are deliberately
// loose (json.RawMessage for polymorphic "type"/"default"/"additionalProperties")
// because values.schema.json mixes string and []string "type", and
// "additionalProperties" is sometimes a bool and sometimes a nested schema
// object (the map-of-arbitrary-keys pattern, e.g. global.llmProfiles).
type SchemaNode struct {
	Type                 json.RawMessage        `json:"type,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Default              json.RawMessage        `json:"default,omitempty"`
	Properties           map[string]*SchemaNode `json:"properties,omitempty"`
	Items                *SchemaNode            `json:"items,omitempty"`
	Required             []string               `json:"required,omitempty"`
	Ref                  string                 `json:"$ref,omitempty"`
	AllOf                []*SchemaNode          `json:"allOf,omitempty"`
	AdditionalProperties json.RawMessage        `json:"additionalProperties,omitempty"`
	Enum                 []json.RawMessage      `json:"enum,omitempty"`
}

// RootSchema is the top-level values.schema.json document.
type RootSchema struct {
	Definitions map[string]*SchemaNode `json:"definitions"`
	Properties  map[string]*SchemaNode `json:"properties"`
}

// Row is one generated Markdown table row: a single leaf configuration
// parameter (dotted path) with its type/description/default/required state.
type Row struct {
	Parameter   string
	Type        string
	Description string
	Default     string
	Required    bool
}

func parseSchema(data []byte) (*RootSchema, error) {
	var root RootSchema
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parsing values.schema.json: %w", err)
	}
	if root.Definitions == nil {
		root.Definitions = map[string]*SchemaNode{}
	}
	return &root, nil
}

// refName extracts "goDuration" from "#/definitions/goDuration". Only local
// same-document definition refs are supported -- values.schema.json never
// uses external/remote $refs.
func refName(ref string) string {
	const prefix = "#/definitions/"
	return strings.TrimPrefix(ref, prefix)
}

// resolveNode expands $ref/allOf against defs, merging the referenced
// definition's fields as a base with the node's own fields taking priority
// (a property's own "default"/"description" always wins over the shared
// definition it references -- e.g. goDuration's own doc-comment description
// is generic, but a specific field's "default": "30s" is field-specific).
func resolveNode(n *SchemaNode, defs map[string]*SchemaNode) *SchemaNode {
	if n == nil {
		return nil
	}
	merged := &SchemaNode{
		Type:                 n.Type,
		Description:          n.Description,
		Default:              n.Default,
		Properties:           n.Properties,
		Items:                n.Items,
		Required:             n.Required,
		Enum:                 n.Enum,
		AdditionalProperties: n.AdditionalProperties,
	}
	if n.Ref != "" {
		merged = mergeNodes(resolveNode(defs[refName(n.Ref)], defs), merged)
	}
	for _, sub := range n.AllOf {
		merged = mergeNodes(resolveNode(sub, defs), merged)
	}
	return merged
}

// mergeNodes overlays override's non-empty fields onto base. base is nil-safe
// (returns override unchanged if base is nil, e.g. an unresolvable $ref).
func mergeNodes(base, override *SchemaNode) *SchemaNode {
	if base == nil {
		return override
	}
	result := *base
	if len(override.Type) > 0 {
		result.Type = override.Type
	}
	if override.Description != "" {
		result.Description = override.Description
	}
	if len(override.Default) > 0 {
		result.Default = override.Default
	}
	if override.Properties != nil {
		result.Properties = override.Properties
	}
	if override.Items != nil {
		result.Items = override.Items
	}
	if override.Required != nil {
		result.Required = override.Required
	}
	if override.Enum != nil {
		result.Enum = override.Enum
	}
	if len(override.AdditionalProperties) > 0 {
		result.AdditionalProperties = override.AdditionalProperties
	}
	return &result
}

// typeString renders the resolved node's JSON Schema "type" (plus map/array
// shape) as a short human-readable string for the generated table's Type
// column, e.g. "string", "array of string", "map[string]object". defs
// resolves any $ref/allOf on the array's "items" (e.g. tolerations' items
// ref the "toleration" definition rather than declaring "type" inline).
func (n *SchemaNode) typeString(defs map[string]*SchemaNode) string {
	if n.isMap() {
		return "map[string]object"
	}
	t := decodeTypeField(n.Type)
	if t == "array" {
		item := resolveNode(n.Items, defs)
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

// isMap reports whether additionalProperties is a nested schema object
// (map-of-arbitrary-keys, e.g. global.llmProfiles keyed by profile name)
// rather than the more common boolean form (additionalProperties: false,
// meaning "no extra keys allowed", which carries no type information).
func (n *SchemaNode) isMap() bool {
	if len(n.AdditionalProperties) == 0 {
		return false
	}
	var b bool
	if err := json.Unmarshal(n.AdditionalProperties, &b); err == nil {
		return false // boolean form, not a map schema
	}
	return true
}

func (n *SchemaNode) defaultString() string {
	if len(n.Default) == 0 {
		return ""
	}
	return string(n.Default)
}

// isObjectWithProperties reports whether a node should be recursed into
// (rendered as nested dotted-path rows) rather than emitted as a single leaf
// row. Map-type objects (isMap) are always leaves -- their keys are
// arbitrary user-chosen names, not fixed schema properties to enumerate.
func (n *SchemaNode) isObjectWithProperties() bool {
	return len(n.Properties) > 0 && !n.isMap()
}

func sortedKeys(m map[string]*SchemaNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func toSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, i := range items {
		set[i] = true
	}
	return set
}

// walk recursively flattens a properties map into leaf Rows, expanding
// $ref/allOf via defs and threading dotted parameter paths (e.g.
// "gateway.config.server.readTimeout") through nested objects.
func walk(prefix string, props map[string]*SchemaNode, required []string, defs map[string]*SchemaNode) []Row {
	var rows []Row
	reqSet := toSet(required)
	for _, name := range sortedKeys(props) {
		node := resolveNode(props[name], defs)
		path := name
		if prefix != "" {
			path = prefix + "." + name
		}
		if node.isObjectWithProperties() {
			rows = append(rows, walk(path, node.Properties, node.Required, defs)...)
			continue
		}
		rows = append(rows, Row{
			Parameter:   path,
			Type:        node.typeString(defs),
			Description: node.Description,
			Default:     node.defaultString(),
			Required:    reqSet[name],
		})
	}
	return rows
}

// GenerateMarkdown renders one "## <service>" section with its own
// Parameter/Type/Description/Default/Required table per top-level
// values.schema.json property, mirroring README.md's existing per-service
// structure so the two stay visually consistent.
func GenerateMarkdown(root *RootSchema) string {
	var sb strings.Builder
	sb.WriteString("# Kubernaut Helm Chart Configuration Reference\n\n")
	sb.WriteString("Auto-generated from `charts/kubernaut/values.schema.json` by " +
		"`hack/gen-helm-config-docs` (`make generate-helm-config-docs`). " +
		"Do not edit by hand -- changes will be overwritten on the next " +
		"generation and CI's drift check (`make generate-helm-config-docs && " +
		"git diff --exit-code -- docs/generated/helm-values-reference.md`) " +
		"will fail on a stale, hand-edited copy.\n\n")

	for _, service := range sortedKeys(root.Properties) {
		node := resolveNode(root.Properties[service], root.Definitions)
		sb.WriteString("## " + service + "\n\n")
		rows := walk("", node.Properties, node.Required, root.Definitions)
		if len(rows) == 0 {
			// Leaf top-level scalar (e.g. nameOverride/fullnameOverride,
			// which have no nested properties of their own).
			rows = []Row{{
				Parameter:   service,
				Type:        node.typeString(root.Definitions),
				Description: node.Description,
				Default:     node.defaultString(),
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
	root, err := parseSchema(data)
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
