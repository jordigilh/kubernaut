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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"
)

// EAIGWImage is the Envoy AI Gateway CLI image for fleet MCP routing.
const EAIGWImage = "docker.io/envoyproxy/ai-gateway-cli:v1.0.0"

// EAIGWMCPServerEntry represents a backend MCP server in the EAIGW JSON config.
type EAIGWMCPServerEntry struct {
	Name string `json:"name"`
	Host string `json:"host"`
}

// StartEAIGWContainer starts an EAIGW container configured to proxy to the
// specified MCP backend servers. The config is written as a JSON file mounted
// into the container.
//
// Per spike findings: EAIGW uses `--mcp-config <json-file>` for standalone mode.
// Tool names are prefixed with `{backendName}__` (e.g., cluster-a__resources_get).
func StartEAIGWContainer(servers []EAIGWMCPServerEntry, writer io.Writer) (*ContainerInstance, error) {
	configJSON, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal EAIGW config: %w", err)
	}

	tmpFile, err := os.CreateTemp("", "eaigw-mcp-config-*.json")
	if err != nil {
		return nil, fmt.Errorf("create temp config file: %w", err)
	}
	if _, err := tmpFile.Write(configJSON); err != nil {
		return nil, fmt.Errorf("write EAIGW config: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("close EAIGW config file: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   📝 EAIGW config written to %s: %s\n", tmpFile.Name(), string(configJSON))

	cfg := GenericContainerConfig{
		Name:  "fleet_mcp_gateway_it",
		Image: EAIGWImage,
		Ports: map[int]int{
			1975: 19750,
			1064: 11064,
		},
		Volumes: map[string]string{
			tmpFile.Name(): "/etc/aigw/mcp-servers.json",
		},
		Cmd:        []string{"run", "--mcp-config", "/etc/aigw/mcp-servers.json", "--run-id=0"},
		ExtraHosts: []string{"host.containers.internal:host-gateway"},
		HealthCheck: &HealthCheckConfig{
			URL:     "http://127.0.0.1:11064/health",
			Timeout: 45 * time.Second,
		},
	}

	return StartGenericContainer(cfg, writer)
}

// StopEAIGWContainer stops and removes the EAIGW IT container.
func StopEAIGWContainer(instance *ContainerInstance, writer io.Writer) error {
	return StopGenericContainer(instance, writer)
}
