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
	"context"
	"encoding/json"

	sharedsanitization "github.com/jordigilh/kubernaut/pkg/shared/sanitization"
)

// SecretSanitizer redacts Kubernetes Secret data/stringData values from tool
// output to prevent cleartext credentials from reaching audit storage (SOC2).
//
// Unlike the regex-based k8s-secret-data rule in shared/sanitization (which
// targets YAML-format lines), this stage handles the JSON representation
// returned by K8s tools via json.Marshal of API objects.
type SecretSanitizer struct{}

// NewSecretSanitizer creates a K8s Secret sanitizer stage.
func NewSecretSanitizer() *SecretSanitizer {
	return &SecretSanitizer{}
}

// Name implements Stage.
func (s *SecretSanitizer) Name() string { return "K8S-SECRET" }

// Sanitize implements Stage. Detects JSON blobs containing K8s Secrets
// and replaces data/stringData values with [REDACTED].
func (s *SecretSanitizer) Sanitize(_ context.Context, input string) (string, error) {
	if !json.Valid([]byte(input)) {
		return input, nil
	}

	redacted, changed := redactSecretJSON([]byte(input))
	if changed {
		return string(redacted), nil
	}
	return input, nil
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
		var m map[string]interface{}
		if err := json.Unmarshal(fieldRaw, &m); err != nil {
			continue
		}
		for k := range m {
			m[k] = sharedsanitization.RedactedPlaceholder
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
