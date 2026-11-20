/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SourcesConfig selects where FMC collects fleet inventory from
// (DD-FLEET-017 D4.1a). The operator renders this block; FMC receives
// resolved artifact paths only, never policy.
//
// `declared` records that a valid block was present, so presence can never
// masquerade as absence and silently fall back to the MCP collector
// (D4.1a rule 2).
type SourcesConfig struct {
	ACM      *ACMSourceConfig `yaml:"acm,omitempty"`
	declared bool
}

// ACMSourceConfig is the ACM Search source. Endpoint and TokenPath are
// required; an absent CAFile means system trust. There is deliberately no
// timeout field: timeouts are derived from the staging budget (D4.1a 7a).
type ACMSourceConfig struct {
	Endpoint  string `yaml:"endpoint"`
	TokenPath string `yaml:"tokenPath"`
	CAFile    string `yaml:"caFile,omitempty"`
}

// Source identifies the collector an FMC deployment runs.
type Source string

const (
	// SourceMCP is the default MCP/GitOps collector, and the behaviour when no
	// `sources` block is rendered.
	SourceMCP Source = "mcp"
	// SourceACM is the ACM Search collector, selected by `sources.acm`.
	SourceACM Source = "acm"
)

// acmSourceKeys is the closed set of keys D4.1a defines. Keeping it as data
// means the unknown-key rejection cannot drift from the struct tags.
var acmSourceKeys = []string{"endpoint", "tokenPath", "caFile"}

// SourceKind reports the selected collector. Absence of `sources` means the
// MCP collector runs, so a rendered config without the block is unchanged
// (D4.1a rule 1).
func (c *ServiceConfig) SourceKind() Source {
	if c.Sources.declared {
		return SourceACM
	}
	return SourceMCP
}

// decodeSourcesStrict validates the `sources` subtree against a closed key set.
// The service refuses to load on an unknown key or a source it does not
// implement: a typo or a stale operator block must never silently disable a
// guard (D4.1a rules 2 and 3). Every other config key keeps its historical
// lenient decoding, so this adds no new failure mode outside `sources`.
//
// YAML merge keys and aliases are flattened before validation. The lenient
// decode resolves them, so inspecting only literal keys would let
// `<<: *anchor` inject an unknown ACM key this pass never sees.
func decodeSourcesStrict(data []byte) (*SourcesConfig, error) {
	rootValues, err := flattenConfig(data)
	if err != nil || rootValues == nil {
		return nil, err
	}
	sourcesNode, found := rootValues["sources"]
	if !found {
		return nil, nil //nolint:nilnil // absence means today's collector
	}
	if isNull(sourcesNode) {
		// Presence selects a source (D4.1a rule 2). A rendered but empty block
		// is a wiring defect, not a request to fall back to MCP.
		return nil, fmt.Errorf("sources is declared but empty; expected sources.acm")
	}
	acmNode, err := acmSourceNode(sourcesNode)
	if err != nil {
		return nil, err
	}
	acm, err := decodeACMStrict(acmNode)
	if err != nil {
		return nil, err
	}
	return &SourcesConfig{ACM: acm, declared: true}, nil
}

// flattenConfig parses the document and returns its effective root mapping
// with merge keys expanded. A nil map means there is nothing to validate.
func flattenConfig(data []byte) (map[string]*yaml.Node, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("failed to parse config sources: %w", err)
	}
	if len(document.Content) == 0 || isNull(document.Content[0]) {
		return nil, nil //nolint:nilnil // empty document: nothing to validate
	}
	root := resolveNode(document.Content[0])
	if root == nil || root.Kind != yaml.MappingNode {
		return nil, nil //nolint:nilnil // the main unmarshal owns non-mapping documents
	}
	_, values, err := flattenMapping(root, "config")
	return values, err
}

