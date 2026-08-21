// Command check-helm-coverage is a CI merge gate (BR-PLATFORM-011, issue
// #2226) that proves every schema-defaulted/map leaf field in
// charts/kubernaut/values.schema.json has at least one real helm-unittest
// assertion proving it is actually wired into a rendered manifest -- not
// merely accepted by schema validation (that is BR-PLATFORM-010's separate
// concern).
//
// A naive "does the leaf name appear anywhere in tests/*.yaml text"
// heuristic is unreliable in both directions (see the BR doc for the full
// triage): a doc comment mentioning a field name would count as false
// coverage, an unrelated common word (e.g. "global") would too, and
// identically-named leaves across two different services (e.g.
// "readTimeout" under both datastorage.config.server and
// gateway.config.server) can't be told apart. This tool instead:
//
//  1. Parses values.schema.json structurally (reusing hack/internal/helmschema,
//     the same core hack/gen-helm-defaults and hack/gen-helm-config-docs
//     already share) into per-service checkable leaves.
//  2. Parses every charts/kubernaut/tests/*.yaml suite as structured YAML
//     (comments disappear for free), resolves each suite's `templates:`
//     entries to an owning service, and collects only the real string
//     content found under `tests[].asserts[]` into that service's
//     assertion corpus -- never `it:`/`suite:` descriptions or `set:` keys.
//  3. Marks a leaf covered if its bare key name appears as a whole-word
//     match in its own service's corpus.
//  4. Allows a seeded, reviewable exception list
//     (charts/kubernaut/.helm-coverage-allowlist.yaml) for already-known,
//     lower-priority gaps, so the gate only blocks *new* regressions.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jordigilh/kubernaut/hack/internal/helmschema"
	"gopkg.in/yaml.v3"
)

// Leaf is one checkable config-knob field: a schema-declared leaf (scalar
// with a default, or a map-type node) belonging to a top-level service.
type Leaf struct {
	Service string
	// Path is the dotted path within Service (e.g. "config.server.readTimeout"),
	// or "" for a bare top-level scalar service property (e.g. "nameOverride").
	Path string
}

// FullPath renders the leaf's complete dotted path including its owning
// service, e.g. "gateway.config.server.readTimeout". This is the identity
// used both for allowlist entries and for gate failure reporting.
func (l Leaf) FullPath() string {
	if l.Path == "" {
		return l.Service
	}
	return l.Service + "." + l.Path
}

// BareName returns the leaf's own key name (the last dotted segment),
// which is what must appear as a whole-word match in the owning service's
// assertion corpus to count as covered.
func (l Leaf) BareName() string {
	if l.Path == "" {
		return l.Service
	}
	if idx := strings.LastIndex(l.Path, "."); idx >= 0 {
		return l.Path[idx+1:]
	}
	return l.Path
}

// CollectSchemaLeaves walks every top-level values.schema.json property
// (service) into its checkable leaves.
func CollectSchemaLeaves(root *helmschema.RootSchema) []Leaf {
	var leaves []Leaf
	for _, service := range helmschema.SortedKeys(root.Properties) {
		node := helmschema.ResolveNode(root.Properties[service], root.Definitions)
		leaves = append(leaves, walkLeaves(service, "", node, root.Definitions)...)
	}
	return leaves
}

// walkLeaves recurses into a resolved schema node, registering:
//   - a map-type node (additionalProperties is a nested schema, e.g.
//     global.llmProfiles) as one leaf in its own right -- its arbitrary
//     keys aren't fixed schema properties to enumerate, but the field's
//     own wiring is still a checkable unit, unlike hack/gen-helm-defaults's
//     walkDefaults, which skips these entirely since it only cares about
//     materializable static defaults.
//   - a regular object with fixed properties by recursing into each one.
//   - a scalar/array leaf that declares a schema "default" -- fields with
//     no declared default have nothing to prove ("this default renders
//     through") and are intentionally excluded.
func walkLeaves(service, prefix string, node *helmschema.SchemaNode, defs map[string]*helmschema.SchemaNode) []Leaf {
	if node == nil {
		return nil
	}
	if node.IsMap() {
		return []Leaf{{Service: service, Path: prefix}}
	}
	if node.IsObjectWithProperties() {
		var leaves []Leaf
		for _, name := range helmschema.SortedKeys(node.Properties) {
			childPath := name
			if prefix != "" {
				childPath = prefix + "." + name
			}
			child := helmschema.ResolveNode(node.Properties[name], defs)
			leaves = append(leaves, walkLeaves(service, childPath, child, defs)...)
		}
		return leaves
	}
	if node.HasDefault() {
		return []Leaf{{Service: service, Path: prefix}}
	}
	return nil
}

