// Package main tests for the Helm chart config-knob test-coverage gate
// (BR-PLATFORM-011, issue #2226). Uses Go's standard `testing` package
// rather than Ginkgo/Gomega: this is hack/-tier build tooling (a CI merge
// gate invoked by `make check-helm-coverage`), not pkg/ business logic in a
// production request path -- see AGENTS.md's Testing Requirements, which
// scope the Ginkgo/BDD mandate to business logic (same precedent as
// hack/gen-helm-defaults and hack/gen-helm-config-docs).
package main

import (
	"os"
	"path/filepath"
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

func writeTestFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writing fixture %s: %v", name, err)
	}
}

func leafPaths(leaves []Leaf) map[string]bool {
	set := make(map[string]bool, len(leaves))
	for _, l := range leaves {
		set[l.FullPath()] = true
	}
	return set
}

// TestCollectSchemaLeavesRecursesNestedObjects proves a multi-level nested
// scalar leaf (service.config.section.field) is collected with its full
// dotted path and correct bare name, mirroring values.schema.json's actual
// nesting depth.
func TestCollectSchemaLeavesRecursesNestedObjects(t *testing.T) {
	root := mustParseSchema(t, `{
		"definitions": {},
		"properties": {
			"gateway": {
				"type": "object",
				"properties": {
					"config": {
						"type": "object",
						"properties": {
							"server": {
								"type": "object",
								"properties": {
									"readTimeout": {"type": "string", "default": "30s"}
								}
							}
						}
					}
				}
			}
		}
	}`)

	leaves := CollectSchemaLeaves(root)
	paths := leafPaths(leaves)
	if !paths["gateway.config.server.readTimeout"] {
		t.Fatalf("CollectSchemaLeaves() = %v, want it to contain gateway.config.server.readTimeout", paths)
	}
	for _, l := range leaves {
		if l.FullPath() == "gateway.config.server.readTimeout" {
			if got, want := l.BareName(), "readTimeout"; got != want {
				t.Errorf("BareName() = %q, want %q", got, want)
			}
		}
	}
}

// TestCollectSchemaLeavesSkipsFieldsWithNoDeclaredDefault proves a scalar
// field with no schema "default" (a mandatory-to-supply field, or a
// freeform passthrough) is not registered as a checkable leaf -- there is
// no materialized default value whose wiring could be proven anyway.
func TestCollectSchemaLeavesSkipsFieldsWithNoDeclaredDefault(t *testing.T) {
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

	paths := leafPaths(CollectSchemaLeaves(root))
	if paths["kubernautAgent.apiKey"] {
		t.Errorf("CollectSchemaLeaves() = %v, want kubernautAgent.apiKey absent (no schema default declared)", paths)
	}
	if !paths["kubernautAgent.llmProfileRef"] {
		t.Errorf("CollectSchemaLeaves() = %v, want kubernautAgent.llmProfileRef present", paths)
	}
}

// TestCollectSchemaLeavesRegistersMapTypeNodesAsTheirOwnLeaf proves a
// genuine arbitrary-key map node (additionalProperties is a nested schema,
// e.g. global.llmProfiles) is itself registered as one checkable leaf --
// unlike hack/gen-helm-defaults's walkDefaults, which skips these entirely
// since there's nothing to materialize a static default for. Coverage
// checking is a different question ("is this field's wiring tested?"), so
// the map field itself must still be checkable.
func TestCollectSchemaLeavesRegistersMapTypeNodesAsTheirOwnLeaf(t *testing.T) {
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

	paths := leafPaths(CollectSchemaLeaves(root))
	if !paths["global.llmProfiles"] {
		t.Errorf("CollectSchemaLeaves() = %v, want global.llmProfiles present as its own leaf", paths)
	}
}

// TestCollectSchemaLeavesResolvesRefAndAllOf proves the walk reuses
// helmschema's $ref/allOf resolution -- a field declared via
// `"allOf": [{"$ref": "#/definitions/goDuration"}]` still yields its own
// field-level default and is registered as a leaf.
func TestCollectSchemaLeavesResolvesRefAndAllOf(t *testing.T) {
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

	paths := leafPaths(CollectSchemaLeaves(root))
	if !paths["datastorage.config.database.connMaxLifetime"] {
		t.Errorf("CollectSchemaLeaves() = %v, want datastorage.config.database.connMaxLifetime present", paths)
	}
}

