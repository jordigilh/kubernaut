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

package mcpclient

import (
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	sigsyaml "sigs.k8s.io/yaml"
)

// ParseUnstructuredResponse parses a kube-mcp-server tool response (JSON or
// YAML text) into an unstructured.Unstructured object. Exported (renamed from
// parseUnstructured, issue #2306) so KA's overlayClientReader can reuse this
// already-proven parsing path instead of reimplementing it.
func ParseUnstructuredResponse(text string) (*unstructured.Unstructured, error) {
	if text == "" {
		return nil, fmt.Errorf("empty response")
	}

	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal([]byte(text), &obj.Object); err != nil {
		jsonData, yamlErr := sigsyaml.YAMLToJSON([]byte(text))
		if yamlErr != nil {
			return nil, fmt.Errorf("unmarshaling resource: %w", err)
		}
		if err2 := json.Unmarshal(jsonData, &obj.Object); err2 != nil {
			return nil, fmt.Errorf("unmarshaling resource: %w", err2)
		}
	}
	return obj, nil
}

// ParseUnstructuredListResponse parses a kube-mcp-server list response into
// unstructured objects. List tools may return either a top-level sequence or
// the Kubernetes-style {"items": [...]} envelope.
func ParseUnstructuredListResponse(text string) ([]unstructured.Unstructured, error) {
	if text == "" {
		return nil, fmt.Errorf("empty response")
	}

	jsonData := []byte(text)
	if !json.Valid(jsonData) {
		converted, err := sigsyaml.YAMLToJSON(jsonData)
		if err != nil {
			return nil, fmt.Errorf("unmarshaling resource list: %w", err)
		}
		jsonData = converted
	}

	var rawItems []json.RawMessage
	if err := json.Unmarshal(jsonData, &rawItems); err != nil {
		var envelope struct {
			Items []json.RawMessage `json:"items"`
		}
		if envelopeErr := json.Unmarshal(jsonData, &envelope); envelopeErr != nil {
			return nil, fmt.Errorf("unmarshaling resource list: %w", err)
		}
		if envelope.Items == nil {
			return nil, fmt.Errorf("resource list response does not contain items")
		}
		rawItems = envelope.Items
	}

	items := make([]unstructured.Unstructured, 0, len(rawItems))
	for i, rawItem := range rawItems {
		var object map[string]interface{}
		if err := json.Unmarshal(rawItem, &object); err != nil || object == nil {
			if err == nil {
				err = fmt.Errorf("item is null")
			}
			return nil, fmt.Errorf("resource list item %d is not an object: %w", i, err)
		}
		items = append(items, unstructured.Unstructured{Object: object})
	}
	return items, nil
}

// normalizeTableItems converts flat table-row maps from kube-mcp-server
// structuredContent (--list-output=table) into proper unstructured.Unstructured
// objects using typed setters.
//
// Flat maps have capitalized column keys (Name, Namespace, Status, Age, etc.)
// but lack metadata, kind, and apiVersion. These are injected from the tool
// call context. Validated by Spike S17 against output.Table.PrintObjStructured.
func normalizeTableItems(items []map[string]any, kind, apiVersion string) []unstructured.Unstructured {
	result := make([]unstructured.Unstructured, 0, len(items))
	for _, m := range items {
		if _, hasMetadata := m["metadata"]; hasMetadata {
			result = append(result, unstructured.Unstructured{Object: m})
			continue
		}

		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(apiVersion)
		obj.SetKind(kind)

		if name, ok := m["Name"].(string); ok {
			obj.SetName(name)
		}
		if ns, ok := m["Namespace"].(string); ok && ns != "" {
			obj.SetNamespace(ns)
		}

		result = append(result, *obj)
	}
	return result
}