// rawTestFile mirrors the subset of a charts/kubernaut/tests/*.yaml
// helm-unittest suite this tool needs: which templates (services) it
// renders, and its tests' assertion content. `set:`/`it:`/`documentSelector:`
// are deliberately not modeled -- they must never contribute to the
// assertion corpus.
type rawTestFile struct {
	Templates []string      `yaml:"templates"`
	Tests     []rawTestCase `yaml:"tests"`
}

type rawTestCase struct {
	Asserts []interface{} `yaml:"asserts"`
}

// serviceFromTemplatePath extracts the owning service name from a
// `templates:` entry: the first path segment after "templates/", e.g.
// "templates/datastorage/datastorage.yaml" -> "datastorage".
func serviceFromTemplatePath(p string) string {
	const prefix = "templates/"
	trimmed := strings.TrimPrefix(p, prefix)
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		return trimmed[:idx]
	}
	return trimmed
}

// collectStrings recursively walks an arbitrary decoded-YAML value
// (map[string]interface{}/[]interface{}/scalar, as produced by yaml.v3
// unmarshalling into interface{}) and appends every string scalar it finds
// to out, regardless of which key it was found under (pattern/value/path,
// whatever helm-unittest assertion shape uses) -- comments are already gone
// by construction once YAML is parsed, and callers only ever pass in
// `asserts[]` content, never `it:`/`suite:`/`set:` blocks.
func collectStrings(v interface{}, out *[]string) {
	switch t := v.(type) {
	case string:
		*out = append(*out, t)
	case map[string]interface{}:
		for _, key := range t {
			collectStrings(key, out)
		}
	case []interface{}:
		for _, item := range t {
			collectStrings(item, out)
		}
	}
}

// BuildAssertionCorpus parses every charts/kubernaut/tests/*.yaml suite in
// dir and returns, per owning service, the flat list of every string found
// under any of that suite's `tests[].asserts[]` entries.
func BuildAssertionCorpus(dir string) (map[string][]string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("globbing %s: %w", dir, err)
	}
	corpus := make(map[string][]string)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		var tf rawTestFile
		if err := yaml.Unmarshal(data, &tf); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}

		services := make(map[string]bool, len(tf.Templates))
		for _, tmpl := range tf.Templates {
			services[serviceFromTemplatePath(tmpl)] = true
		}

		var strs []string
		for _, tc := range tf.Tests {
			for _, a := range tc.Asserts {
				collectStrings(a, &strs)
			}
		}
		for svc := range services {
			corpus[svc] = append(corpus[svc], strs...)
		}
	}
	return corpus, nil
}

// IsCovered reports whether name appears as a whole-word match anywhere in
// corpus (one service's flat assertion-string list).
func IsCovered(name string, corpus []string) bool {
	pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(name) + `\b`)
	for _, s := range corpus {
		if pattern.MatchString(s) {
			return true
		}
	}
	return false
}

// ComputeGaps returns the subset of leaves with no coverage in corpus,
// i.e. every field that is schema-declared but not proven to be wired by
// any real helm-unittest assertion.
func ComputeGaps(leaves []Leaf, corpus map[string][]string) []Leaf {
	var gaps []Leaf
	for _, l := range leaves {
		if !IsCovered(l.BareName(), corpus[l.Service]) {
			gaps = append(gaps, l)
		}
	}
	return gaps
}

