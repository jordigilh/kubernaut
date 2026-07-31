// Package main tests for the Helm values.schema.json -> _generated_defaults.tpl
// generator (DD-PLATFORM-006 Decision Area 14, PR9). Uses Go's standard
// `testing` package rather than Ginkgo/Gomega: this is hack/-tier build
// tooling (a schema-to-template code generator invoked by `make
// generate-helm-defaults`), not pkg/ business logic in a production request
// path -- see AGENTS.md's Testing Requirements, which scope the Ginkgo/BDD
// mandate to business logic.
package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jordigilh/kubernaut/hack/internal/helmschema"
)

func mustParseSchema(t *testing.T, schema string) *helmschema.RootSchema {
	t.Helper()
	root, err := helmschema.ParseSchema([]byte(schema))
	if err != nil {
		t.Fatalf("ParseSchema failed: %v", err)
	}
	return root
}

// TestBuildDefaultsTreeCoversAllLeafTypes proves every leaf type the schema
// uses (bool/string/int/float/array) decodes into a correctly-typed Go value
// under its service+field nested path, not just a string echo of the raw
// JSON default text (which is what the docs generator's Row.Default does --
// the defaults generator needs the actual typed value, since it gets
// re-marshaled to YAML and consumed by Sprig's mergeOverwrite at Helm
// render time, where type matters).
func TestBuildDefaultsTreeCoversAllLeafTypes(t *testing.T) {
	root := mustParseSchema(t, `{
		"definitions": {},
		"properties": {
			"gateway": {
				"type": "object",
				"properties": {
					"replicas": {"type": "integer", "default": 1},
					"enabled": {"type": "boolean", "default": true},
					"logLevel": {"type": "string", "default": "info"},
					"cpuFactor": {"type": "number", "default": 1.5},
					"ingressNamespaces": {"type": "array", "default": [], "items": {"type": "string"}}
				}
			}
		}
	}`)

	tree := buildDefaultsTree(root)
	gw, ok := tree["gateway"].(map[string]interface{})
	if !ok {
		t.Fatalf("tree[\"gateway\"] = %#v (%T), want map[string]interface{}", tree["gateway"], tree["gateway"])
	}

	if got, want := gw["replicas"], float64(1); got != want {
		t.Errorf("gateway.replicas = %#v (%T), want %#v (json.Unmarshal decodes JSON numbers as float64)", got, got, want)
	}
	if got, want := gw["enabled"], true; got != want {
		t.Errorf("gateway.enabled = %#v (%T), want %#v", got, got, want)
	}
	if got, want := gw["logLevel"], "info"; got != want {
		t.Errorf("gateway.logLevel = %#v, want %#v", got, want)
	}
	if got, want := gw["cpuFactor"], 1.5; got != want {
		t.Errorf("gateway.cpuFactor = %#v, want %#v", got, want)
	}
	arr, ok := gw["ingressNamespaces"].([]interface{})
	if !ok || len(arr) != 0 {
		t.Errorf("gateway.ingressNamespaces = %#v, want an empty []interface{}", gw["ingressNamespaces"])
	}
}

// TestBuildDefaultsTreeRecursesNestedObjects proves a multi-level nested
// object (service.config.section.field) threads through to the correct
// nested map path, mirroring values.yaml's actual nesting depth (most of
// the 154 DA14 fields are 2-4 levels deep, e.g.
// apifrontend.config.mcp.enabled).
func TestBuildDefaultsTreeRecursesNestedObjects(t *testing.T) {
	root := mustParseSchema(t, `{
		"definitions": {},
		"properties": {
			"apifrontend": {
				"type": "object",
				"properties": {
					"config": {
						"type": "object",
						"properties": {
							"mcp": {
								"type": "object",
								"properties": {
									"enabled": {"type": "boolean", "default": true}
								}
							}
						}
					}
				}
			}
		}
	}`)

	tree := buildDefaultsTree(root)
	nested, err := digNested(tree, "apifrontend", "config", "mcp")
	if err != nil {
		t.Fatalf("%v (tree: %#v)", err, tree)
	}
	if got, want := nested["enabled"], true; got != want {
		t.Errorf("apifrontend.config.mcp.enabled = %#v, want %#v", got, want)
	}
}

// TestBuildDefaultsTreeSkipsMapTypeNodes proves a map-type node (arbitrary
// user-chosen keys, e.g. global.llmProfiles) is omitted entirely from the
// generated tree -- there are no fixed property names to emit a default
// for, and the node itself has no "default" of its own in this fixture.
func TestBuildDefaultsTreeSkipsMapTypeNodes(t *testing.T) {
	root := mustParseSchema(t, `{
		"definitions": {},
		"properties": {
			"global": {
				"type": "object",
				"properties": {
					"llmProfiles": {
						"type": "object",
						"additionalProperties": {"type": "object"}
					}
				}
			}
		}
	}`)

	tree := buildDefaultsTree(root)
	if _, present := tree["global"]; present {
		t.Errorf("tree[\"global\"] = %#v, want absent entirely (its only child is a map-type node with no default of its own)", tree["global"])
	}
}

