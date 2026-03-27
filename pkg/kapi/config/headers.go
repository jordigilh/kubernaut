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
	"os"
	"strings"
)

var reservedHeaders = map[string]struct{}{
	"content-type": {},
	"accept":       {},
	"host":         {},
	"user-agent":   {},
}

// HeaderDefinition describes a custom HTTP header to inject into outbound LLM requests.
// Exactly one value source (Value, SecretKeyRef, or FilePath) must be set.
//
// Authority: Issue #417 — Support custom authentication headers for LLM proxy endpoints
type HeaderDefinition struct {
	Name         string `yaml:"name"`
	Value        string `yaml:"value,omitempty"`
	SecretKeyRef string `yaml:"secretKeyRef,omitempty"`
	FilePath     string `yaml:"filePath,omitempty"`
}

// ParseCustomHeaders validates a slice of header definitions, returning the validated
// slice or an error if any definition is malformed. Returns an empty slice for nil/empty input.
func ParseCustomHeaders(defs []HeaderDefinition) ([]HeaderDefinition, error) {
	if len(defs) == 0 {
		return []HeaderDefinition{}, nil
	}

	seen := make(map[string]struct{}, len(defs))
	for i, def := range defs {
		if err := validateDefinition(def); err != nil {
			return nil, fmt.Errorf("header[%d]: %w", i, err)
		}
		canonical := strings.ToLower(def.Name)
		if _, exists := seen[canonical]; exists {
			return nil, fmt.Errorf("header[%d]: duplicate header name %q", i, def.Name)
		}
		seen[canonical] = struct{}{}
	}
	return defs, nil
}

// ValidateSource checks that exactly one value source is set on a header definition.
func ValidateSource(def HeaderDefinition) error {
	count := 0
	if def.Value != "" {
		count++
	}
	if def.SecretKeyRef != "" {
		count++
	}
	if def.FilePath != "" {
		count++
	}
	if count != 1 {
		return fmt.Errorf("exactly one of value, secretKeyRef, or filePath must be set (got %d)", count)
	}
	return nil
}

// ValidateHeaderName checks that a header name is not reserved.
func ValidateHeaderName(name string) error {
	if _, reserved := reservedHeaders[strings.ToLower(name)]; reserved {
		return fmt.Errorf("header name %q is reserved and cannot be overridden", name)
	}
	return nil
}

// ValidateHeaderSources checks that all secretKeyRef sources resolve to non-empty
// environment variables at startup. Value and filePath sources are not validated here
// (filePath is validated at request time, value is always available).
func ValidateHeaderSources(defs []HeaderDefinition) error {
	for _, def := range defs {
		if def.SecretKeyRef != "" {
			val := os.Getenv(def.SecretKeyRef)
			if val == "" {
				return fmt.Errorf("secretKeyRef %q: environment variable is empty or unset", def.SecretKeyRef)
			}
		}
	}
	return nil
}

func validateDefinition(def HeaderDefinition) error {
	if strings.TrimSpace(def.Name) == "" {
		return fmt.Errorf("header name is required")
	}
	if err := ValidateHeaderName(def.Name); err != nil {
		return err
	}
	return ValidateSource(def)
}
