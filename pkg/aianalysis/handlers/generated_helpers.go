/*
Copyright 2025 Jordi Gil.

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

// Package handlers provides helper functions for working with generated HAPI types
package handlers

import (
	"encoding/json"

	"github.com/go-faster/jx"
	client "github.com/jordigilh/kubernaut/pkg/holmesgpt/client"
)

// Helper functions for extracting values from generated optional types

// GetOptBoolValue returns the bool value from OptBool, or false if not set
func GetOptBoolValue(opt client.OptBool) bool {
	if opt.Set {
		return opt.Value
	}
	return false
}

// GetOptNilStringValue returns the string value from OptNilString, or empty string if not set
func GetOptNilStringValue(opt client.OptNilString) string {
	if opt.Set && !opt.Null {
		return opt.Value
	}
	return ""
}

// GetMapFromOptNil extracts a map from optional generated types using JSON marshaling
func GetMapFromOptNil(data interface{}) map[string]interface{} {
	if data == nil {
		return nil
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		return nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return nil
	}

	return result
}

// GetStringFromMap safely extracts a string from a map
func GetStringFromMap(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// GetFloat64FromMap safely extracts a float64 from a map
func GetFloat64FromMap(m map[string]interface{}, key string) float64 {
	if m == nil {
		return 0
	}
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}
	return 0
}

// GetStringSliceFromMap safely extracts []string from a map
func GetStringSliceFromMap(m map[string]interface{}, key string) []string {
	if m == nil {
		return nil
	}
	if val, ok := m[key]; ok {
		if slice, ok := val.([]interface{}); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}
	return nil
}

// GetMapFromMapSafe safely extracts a nested map
func GetMapFromMapSafe(m map[string]interface{}, key string) map[string]interface{} {
	if m == nil {
		return nil
	}
	if val, ok := m[key]; ok {
		if nested, ok := val.(map[string]interface{}); ok {
			return nested
		}
	}
	return nil
}

// GetBoolFromMap safely extracts a bool from a map
func GetBoolFromMap(m map[string]interface{}, key string) bool {
	if m == nil {
		return false
	}
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
	}
	return false
}

// GetMapFromJxRaw extracts a map[string]interface{} from jx.Raw
func GetMapFromJxRaw(raw jx.Raw) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// convertMapToStringMap converts map[string]interface{} to map[string]string
// by converting all values to strings (for workflow parameters)
func convertMapToStringMap(m map[string]interface{}) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		if str, ok := v.(string); ok {
			result[k] = str
		} else {
			// Convert non-string values to JSON string representation
			if bytes, err := json.Marshal(v); err == nil {
				result[k] = string(bytes)
			}
		}
	}
	return result
}
