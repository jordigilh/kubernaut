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

package infrastructure

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFleetGatewayVersionOverrides loads the optional dependency override file
// used by the fleet demo. Empty paths preserve the validated built-in matrix.
func LoadFleetGatewayVersionOverrides(path string) (FleetGatewayVersionOverrides, error) {
	if strings.TrimSpace(path) == "" {
		return FleetGatewayVersionOverrides{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return FleetGatewayVersionOverrides{}, fmt.Errorf("read dependency overrides file %q: %w", path, err)
	}
	return parseFleetGatewayVersionOverrides(data)
}

// parseFleetGatewayVersionOverrides decodes one strict YAML override document.
func parseFleetGatewayVersionOverrides(data []byte) (FleetGatewayVersionOverrides, error) {
	var overrides FleetGatewayVersionOverrides
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&overrides); err != nil {
		if errors.Is(err, io.EOF) {
			return FleetGatewayVersionOverrides{}, nil
		}
		return FleetGatewayVersionOverrides{}, fmt.Errorf("parse dependency overrides: %w", err)
	}
	return overrides, nil
}

// MergeFleetGatewayVersionOverrides applies explicit CLI/Make values over the
// optional file values. This keeps one-off compatibility experiments convenient
// without requiring a second file for every version change.
func MergeFleetGatewayVersionOverrides(file, explicit FleetGatewayVersionOverrides) FleetGatewayVersionOverrides {
	merged := file
	if strings.TrimSpace(explicit.EnvoyGatewayVersion) != "" {
		merged.EnvoyGatewayVersion = explicit.EnvoyGatewayVersion
	}
	if strings.TrimSpace(explicit.AgentRouterVersion) != "" {
		merged.AgentRouterVersion = explicit.AgentRouterVersion
	}
	return merged
}
