package config_test

// TC-P2C-04: Helm chart schema-default drift detection.
//
// Fast-follow from PR #2225 review (issue #2226, BR-PLATFORM-011): for a
// field with a real Go-side default AND a Helm-schema-declared default
// (mcp.sessionIdleTimeout/toolTimeout/toolTimeouts), those two
// independently-authored literals -- config.DefaultConfig()'s Go value and
// charts/kubernaut/values.schema.json's "default" -- are kept in sync only
// by `make generate-helm-defaults` + code review, with no automated
// cross-check. That is structurally the same drift class that caused
// kubernaut-operator#374 (a hand-maintained copy of this exact
// ToolTimeouts map silently fell 2-of-4 entries out of date). Unlike #374,
// deleting the schema-side copy isn't the right fix here: issue #2221
// deliberately made these fields user-overridable via values.yaml, so the
// schema's own default has to exist for kubernaut.mergedValues to
// materialize something when the user doesn't override it.
//
// Rather than one narrow, hand-written assertion per field (the shape
// TC-P2C-03d used for sessionIdleTimeout alone), this is a small
// table-driven guard: adding a new dual-sourced default in the future only
// requires a new Entry() row, not a new bespoke parsing/comparison test.
//
// This intentionally does NOT reuse hack/internal/helmschema: that package
// is Go-`internal`-scoped to hack/ (its own parent directory), so pkg/
// cannot import it -- and pkg/ business/test code importing hack/-tier
// build tooling would blur a real, load-bearing repo boundary anyway. The
// resolution this test needs (walk a dotted path through nested
// "properties", read a leaf's own "default") is a handful of lines against
// the raw parsed JSON, so it's inlined here rather than restructuring
// shared tooling to serve one caller.
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
)

// repoRootFromTest walks up from this source file's own directory looking
// for go.mod, so the schema path resolves correctly regardless of the
// working directory `go test` is invoked from.
func repoRootFromTest() (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("runtime.Caller failed to resolve this test file's own path")
	}
	dir := filepath.Dir(currentFile)
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("go.mod not found walking up from %s", currentFile)
}

// loadValuesSchema reads and json-decodes the repo's real
// charts/kubernaut/values.schema.json into a generic tree, so this test
// fails the moment the schema's declared default diverges from the Go
// value, not just when someone remembers to write a matching assertion.
func loadValuesSchema() (map[string]interface{}, error) {
	root, err := repoRootFromTest()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(root, "charts", "kubernaut", "values.schema.json"))
	if err != nil {
		return nil, fmt.Errorf("reading values.schema.json: %w", err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing values.schema.json: %w", err)
	}
	return doc, nil
}

// resolveSchemaNode walks a dotted path (e.g.
// "apifrontend.config.mcp.sessionIdleTimeout") through nested
// values.schema.json "properties" objects and returns the leaf node's own
// raw JSON object.
func resolveSchemaNode(doc map[string]interface{}, dottedPath string) (map[string]interface{}, error) {
	props, ok := doc["properties"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("%s: schema root has no top-level \"properties\"", dottedPath)
	}
	segments := strings.Split(dottedPath, ".")
	cur := props
	var node map[string]interface{}
	for i, seg := range segments {
		raw, ok := cur[seg]
		if !ok {
			return nil, fmt.Errorf("%s: no schema property %q at segment %d", dottedPath, seg, i)
		}
		node, ok = raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s: schema property %q is not an object", dottedPath, seg)
		}
		if i == len(segments)-1 {
			break
		}
		nextProps, ok := node["properties"].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%s: schema property %q has no nested \"properties\" to continue resolving %q", dottedPath, seg, segments[i+1])
		}
		cur = nextProps
	}
	return node, nil
}

// schemaDefaultDuration resolves dottedPath and parses its declared
// "default" as a Go duration.
func schemaDefaultDuration(doc map[string]interface{}, dottedPath string) (time.Duration, error) {
	node, err := resolveSchemaNode(doc, dottedPath)
	if err != nil {
		return 0, err
	}
	raw, ok := node["default"]
	if !ok {
		return 0, fmt.Errorf("%s: schema declares no \"default\"", dottedPath)
	}
	s, ok := raw.(string)
	if !ok {
		return 0, fmt.Errorf("%s: schema \"default\" is %T (%v), not a string", dottedPath, raw, raw)
	}
	return time.ParseDuration(s)
}

var _ = Describe("TC-P2C-04: Helm chart schema-default drift detection (fast-follow from PR #2225 review, BR-PLATFORM-011)", func() {
	var schemaDoc map[string]interface{}

	BeforeEach(func() {
		doc, err := loadValuesSchema()
		Expect(err).NotTo(HaveOccurred(), "failed to load charts/kubernaut/values.schema.json")
		schemaDoc = doc
	})

	DescribeTable("config.DefaultConfig()'s dual-sourced MCP duration fields match values.schema.json's declared default exactly",
		func(schemaPath string, goValue time.Duration) {
			schemaValue, err := schemaDefaultDuration(schemaDoc, schemaPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(goValue).To(Equal(schemaValue),
				"config.DefaultConfig() (%s) and values.schema.json's %q default (%s) have drifted apart -- "+
					"update whichever side is stale, or run `make generate-helm-defaults` if only the "+
					"materialized template is stale", goValue, schemaPath, schemaValue)
		},
		Entry("mcp.sessionIdleTimeout (#2220)", "apifrontend.config.mcp.sessionIdleTimeout", config.DefaultConfig().MCP.SessionIdleTimeout),
		Entry("mcp.toolTimeout (#2221)", "apifrontend.config.mcp.toolTimeout", config.DefaultConfig().MCP.ToolTimeout),
		Entry("mcp.toolTimeouts.kubernaut_investigate (#2221)", "apifrontend.config.mcp.toolTimeouts.kubernaut_investigate", config.DefaultConfig().MCP.ToolTimeouts["kubernaut_investigate"]),
		Entry("mcp.toolTimeouts.kubernaut_await_session (#2221)", "apifrontend.config.mcp.toolTimeouts.kubernaut_await_session", config.DefaultConfig().MCP.ToolTimeouts["kubernaut_await_session"]),
		Entry("mcp.toolTimeouts.kubernaut_watch (#2221)", "apifrontend.config.mcp.toolTimeouts.kubernaut_watch", config.DefaultConfig().MCP.ToolTimeouts["kubernaut_watch"]),
		Entry("mcp.toolTimeouts.kubernaut_discover_workflows (#2221)", "apifrontend.config.mcp.toolTimeouts.kubernaut_discover_workflows", config.DefaultConfig().MCP.ToolTimeouts["kubernaut_discover_workflows"]),
	)
})