// TestBuildAssertionCorpusCommentOnlyMentionDoesNotCount is the spike's key
// regression fixture: a suite/it doc comment mentioning a field name (e.g.
// "readTimeout/writeTimeout are schema-driven") must NOT count as coverage,
// since YAML comments never survive parsing into the corpus and the field
// name never appears inside a real `asserts[]` entry.
func TestBuildAssertionCorpusCommentOnlyMentionDoesNotCount(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "gateway_test.yaml", `suite: "gateway server timeouts"
# gateway.config.server.readTimeout/writeTimeout are schema-driven, see
# templates/gateway/gateway.yaml.
templates:
  - templates/gateway/gateway.yaml
tests:
  - it: "renders idleTimeout"
    asserts:
      - matchRegex:
          path: data['config.yaml']
          pattern: 'idleTimeout: 60s'
`)

	corpus, err := BuildAssertionCorpus(dir)
	if err != nil {
		t.Fatalf("BuildAssertionCorpus failed: %v", err)
	}
	if IsCovered("readTimeout", corpus["gateway"]) {
		t.Errorf("IsCovered(readTimeout) = true, want false (only mentioned in a comment, never asserted)")
	}
	if !IsCovered("idleTimeout", corpus["gateway"]) {
		t.Errorf("IsCovered(idleTimeout) = false, want true (asserted via matchRegex pattern)")
	}
}

// TestBuildAssertionCorpusRealAssertionCounts proves the converse: a field
// name that genuinely appears inside a real `asserts[]` pattern/value IS
// counted as covered.
func TestBuildAssertionCorpusRealAssertionCounts(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "datastorage_test.yaml", `suite: "datastorage connection pool"
templates:
  - templates/datastorage/datastorage.yaml
tests:
  - it: "default install renders connMaxLifetime"
    asserts:
      - matchRegex:
          path: data['config.yaml']
          pattern: 'connMaxLifetime: 1h'
`)

	corpus, err := BuildAssertionCorpus(dir)
	if err != nil {
		t.Fatalf("BuildAssertionCorpus failed: %v", err)
	}
	if !IsCovered("connMaxLifetime", corpus["datastorage"]) {
		t.Errorf("IsCovered(connMaxLifetime) = false, want true (asserted via matchRegex pattern)")
	}
}

// TestBuildAssertionCorpusCommonWordFalsePositiveAvoided is the spike's
// second regression fixture:
// remediationorchestrator.config.timeouts.global must not be considered
// covered merely because the unrelated word "global" appears in another
// service's `set:` override block (e.g. every suite's `global.fleet...`
// stanza) -- `set:` keys are outside `tests[].asserts[]` and must never be
// walked into the corpus at all.
func TestBuildAssertionCorpusCommonWordFalsePositiveAvoided(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "ro_test.yaml", `suite: "remediationorchestrator timeouts"
templates:
  - templates/remediationorchestrator/remediationorchestrator.yaml
set:
  global:
    fleet:
      enabled: false
tests:
  - it: "renders processing timeout"
    asserts:
      - matchRegex:
          path: data['config.yaml']
          pattern: 'processing: 5m'
`)

	corpus, err := BuildAssertionCorpus(dir)
	if err != nil {
		t.Fatalf("BuildAssertionCorpus failed: %v", err)
	}
	if IsCovered("global", corpus["remediationorchestrator"]) {
		t.Errorf("IsCovered(global) = true, want false (\"global\" only appears in the suite's set: block, never in an assert)")
	}
	if !IsCovered("processing", corpus["remediationorchestrator"]) {
		t.Errorf("IsCovered(processing) = false, want true (asserted via matchRegex pattern)")
	}
}

