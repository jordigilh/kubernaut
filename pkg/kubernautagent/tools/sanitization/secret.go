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

package sanitization

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	sharedsanitization "github.com/jordigilh/kubernaut/pkg/shared/sanitization"
	"gopkg.in/yaml.v3"
)

// SecretSanitizer redacts Kubernetes Secret data/stringData values from tool
// output to prevent cleartext credentials from reaching audit storage (SOC2).
//
// Unlike the regex-based k8s-secret-data rule in shared/sanitization (which
// cannot establish Kubernetes object context), this stage handles JSON and YAML
// representations returned by Kubernetes tools. It only redacts values below
// data/stringData on Secret-shaped objects.
type SecretSanitizer struct{}

// NewSecretSanitizer creates a K8s Secret sanitizer stage.
func NewSecretSanitizer() *SecretSanitizer {
	return &SecretSanitizer{}
}

// Name implements Stage.
func (s *SecretSanitizer) Name() string { return "K8S-SECRET" }

// Sanitize implements Stage. Detects JSON or YAML blobs containing K8s Secrets
// and replaces data/stringData values with [REDACTED]. Non-Secret, invalid, and
// unmodified input is returned byte-for-byte unchanged.
func (s *SecretSanitizer) Sanitize(_ context.Context, input string) (string, error) {
	if json.Valid([]byte(input)) {
		redacted, changed := redactSecretJSON([]byte(input))
		if changed {
			return string(redacted), nil
		}
		return input, nil
	}

	redacted, changed := redactSecretYAML([]byte(input))
	if changed {
		return string(redacted), nil
	}
	return input, nil
}

func redactSecretYAML(raw []byte) ([]byte, bool) {
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	documents := make([]yaml.Node, 0, 1)
	changed := false

	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return raw, false
		}
		documents = append(documents, document)
		if redactSecretYAMLDocument(&document) {
			changed = true
		}
	}

	if !changed {
		return raw, false
	}

	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	for i := range documents {
		if err := encoder.Encode(&documents[i]); err != nil {
			if closeErr := encoder.Close(); closeErr != nil {
				return raw, false
			}
			return raw, false
		}
	}
	if err := encoder.Close(); err != nil {
		return raw, false
	}
	return output.Bytes(), true
}

func redactSecretYAMLDocument(document *yaml.Node) bool {
	if len(document.Content) == 0 {
		return false
	}
	return redactSecretYAMLResource(document.Content[0])
}

func redactSecretYAMLResource(resource *yaml.Node) bool {
	if resource == nil || resource.Kind != yaml.MappingNode {
		return false
	}

	kind := yamlMappingScalar(resource, "kind")
	switch kind {
	case "Secret":
		return redactSecretYAMLFields(resource)
	case "SecretList", "List":
		items := yamlMappingNode(resource, "items")
		if items == nil || items.Kind != yaml.SequenceNode {
			return false
		}
		changed := false
		for _, item := range items.Content {
			if redactSecretYAMLResource(item) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

func redactSecretYAMLFields(secret *yaml.Node) bool {
	changed := false
	for _, field := range []string{"data", "stringData"} {
		values := yamlMappingNode(secret, field)
		if values == nil || values.Kind != yaml.MappingNode {
			continue
		}
		for i := 1; i < len(values.Content); i += 2 {
			value := values.Content[i]
			value.Kind = yaml.ScalarNode
			value.Tag = "!!str"
			// yaml.v3 represents a plain scalar with zero style; there is no
			// exported PlainStyle constant. Clear any literal/quoted style from
			// the original Secret value before writing the placeholder.
			value.Style = 0
			value.Value = sharedsanitization.RedactedPlaceholder
			changed = true
		}
	}
	return changed
}

func yamlMappingNode(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func yamlMappingScalar(mapping *yaml.Node, key string) string {
	value := yamlMappingNode(mapping, key)
	if value == nil || value.Kind != yaml.ScalarNode {
		return ""
	}
	return value.Value
}

// redactSecretJSON checks if the JSON blob is a K8s Secret (or a list
// containing Secrets) and redacts data/stringData map values.
func redactSecretJSON(raw []byte) ([]byte, bool) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return raw, false
	}

	if isSecretKind(obj) {
		return redactSecretFields(obj)
	}

	if isList(obj) {
		return redactSecretList(raw, obj)
	}

	return raw, false
}

func isSecretKind(obj map[string]json.RawMessage) bool {
	kindRaw, ok := obj["kind"]
	if !ok {
		return false
	}
	var kind string
	if err := json.Unmarshal(kindRaw, &kind); err != nil {
		return false
	}
	return kind == "Secret"
}

func isList(obj map[string]json.RawMessage) bool {
	kindRaw, ok := obj["kind"]
	if !ok {
		return false
	}
	var kind string
	if err := json.Unmarshal(kindRaw, &kind); err != nil {
		return false
	}
	return kind == "SecretList" || kind == "List"
}

func redactSecretFields(obj map[string]json.RawMessage) ([]byte, bool) {
	changed := false
	for _, field := range []string{"data", "stringData"} {
		fieldRaw, ok := obj[field]
		if !ok {
			continue
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(fieldRaw, &m); err != nil {
			continue
		}
		redactedValue, err := json.Marshal(sharedsanitization.RedactedPlaceholder)
		if err != nil {
			continue
		}
		for k := range m {
			m[k] = redactedValue
		}
		redacted, err := json.Marshal(m)
		if err != nil {
			continue
		}
		obj[field] = json.RawMessage(redacted)
		changed = true
	}
	if !changed {
		return nil, false
	}
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, false
	}
	return out, true
}

func redactSecretList(raw []byte, obj map[string]json.RawMessage) ([]byte, bool) {
	itemsRaw, ok := obj["items"]
	if !ok {
		return raw, false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(itemsRaw, &items); err != nil {
		return raw, false
	}
	anyChanged := false
	for i, item := range items {
		var itemObj map[string]json.RawMessage
		if err := json.Unmarshal(item, &itemObj); err != nil {
			continue
		}
		if isSecretKind(itemObj) {
			if redacted, changed := redactSecretFields(itemObj); changed {
				items[i] = redacted
				anyChanged = true
			}
		}
	}
	if !anyChanged {
		return raw, false
	}
	redactedItems, err := json.Marshal(items)
	if err != nil {
		return raw, false
	}
	obj["items"] = json.RawMessage(redactedItems)
	out, err := json.Marshal(obj)
	if err != nil {
		return raw, false
	}
	return out, true
}
