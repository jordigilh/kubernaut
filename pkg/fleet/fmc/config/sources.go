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
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// SourcesConfig selects where FMC collects fleet inventory from
// (DD-FLEET-017 D4.1a). The operator renders this block; FMC receives
// resolved artifact paths only, never policy.
type SourcesConfig struct {
	ACM *ACMSourceConfig `yaml:"acm,omitempty"`
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

// SourceKind reports the selected collector. Absence of `sources` means the
// MCP collector runs, so a rendered config without the block is unchanged
// (D4.1a rule 1).
func (c *ServiceConfig) SourceKind() Source {
	if c.Sources.ACM != nil {
		return SourceACM
	}
	return SourceMCP
}

// decodeSourcesStrict re-decodes the `sources` subtree with strict field
// matching. The service refuses to load on an unknown key: a typo in an
// operator-rendered block must never silently disable a guard (D4.1a rule 3).
// Every other key keeps its historical lenient decoding, so this adds no new
// failure mode to existing configuration.
func decodeSourcesStrict(data []byte) (*SourcesConfig, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("failed to parse config sources: %w", err)
	}
	if len(document.Content) == 0 || document.Content[0].Kind != yaml.MappingNode {
		return nil, nil //nolint:nilnil // nothing to decode strictly; the main unmarshal owns shape errors
	}
	root := document.Content[0]
	var sourcesNode *yaml.Node
	for index := 0; index+1 < len(root.Content); index += 2 {
		if root.Content[index].Value == "sources" {
			sourcesNode = root.Content[index+1]
			break
		}
	}
	if sourcesNode == nil || (sourcesNode.Kind == yaml.ScalarNode && sourcesNode.Tag == "!!null") {
		return nil, nil //nolint:nilnil // absence means today's collector
	}
	rendered, err := yaml.Marshal(sourcesNode)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config sources: %w", err)
	}
	decoded := &SourcesConfig{}
	decoder := yaml.NewDecoder(bytes.NewReader(rendered))
	decoder.KnownFields(true)
	if err := decoder.Decode(decoded); err != nil {
		return nil, fmt.Errorf("failed to parse config sources: %w", err)
	}
	return decoded, nil
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
	info, err := os.Stat(source.TokenPath)
	if err != nil || info.IsDir() {
		return fmt.Errorf("sources.acm.tokenPath must be a readable file, got %q", source.TokenPath)
	}
	if source.CAFile != "" && !filepath.IsAbs(source.CAFile) {
		return fmt.Errorf("sources.acm.caFile must be an absolute path, got %q", source.CAFile)
	}
	return nil
}