// TestBuildAssertionCorpusServiceDisambiguation proves identically-named
// leaves across two different services (readTimeout exists under both
// datastorage.config.server and gateway.config.server) are scoped to their
// owning suite's own service, not conflated across the whole corpus: a
// suite that only renders gateway's template must not make datastorage's
// readTimeout appear covered, and vice versa.
func TestBuildAssertionCorpusServiceDisambiguation(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "gateway_test.yaml", `suite: "gateway readTimeout"
templates:
  - templates/gateway/gateway.yaml
tests:
  - it: "renders readTimeout"
    asserts:
      - matchRegex:
          path: data['config.yaml']
          pattern: 'readTimeout: 30s'
`)

	corpus, err := BuildAssertionCorpus(dir)
	if err != nil {
		t.Fatalf("BuildAssertionCorpus failed: %v", err)
	}
	if !IsCovered("readTimeout", corpus["gateway"]) {
		t.Errorf("IsCovered(readTimeout, gateway) = false, want true")
	}
	if IsCovered("readTimeout", corpus["datastorage"]) {
		t.Errorf("IsCovered(readTimeout, datastorage) = true, want false (no datastorage suite exists in this fixture)")
	}
}

// TestBuildAssertionCorpusWholeWordMatch proves the bare-name match respects
// word boundaries: a suite asserting "toolTimeouts" (the map field) must
// not be mistaken for coverage of the distinct, shorter sibling field
// "toolTimeout" -- and vice versa, a suite asserting only "toolTimeout"
// must not count as covering "toolTimeouts".
func TestBuildAssertionCorpusWholeWordMatch(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "apifrontend_test.yaml", `suite: "apifrontend mcp toolTimeouts"
templates:
  - templates/apifrontend/apifrontend.yaml
tests:
  - it: "renders toolTimeouts entries"
    asserts:
      - matchRegex:
          path: data['config.yaml']
          pattern: 'toolTimeouts:'
      - matchRegex:
          path: data['config.yaml']
          pattern: 'kubernaut_investigate: "15m"'
`)

	corpus, err := BuildAssertionCorpus(dir)
	if err != nil {
		t.Fatalf("BuildAssertionCorpus failed: %v", err)
	}
	if !IsCovered("toolTimeouts", corpus["apifrontend"]) {
		t.Errorf("IsCovered(toolTimeouts) = false, want true (asserted directly)")
	}
	if IsCovered("toolTimeout", corpus["apifrontend"]) {
		t.Errorf("IsCovered(toolTimeout) = true, want false (fixture only asserts the plural \"toolTimeouts:\", never the bare \"toolTimeout\" field on its own)")
	}
}

// TestComputeCoverageAllowlist proves the allowlist mechanism: an uncovered
// leaf present in the allowlist is not reported as a gap, but an uncovered
// leaf absent from the allowlist is.
func TestComputeCoverageAllowlist(t *testing.T) {
	leaves := []Leaf{
		{Service: "gateway", Path: "replicas"},
		{Service: "gateway", Path: "config.server.readTimeout"},
	}
	corpus := map[string][]string{}
	allow := map[string]bool{"gateway.replicas": true}

	gaps := ComputeGaps(leaves, corpus)
	failures := FilterAllowlisted(gaps, allow)

	if len(gaps) != 2 {
		t.Fatalf("ComputeGaps() = %v, want 2 entries (both leaves uncovered)", gaps)
	}
	if len(failures) != 1 || failures[0].FullPath() != "gateway.config.server.readTimeout" {
		t.Errorf("FilterAllowlisted() = %v, want exactly [gateway.config.server.readTimeout]", failures)
	}
}

// TestLoadAllowlistMissingFileIsEmpty proves a missing allowlist file is
// treated as an empty allowlist (not an error) -- relevant for first-ever
// runs before the file is seeded.
func TestLoadAllowlistMissingFileIsEmpty(t *testing.T) {
	allow, err := LoadAllowlist(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("LoadAllowlist() error = %v, want nil for a missing file", err)
	}
	if len(allow) != 0 {
		t.Errorf("LoadAllowlist() = %v, want empty", allow)
	}
}

// TestServiceFromTemplatePath proves the owning-service extraction: the
// first path segment after "templates/".
func TestServiceFromTemplatePath(t *testing.T) {
	cases := map[string]string{
		"templates/datastorage/datastorage.yaml":     "datastorage",
		"templates/gateway/gateway.yaml":              "gateway",
		"templates/remediationorchestrator/remediationorchestrator.yaml": "remediationorchestrator",
	}
	for input, want := range cases {
		if got := serviceFromTemplatePath(input); got != want {
			t.Errorf("serviceFromTemplatePath(%q) = %q, want %q", input, got, want)
		}
	}
}
