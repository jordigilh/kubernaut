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
)

func parseUnstructured(text string) (*unstructured.Unstructured, error) {
	if text == "" {
		return nil, fmt.Errorf("empty response")
	}

	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal([]byte(text), &obj.Object); err != nil {
		return nil, fmt.Errorf("unmarshaling resource: %w", err)
	}
	return obj, nil
}

func parseUnstructuredList(text string) ([]unstructured.Unstructured, error) {
	if text == "" {
		return nil, nil
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		var items []map[string]any
		if err2 := json.Unmarshal([]byte(text), &items); err2 != nil {
			return nil, fmt.Errorf("unmarshaling list response: %w", err)
		}
		result := make([]unstructured.Unstructured, len(items))
		for i, item := range items {
			result[i] = unstructured.Unstructured{Object: item}
		}
		return result, nil
	}

	if items, ok := raw["items"].([]any); ok {
		result := make([]unstructured.Unstructured, 0, len(items))
		for _, item := range items {
			if m, ok := item.(map[string]any); ok {
				result = append(result, unstructured.Unstructured{Object: m})
			}
		}
		return result, nil
	}

	return []unstructured.Unstructured{{Object: raw}}, nil
}
