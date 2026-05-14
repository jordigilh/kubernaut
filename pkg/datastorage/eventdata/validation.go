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

// Package eventdata provides shared EventData validation used by both
// the DLQ client (pkg/datastorage/dlq) and InternalAuditClient (pkg/audit).
// Extracted to break the import cycle between those packages.
package eventdata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const (
	// MaxEventDataSize caps EventData payload size (consistent with gateway MaxRequestBodySize).
	MaxEventDataSize = 256 * 1024 // 256 KB
	// MaxEventDataDepth prevents billion-laughs / recursive JSON attacks.
	MaxEventDataDepth = 10
)

// ValidateEventData checks EventData size and JSON nesting depth.
func ValidateEventData(data []byte) error {
	if len(data) > MaxEventDataSize {
		return fmt.Errorf("EventData exceeds maximum size (%d > %d bytes)", len(data), MaxEventDataSize)
	}
	if len(data) == 0 {
		return nil
	}
	return ValidateJSONDepth(data, MaxEventDataDepth)
}

// ValidateJSONDepth walks JSON tokens and returns an error if nesting exceeds maxDepth.
func ValidateJSONDepth(data []byte, maxDepth int) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var depth int
	for {
		t, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("invalid JSON in EventData: %w", err)
		}
		switch t {
		case json.Delim('{'), json.Delim('['):
			depth++
			if depth > maxDepth {
				return fmt.Errorf("EventData JSON nesting depth exceeds maximum (%d)", maxDepth)
			}
		case json.Delim('}'), json.Delim(']'):
			depth--
		}
	}
}