// acmSourceNode validates the `sources` mapping against its closed key set and
// returns the acm entry. A present block that names no source is an error, not
// a silent fallback to MCP (D4.1a rule 2).
func acmSourceNode(sourcesNode *yaml.Node) (*yaml.Node, error) {
	order, values, err := flattenMapping(sourcesNode, "sources")
	if err != nil {
		return nil, err
	}
	if unknown := unexpectedKeys(order, []string{"acm"}); len(unknown) > 0 {
		return nil, fmt.Errorf("sources declares unsupported source %q; only sources.acm is supported", unknown[0])
	}
	acmNode, found := values["acm"]
	if !found || isNull(acmNode) {
		return nil, fmt.Errorf("sources is declared but names no source; expected sources.acm")
	}
	return acmNode, nil
}

// decodeACMStrict validates the closed key set of `sources.acm` and decodes it.
// A typo fails the load rather than being dropped (D4.1a rule 3).
func decodeACMStrict(acmNode *yaml.Node) (*ACMSourceConfig, error) {
	order, values, err := flattenMapping(acmNode, "sources.acm")
	if err != nil {
		return nil, err
	}
	if unknown := unexpectedKeys(order, acmSourceKeys); len(unknown) > 0 {
		return nil, fmt.Errorf("sources.acm has unknown key %q", unknown[0])
	}
	decoded := &ACMSourceConfig{}
	// Decode from the flattened, already-validated mapping so a merged key can
	// never reach the struct without having passed the key check above.
	if err := decodeMapping(rebuildMapping(order, values), decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

// flattenMapping returns the effective key order and values of a mapping with
// YAML merge keys expanded, using standard merge precedence: a directly
// declared key wins, and an earlier merge target wins over a later one. The
// result is what a lenient decode would see, which is exactly what must be
// validated.
func flattenMapping(node *yaml.Node, path string) ([]string, map[string]*yaml.Node, error) {
	resolved := resolveNode(node)
	if resolved == nil || resolved.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("%s must be a mapping, got %s", path, nodeKindName(node))
	}
	order, values, err := mergeEntries(resolved, path)
	if err != nil {
		return nil, nil, err
	}
	directOrder, directValues := directEntries(resolved)
	for _, key := range directOrder {
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		// A directly declared key overrides a merged one, which is standard
		// merge precedence and the pattern an operator is most likely to use.
		values[key] = directValues[key]
	}
	return order, values, nil
}

// mergeEntries contributes the mappings named by `<<` keys, earlier merge
// targets winning over later ones.
func mergeEntries(mapping *yaml.Node, path string) ([]string, map[string]*yaml.Node, error) {
	order := make([]string, 0, len(mapping.Content)/2)
	values := make(map[string]*yaml.Node, len(mapping.Content)/2)
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if !isMergeKey(mapping.Content[index]) {
			continue
		}
		for _, merged := range mergeMappings(mapping.Content[index+1]) {
			mergedOrder, mergedValues, err := flattenMapping(merged, path)
			if err != nil {
				return nil, nil, err
			}
			for _, key := range mergedOrder {
				if _, exists := values[key]; exists {
					continue
				}
				values[key] = mergedValues[key]
				order = append(order, key)
			}
		}
	}
	return order, values, nil
}

// directEntries returns the literally declared keys of a mapping, excluding
// merge keys.
func directEntries(mapping *yaml.Node) ([]string, map[string]*yaml.Node) {
	order := make([]string, 0, len(mapping.Content)/2)
	values := make(map[string]*yaml.Node, len(mapping.Content)/2)
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		keyNode := mapping.Content[index]
		if isMergeKey(keyNode) {
			continue
		}
		key := keyNode.Value
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = resolveNode(mapping.Content[index+1])
	}
	return order, values
}

// mergeMappings flattens the value of a `<<` merge key into the mappings it
// contributes: one mapping, or a sequence of them.
func mergeMappings(value *yaml.Node) []*yaml.Node {
	resolved := resolveNode(value)
	switch {
	case resolved == nil:
		return nil
	case resolved.Kind == yaml.MappingNode:
		return []*yaml.Node{resolved}
	case resolved.Kind == yaml.SequenceNode:
		contributed := make([]*yaml.Node, 0, len(resolved.Content))
		for _, item := range resolved.Content {
			if mapping := resolveNode(item); mapping != nil && mapping.Kind == yaml.MappingNode {
				contributed = append(contributed, mapping)
			}
		}
		return contributed
	default:
		return nil
	}
}

