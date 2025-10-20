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

// Package query provides vector search utilities for pgvector
// BR-CONTEXT-002: Semantic search on embeddings
package query

import (
	"fmt"
	"strconv"
	"strings"
)

// VectorToString converts embedding vector to PostgreSQL vector format
// BR-CONTEXT-002: Vector serialization for pgvector queries
//
// Converts []float32 to "[x,y,z,...]" format required by pgvector
//
// Example:
//
//	[]float32{0.1, 0.2, 0.3} → "[0.1,0.2,0.3]"
func VectorToString(embedding []float32) (string, error) {
	if embedding == nil {
		return "", fmt.Errorf("embedding cannot be nil")
	}
	if len(embedding) == 0 {
		return "", fmt.Errorf("embedding cannot be empty")
	}

	// Convert to string format: [x,y,z,...]
	parts := make([]string, len(embedding))
	for i, val := range embedding {
		parts[i] = strconv.FormatFloat(float64(val), 'f', -1, 32)
	}

	return "[" + strings.Join(parts, ",") + "]", nil
}

// StringToVector converts PostgreSQL vector format to embedding vector
// BR-CONTEXT-002: Vector deserialization from pgvector results
//
// Converts "[x,y,z,...]" format to []float32
//
// Example:
//
//	"[0.1,0.2,0.3]" → []float32{0.1, 0.2, 0.3}
func StringToVector(vectorStr string) ([]float32, error) {
	if vectorStr == "" {
		return nil, fmt.Errorf("vector string cannot be empty")
	}

	// Remove brackets
	vectorStr = strings.TrimPrefix(vectorStr, "[")
	vectorStr = strings.TrimSuffix(vectorStr, "]")

	if vectorStr == "" {
		return []float32{}, nil
	}

	// Split and parse
	parts := strings.Split(vectorStr, ",")
	result := make([]float32, len(parts))

	for i, part := range parts {
		val, err := strconv.ParseFloat(strings.TrimSpace(part), 32)
		if err != nil {
			return nil, fmt.Errorf("failed to parse vector element %d: %w", i, err)
		}
		result[i] = float32(val)
	}

	return result, nil
}
