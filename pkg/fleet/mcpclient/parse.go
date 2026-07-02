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

func parseUnstructured(text string) (*unstructured.Unstructured, error) {
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