// rebuildMapping renders the flattened view as a plain mapping node, so the
// strict key check and the decode cannot disagree about which keys exist.
func rebuildMapping(order []string, values map[string]*yaml.Node) *yaml.Node {
	content := make([]*yaml.Node, 0, len(order)*2)
	for _, key := range order {
		content = append(content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			values[key],
		)
	}
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: content}
}

// unexpectedKeys returns the keys outside allowed, preserving order so the
// error names the operator's typo rather than an arbitrary key.
func unexpectedKeys(keys, allowed []string) []string {
	permitted := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		permitted[key] = struct{}{}
	}
	var unexpected []string
	for _, key := range keys {
		if _, ok := permitted[key]; !ok {
			unexpected = append(unexpected, key)
		}
	}
	return unexpected
}

// isMergeKey reports whether a mapping key is YAML's merge key. The tag is the
// only reliable signal: a quoted "<<" or '<<' is an ordinary string key, and
// treating it as a merge would drop an unknown key from the closed-set check.
func isMergeKey(node *yaml.Node) bool {
	return node != nil && node.Tag == "!!merge"
}

// resolveNode follows alias nodes so an anchored value is inspected through
// its real content.
func resolveNode(node *yaml.Node) *yaml.Node {
	for hops := 0; node != nil && node.Kind == yaml.AliasNode && hops < 16; hops++ {
		node = node.Alias
	}
	return node
}

func isNull(node *yaml.Node) bool {
	resolved := resolveNode(node)
	return resolved == nil || (resolved.Kind == yaml.ScalarNode && resolved.Tag == "!!null")
}

func nodeKindName(node *yaml.Node) string {
	resolved := resolveNode(node)
	switch {
	case resolved == nil:
		return "nothing"
	case resolved.Kind == yaml.MappingNode:
		return "mapping"
	case resolved.Kind == yaml.SequenceNode:
		return "sequence"
	case resolved.Kind == yaml.ScalarNode:
		return "scalar"
	default:
		return "document"
	}
}

// decodeMapping decodes one already-validated mapping node into target.
func decodeMapping(node *yaml.Node, target *ACMSourceConfig) error {
	rendered, err := yaml.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to parse config sources: %w", err)
	}
	if err := yaml.Unmarshal(rendered, target); err != nil {
		return fmt.Errorf("failed to parse config sources: %w", err)
	}
	return nil
}

// validateACM fails closed at startup, not at the first sync (D4.1a rule 3).
// Every message names the key so an operator can act on it, and no message
// ever carries token material.
func (c *ServiceConfig) validateACM(source *ACMSourceConfig) error {
	parsed, err := url.Parse(source.Endpoint)
	if err != nil || !parsed.IsAbs() || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("sources.acm.endpoint must be an absolute https URL with a host, got %q", source.Endpoint)
	}
	if source.TokenPath == "" || !filepath.IsAbs(source.TokenPath) {
		return fmt.Errorf("sources.acm.tokenPath must be an absolute path, got %q", source.TokenPath)
	}
	if err := checkReadableRegularFile(source.TokenPath); err != nil {
		return fmt.Errorf("sources.acm.tokenPath %q is not a readable file: %w", source.TokenPath, err)
	}
	if source.CAFile != "" && !filepath.IsAbs(source.CAFile) {
		return fmt.Errorf("sources.acm.caFile must be an absolute path, got %q", source.CAFile)
	}
	return nil
}

// checkReadableRegularFile opens the path, which is the only check that proves
// FMC can actually read it. Stat alone accepts a file the process may not open,
// so the failure would surface on the first Search request instead of at
// startup (D4.1a rule 3).
func checkReadableRegularFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("is a directory")
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("is not a regular file")
	}
	return nil
}