// TestBuildDefaultsTreeSkipsFieldsWithNoDeclaredDefault proves a field with
// no "default" in the schema (e.g. a mandatory-to-supply field, or a
// freeform passthrough like resources.requests) is omitted from the
// generated tree rather than being emitted as a zero value -- emitting a
// fabricated zero value here would be actively harmful, since
// kubernaut.mergedValues would then "helpfully" supply e.g. replicas: 0 for
// a field that was deliberately left un-defaulted.
func TestBuildDefaultsTreeSkipsFieldsWithNoDeclaredDefault(t *testing.T) {
	root := mustParseSchema(t, `{
		"definitions": {},
		"properties": {
			"kubernautAgent": {
				"type": "object",
				"properties": {
					"llmProfileRef": {"type": "string", "default": "primary"},
					"apiKey": {"type": "string"}
				}
			}
		}
	}`)

	tree := buildDefaultsTree(root)
	ka, ok := tree["kubernautAgent"].(map[string]interface{})
	if !ok {
		t.Fatalf("tree[\"kubernautAgent\"] = %#v, want map[string]interface{}", tree["kubernautAgent"])
	}
	if _, present := ka["apiKey"]; present {
		t.Errorf("kubernautAgent.apiKey = %#v, want absent (no schema default declared)", ka["apiKey"])
	}
	if got, want := ka["llmProfileRef"], "primary"; got != want {
		t.Errorf("kubernautAgent.llmProfileRef = %#v, want %#v", got, want)
	}
}

// TestBuildDefaultsTreeResolvesRefAndAllOf proves the generator reuses
// helmschema's $ref/allOf resolution -- a field declared via
// `"allOf": [{"$ref": "#/definitions/goDuration"}]` (values.schema.json's
// pervasive pattern for shared types) still yields its own field-level
// default, not the definition's (which has none in this fixture).
func TestBuildDefaultsTreeResolvesRefAndAllOf(t *testing.T) {
	root := mustParseSchema(t, `{
		"definitions": {
			"goDuration": {"type": "string", "description": "Go time.Duration string"}
		},
		"properties": {
			"datastorage": {
				"type": "object",
				"properties": {
					"config": {
						"type": "object",
						"properties": {
							"database": {
								"type": "object",
								"properties": {
									"connMaxLifetime": {
										"allOf": [{"$ref": "#/definitions/goDuration"}],
										"default": "1h"
									}
								}
							}
						}
					}
				}
			}
		}
	}`)

	tree := buildDefaultsTree(root)
	nested, err := digNested(tree, "datastorage", "config", "database")
	if err != nil {
		t.Fatalf("%v (tree: %#v)", err, tree)
	}
	if got, want := nested["connMaxLifetime"], "1h"; got != want {
		t.Errorf("datastorage.config.database.connMaxLifetime = %#v, want %#v", got, want)
	}
}

// TestRenderTemplateProducesValidHelmDefine proves the rendered template
// text is wrapped in a single `kubernaut.defaults` named-template define
// block (the shape kubernaut.mergedValues expects to `include`+`fromYaml`),
// and that its YAML body actually contains the tree's data.
func TestRenderTemplateProducesValidHelmDefine(t *testing.T) {
	tree := map[string]interface{}{
		"gateway": map[string]interface{}{"replicas": float64(1)},
	}

	rendered, err := renderTemplate(tree)
	if err != nil {
		t.Fatalf("renderTemplate failed: %v", err)
	}
	if !strings.Contains(rendered, `{{- define "kubernaut.defaults" -}}`) {
		t.Errorf("rendered template missing the kubernaut.defaults define header:\n%s", rendered)
	}
	if !strings.HasSuffix(strings.TrimRight(rendered, "\n"), `{{- end -}}`) {
		t.Errorf("rendered template missing the closing {{- end -}}:\n%s", rendered)
	}
	if !strings.Contains(rendered, "gateway:") || !strings.Contains(rendered, "replicas: 1") {
		t.Errorf("rendered template body missing the expected YAML content:\n%s", rendered)
	}
}

// digNested walks a chain of map[string]interface{} keys, failing with a
// descriptive error at the first missing key or type mismatch, so test
// failures point at exactly which path segment broke.
func digNested(tree map[string]interface{}, path ...string) (map[string]interface{}, error) {
	cur := tree
	for i, p := range path {
		v, ok := cur[p]
		if !ok {
			return nil, fmt.Errorf("%s: not present", strings.Join(path[:i+1], "."))
		}
		next, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s: is %T, not a nested map", strings.Join(path[:i+1], "."), v)
		}
		cur = next
	}
	return cur, nil
}
