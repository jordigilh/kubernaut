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

package mcp

import "fmt"

// ServerConfig holds configuration for a single MCP server.
type ServerConfig struct {
	Name      string `yaml:"name"`
	URL       string `yaml:"url"`
	Transport string `yaml:"transport"`
}

// ParseConfigs validates and returns typed MCP server configurations.
func ParseConfigs(entries []ServerConfig) ([]ServerConfig, error) {
	result := make([]ServerConfig, 0, len(entries))
	for i, e := range entries {
		if e.Name == "" {
			return nil, fmt.Errorf("mcp_servers[%d].name is required", i)
		}
		if e.URL == "" {
			return nil, fmt.Errorf("mcp_servers[%d].url is required", i)
		}
		if e.Transport == "" {
			return nil, fmt.Errorf("mcp_servers[%d].transport is required", i)
		}
		result = append(result, e)
	}
	return result, nil
}
