// Package helmschema parses charts/kubernaut/values.schema.json (JSON Schema
// draft-07 subset) and resolves its $ref/allOf indirection, so multiple
// generators (hack/gen-helm-config-docs, hack/gen-helm-defaults) can walk the
// same schema without duplicating the resolution logic (DD-PLATFORM-006
// Decision Area 14, PR9). Extracted unchanged from hack/gen-helm-config-docs,
// which was the original, proven implementation.
package helmschema

import (
	"encoding/json"
	"fmt"
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

// ParseSchema unmarshals values.schema.json's raw bytes into a RootSchema.
func ParseSchema(data []byte) (*RootSchema, error) {
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

// ResolveNode expands $ref/allOf against defs, merging the referenced
// definition's fields as a base with the node's own fields taking priority
// (a property's own "default"/"description" always wins over the shared
// definition it references -- e.g. goDuration's own doc-comment description
// is generic, but a specific field's "default": "30s" is field-specific).
func ResolveNode(n *SchemaNode, defs map[string]*SchemaNode) *SchemaNode {
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
		merged = mergeNodes(ResolveNode(defs[refName(n.Ref)], defs), merged)
	}
	for _, sub := range n.AllOf {
		merged = mergeNodes(ResolveNode(sub, defs), merged)
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

// IsMap reports whether additionalProperties is a nested schema object
// (map-of-arbitrary-keys, e.g. global.llmProfiles keyed by profile name)
// rather than the more common boolean form (additionalProperties: false,
// meaning "no extra keys allowed", which carries no type information).
func (n *SchemaNode) IsMap() bool {
	if len(n.AdditionalProperties) == 0 {
		return false
	}
	var b bool
	if err := json.Unmarshal(n.AdditionalProperties, &b); err == nil {
		return false // boolean form, not a map schema
	}
	return true
}

// HasDefault reports whether the schema declares a "default" for this node.
func (n *SchemaNode) HasDefault() bool {
	return len(n.Default) > 0
}

// DefaultJSON returns the node's raw "default" value as JSON text (empty
// string if none declared). JSON is a valid YAML scalar/sequence/mapping
// literal, so callers needing YAML output can embed this text directly.
func (n *SchemaNode) DefaultJSON() string {
	if len(n.Default) == 0 {
		return ""
	}
	return string(n.Default)
}

// IsObjectWithProperties reports whether a node should be recursed into
// (rendered as nested dotted-path rows, or a nested YAML object) rather than
// treated as a single leaf. Map-type objects (IsMap) are always leaves --
// their keys are arbitrary user-chosen names, not fixed schema properties to
// enumerate.
func (n *SchemaNode) IsObjectWithProperties() bool {
	return len(n.Properties) > 0 && !n.IsMap()
}

// SortedKeys returns m's keys in a stable, deterministic (alphabetical)
// order, so generated output (Markdown tables, template partials) doesn't
// churn from Go's randomized map iteration order between generation runs.
func SortedKeys(m map[string]*SchemaNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ToSet converts a slice (e.g. a JSON Schema "required" array) into a
// membership-testable set.
func ToSet(items []string) map[string]bool {
	set := make(map[string]bool, len(items))
	for _, i := range items {
		set[i] = true
	}
	return set
}
