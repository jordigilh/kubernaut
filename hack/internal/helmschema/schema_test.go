// Package helmschema tests. Uses Go's standard `testing` package rather than
// Ginkgo/Gomega: this is hack/-tier build tooling shared by two schema-driven
// generators, not pkg/ business logic in a production request path -- see
// AGENTS.md's Testing Requirements, which scope the Ginkgo/BDD mandate to
// business logic.
package helmschema

import "testing"

func TestParseSchemaDefaultsDefinitionsToEmptyMap(t *testing.T) {
	root, err := ParseSchema([]byte(`{"properties": {"gateway": {"type": "object"}}}`))
	if err != nil {
		t.Fatalf("ParseSchema failed: %v", err)
	}
	if root.Definitions == nil {
		t.Fatal("Definitions = nil, want an empty (non-nil) map when the schema omits \"definitions\"")
	}
}

func TestResolveNodeExpandsRefWithFieldLevelDefaultOverride(t *testing.T) {
	root, err := ParseSchema([]byte(`{
		"definitions": {
			"goDuration": {"type": "string", "description": "Go time.Duration string"}
		},
		"properties": {
			"timeout": {"allOf": [{"$ref": "#/definitions/goDuration"}], "default": "30s"}
		}
	}`))
	if err != nil {
		t.Fatalf("ParseSchema failed: %v", err)
	}
	node := ResolveNode(root.Properties["timeout"], root.Definitions)
	if node.Description != "Go time.Duration string" {
		t.Errorf("Description = %q, want the referenced definition's description", node.Description)
	}
	if node.DefaultJSON() != `"30s"` {
		t.Errorf("DefaultJSON() = %q, want %q (the property's own default, not the definition's, which has none)", node.DefaultJSON(), `"30s"`)
	}
}

func TestResolveNodeNilInputReturnsNil(t *testing.T) {
	if got := ResolveNode(nil, map[string]*SchemaNode{}); got != nil {
		t.Errorf("ResolveNode(nil, ...) = %+v, want nil", got)
	}
}

func TestIsMapDistinguishesSchemaObjectFromBooleanForm(t *testing.T) {
	mapNode := &SchemaNode{AdditionalProperties: []byte(`{"type": "object"}`)}
	if !mapNode.IsMap() {
		t.Error("IsMap() = false for a nested-schema additionalProperties, want true")
	}
	boolNode := &SchemaNode{AdditionalProperties: []byte(`false`)}
	if boolNode.IsMap() {
		t.Error("IsMap() = true for a boolean additionalProperties, want false")
	}
	noneNode := &SchemaNode{}
	if noneNode.IsMap() {
		t.Error("IsMap() = true when additionalProperties is absent entirely, want false")
	}
}

func TestHasDefaultAndDefaultJSON(t *testing.T) {
	withDefault := &SchemaNode{Default: []byte(`true`)}
	if !withDefault.HasDefault() {
		t.Error("HasDefault() = false, want true when \"default\" is declared")
	}
	if withDefault.DefaultJSON() != "true" {
		t.Errorf("DefaultJSON() = %q, want %q", withDefault.DefaultJSON(), "true")
	}

	withoutDefault := &SchemaNode{}
	if withoutDefault.HasDefault() {
		t.Error("HasDefault() = true, want false when \"default\" is absent")
	}
	if withoutDefault.DefaultJSON() != "" {
		t.Errorf("DefaultJSON() = %q, want empty string when \"default\" is absent", withoutDefault.DefaultJSON())
	}
}

func TestIsObjectWithPropertiesExcludesMapType(t *testing.T) {
	obj := &SchemaNode{Properties: map[string]*SchemaNode{"replicas": {}}}
	if !obj.IsObjectWithProperties() {
		t.Error("IsObjectWithProperties() = false for a node with fixed properties, want true")
	}

	mapType := &SchemaNode{
		Properties:           map[string]*SchemaNode{"placeholder": {}},
		AdditionalProperties: []byte(`{"type": "object"}`),
	}
	if mapType.IsObjectWithProperties() {
		t.Error("IsObjectWithProperties() = true for a map-type node, want false (arbitrary keys are leaves, not fixed properties to recurse into)")
	}
}

func TestSortedKeysIsDeterministic(t *testing.T) {
	m := map[string]*SchemaNode{"zeta": {}, "alpha": {}, "mid": {}}
	got := SortedKeys(m)
	want := []string{"alpha", "mid", "zeta"}
	if len(got) != len(want) {
		t.Fatalf("SortedKeys() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("SortedKeys()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestToSetMembership(t *testing.T) {
	set := ToSet([]string{"a", "b"})
	if !set["a"] || !set["b"] {
		t.Errorf("ToSet([\"a\",\"b\"]) = %v, want both keys present", set)
	}
	if set["c"] {
		t.Errorf("ToSet([\"a\",\"b\"])[\"c\"] = true, want false")
	}
}
