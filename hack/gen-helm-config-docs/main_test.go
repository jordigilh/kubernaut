// Package main tests for the Helm values.schema.json -> Markdown reference
// generator (DD-PLATFORM-006 Decision Area 4). Uses Go's standard `testing`
// package rather than Ginkgo/Gomega: this is hack/-tier build tooling (a
// schema-to-docs code generator invoked by `make generate-helm-config-docs`),
// not pkg/ business logic in a production request path -- see AGENTS.md's
// Testing Requirements, which scope the Ginkgo/BDD mandate to business logic.
package main

import (
	"strings"
	"testing"

	"github.com/jordigilh/kubernaut/hack/internal/helmschema"
)

// TestWalksNestedRefsAndDefinitions verifies a property that references a
// shared definition via `"allOf": [{"$ref": "#/definitions/X"}]` (the pattern
// values.schema.json uses throughout, e.g. goDuration/resourceQuantity)
// expands to the definition's type/description, with the property's own
// "default" overriding anything declared on the definition itself.
func TestWalksNestedRefsAndDefinitions(t *testing.T) {
	schema := []byte(`{
		"definitions": {
			"goDuration": {
				"type": "string",
				"description": "Go time.Duration string (e.g. \"30s\")"
			}
		},
		"properties": {
			"widget": {
				"type": "object",
				"properties": {
					"timeout": {
						"allOf": [{"$ref": "#/definitions/goDuration"}],
						"default": "30s"
					}
				}
			}
		}
	}`)

	root, err := helmschema.ParseSchema(schema)
	if err != nil {
		t.Fatalf("parseSchema failed: %v", err)
	}

	rows := walk("", root.Properties, nil, root.Definitions)
	row := findRow(t, rows, "widget.timeout")

	if row.Type != "string" {
		t.Errorf("Type = %q, want %q (expanded from #/definitions/goDuration)", row.Type, "string")
	}
	if row.Description != `Go time.Duration string (e.g. "30s")` {
		t.Errorf("Description = %q, want the definition's description to be inherited", row.Description)
	}
	if row.Default != `"30s"` {
		t.Errorf("Default = %q, want %q (the property's own default, not the definition's)", row.Default, `"30s"`)
	}
}

// TestMarksRequiredFields verifies fields listed in a parent object's
// "required" array are marked Required=true in the generated row, and
// siblings not listed are marked Required=false.
func TestMarksRequiredFields(t *testing.T) {
	schema := []byte(`{
		"definitions": {},
		"properties": {
			"ansible": {
				"type": "object",
				"required": ["apiURL"],
				"properties": {
					"apiURL": {
						"type": "string",
						"description": "Ansible AWX/Controller API URL"
					},
					"verifySSL": {
						"type": "boolean",
						"default": true
					}
				}
			}
		}
	}`)

	root, err := helmschema.ParseSchema(schema)
	if err != nil {
		t.Fatalf("parseSchema failed: %v", err)
	}

	rows := walk("", root.Properties, nil, root.Definitions)

	required := findRow(t, rows, "ansible.apiURL")
	if !required.Required {
		t.Errorf("ansible.apiURL: Required = false, want true (listed in parent's required array)")
	}

	optional := findRow(t, rows, "ansible.verifySSL")
	if optional.Required {
		t.Errorf("ansible.verifySSL: Required = true, want false (not listed in parent's required array)")
	}
}

// TestOneTablePerTopLevelService verifies the rendered Markdown emits one
// distinct "## <service>" section (with its own table) per top-level
// values.schema.json property, mirroring the README's existing per-service
// structure -- not a single flat table for the whole chart.
func TestOneTablePerTopLevelService(t *testing.T) {
	schema := []byte(`{
		"definitions": {},
		"properties": {
			"gateway": {
				"type": "object",
				"properties": {
					"replicas": {"type": "integer", "default": 1}
				}
			},
			"datastorage": {
				"type": "object",
				"properties": {
					"replicas": {"type": "integer", "default": 1}
				}
			}
		}
	}`)

	root, err := helmschema.ParseSchema(schema)
	if err != nil {
		t.Fatalf("parseSchema failed: %v", err)
	}

	md := GenerateMarkdown(root)

	if !strings.Contains(md, "## gateway") {
		t.Errorf("markdown missing a distinct \"## gateway\" section:\n%s", md)
	}
	if !strings.Contains(md, "## datastorage") {
		t.Errorf("markdown missing a distinct \"## datastorage\" section:\n%s", md)
	}
	gatewayIdx := strings.Index(md, "## gateway")
	datastorageIdx := strings.Index(md, "## datastorage")
	if gatewayIdx == datastorageIdx {
		t.Fatalf("expected two distinct section headers, got the same index")
	}
	// Each service's own replicas parameter must render under its own
	// section, not merged into one shared/ambiguous table.
	if strings.Count(md, "replicas") != 2 {
		t.Errorf("expected exactly 2 occurrences of \"replicas\" (one per service table), got %d:\n%s", strings.Count(md, "replicas"), md)
	}
}

func findRow(t *testing.T, rows []Row, parameter string) Row {
	t.Helper()
	for _, r := range rows {
		if r.Parameter == parameter {
			return r
		}
	}
	t.Fatalf("no row found for parameter %q (rows: %+v)", parameter, rows)
	return Row{}
}