// FilterAllowlisted returns the subset of gaps not present in allow --
// i.e. the actual gate failures.
func FilterAllowlisted(gaps []Leaf, allow map[string]bool) []Leaf {
	var failures []Leaf
	for _, g := range gaps {
		if !allow[g.FullPath()] {
			failures = append(failures, g)
		}
	}
	return failures
}

// LoadAllowlist reads a flat YAML list of dotted-path strings from path. A
// missing file is treated as an empty allowlist (not an error), so the
// gate can run before the file is first seeded.
func LoadAllowlist(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("reading allowlist %s: %w", path, err)
	}
	var list []string
	if err := yaml.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("parsing allowlist %s: %w", path, err)
	}
	set := make(map[string]bool, len(list))
	for _, p := range list {
		set[p] = true
	}
	return set, nil
}

// writeAllowlist renders gaps as a sorted, deduplicated YAML list with a
// header comment, overwriting path. Used by -write-baseline to (re-)seed
// the allowlist from the current gap set.
func writeAllowlist(path string, gaps []Leaf) error {
	sortedPaths := make([]string, len(gaps))
	for i, g := range gaps {
		sortedPaths[i] = g.FullPath()
	}
	sort.Strings(sortedPaths)

	var sb strings.Builder
	sb.WriteString("# Seeded/regenerated by " +
		"`go run ./hack/check-helm-coverage -write-baseline` " +
		"(BR-PLATFORM-011, issue #2226).\n" +
		"# Each entry is a values.schema.json leaf field with a schema default (or a\n" +
		"# map-type field) that has no helm-unittest assertion proving it renders.\n" +
		"# Entries here are accepted, lower-priority gaps -- do not add a new entry\n" +
		"# to silence a gap on a field you just introduced; write the test instead.\n")
	for _, p := range sortedPaths {
		sb.WriteString("- " + p + "\n")
	}
	return os.WriteFile(path, []byte(sb.String()), 0o644)
}

func main() {
	schemaPath := flag.String("schema", "charts/kubernaut/values.schema.json", "path to values.schema.json")
	testsDir := flag.String("tests", "charts/kubernaut/tests", "path to the helm-unittest suite directory")
	allowlistPath := flag.String("allowlist", "charts/kubernaut/.helm-coverage-allowlist.yaml", "path to the coverage allowlist")
	writeBaseline := flag.Bool("write-baseline", false, "regenerate the allowlist from the current gap set instead of gating")
	flag.Parse()

	schemaData, err := os.ReadFile(*schemaPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-helm-coverage: reading schema: %v\n", err)
		os.Exit(1)
	}
	root, err := helmschema.ParseSchema(schemaData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-helm-coverage: %v\n", err)
		os.Exit(1)
	}
	leaves := CollectSchemaLeaves(root)

	corpus, err := BuildAssertionCorpus(*testsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-helm-coverage: %v\n", err)
		os.Exit(1)
	}

	gaps := ComputeGaps(leaves, corpus)

	if *writeBaseline {
		if err := writeAllowlist(*allowlistPath, gaps); err != nil {
			fmt.Fprintf(os.Stderr, "check-helm-coverage: writing baseline: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("check-helm-coverage: wrote baseline with %d entries to %s\n", len(gaps), *allowlistPath)
		return
	}

	allow, err := LoadAllowlist(*allowlistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-helm-coverage: %v\n", err)
		os.Exit(1)
	}
	failures := FilterAllowlisted(gaps, allow)

	if len(failures) > 0 {
		sort.Slice(failures, func(i, j int) bool { return failures[i].FullPath() < failures[j].FullPath() })
		for _, f := range failures {
			fmt.Fprintf(os.Stderr,
				"::error::check-helm-coverage: %s has a schema default/map field but no helm-unittest assertion proves it renders, and it is not in %s\n",
				f.FullPath(), *allowlistPath)
		}
		fmt.Fprintf(os.Stderr, "check-helm-coverage: %d uncovered, unallowlisted config field(s) found (%d total leaves, %d total gaps)\n",
			len(failures), len(leaves), len(gaps))
		os.Exit(1)
	}
	fmt.Printf("check-helm-coverage: OK (%d schema leaves, %d gaps all allowlisted)\n", len(leaves), len(gaps))
}
