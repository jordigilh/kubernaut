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
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// EAIGWImage is the Envoy AI Gateway CLI image for fleet MCP routing.
const EAIGWImage = "docker.io/envoyproxy/ai-gateway-cli:v1.0.0"

// EAIGWMCPServerEntry represents a backend MCP server in the EAIGW JSON config.
type EAIGWMCPServerEntry struct {
	Name string `json:"name"`
	Host string `json:"host"`
}

// eaigwMCPServerConfig is a single entry under the canonical MCP servers
// config's top-level "mcpServers" map (the same format used by Claude
// Desktop/Cursor/VS Code, per aigw's --mcp-config/--mcp-json docs).
type eaigwMCPServerConfig struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// eaigwMCPConfig is the top-level document aigw's --mcp-config expects.
type eaigwMCPConfig struct {
	MCPServers map[string]eaigwMCPServerConfig `json:"mcpServers"`
}

// EAIGWMCPPort and EAIGWAdminPort are aigw's fixed internal listen ports
// (not configurable via flags/env -- only --admin-port exists, and this
// suite keeps the default to match the spike's validated behavior).
const (
	EAIGWMCPPort   = 1975
	EAIGWAdminPort = 1064
)

// StartEAIGWContainer starts an EAIGW container configured to proxy to the
// specified MCP backend servers. The config is written as a JSON file mounted
// into the container. The returned ContainerInstance.Ports maps
// EAIGWMCPPort/EAIGWAdminPort to the actual host-reachable ports -- callers
// must not hardcode a mapped port, since it differs by platform (see below).
//
// Per spike findings: EAIGW uses `--mcp-config <json-file>` for standalone mode.
// Tool names are prefixed with `{backendName}__` (e.g., cluster-a__resources_get).
//
// Platform-specific networking (same DD-AUTH-014 pattern as
// datastorage_bootstrap.go): backend hosts are typically an httptest.Server
// bound to 127.0.0.1 on the *host* test process, which is only reachable
// from inside a container's own network namespace in specific ways:
//   - Linux (CI): --network=host shares the host's net namespace directly,
//     so "127.0.0.1" inside the container IS the host's loopback -- no
//     rewrite needed, but the container's fixed ports (1975/1064) become
//     the actual host ports too (no remapping possible under host network).
//   - macOS: Podman machine's VM proxies "host.containers.internal" through
//     to the host's loopback specifically (a courtesy the VM provides for
//     this exact case) -- true --network=host doesn't work in the Podman
//     VM, so bridge networking + the host.containers.internal rewrite +
//     conventional port mapping is used instead.
func StartEAIGWContainer(servers []EAIGWMCPServerEntry, writer io.Writer) (*ContainerInstance, error) {
	useHostNetwork := runtime.GOOS == "linux"

	mcpServers := make(map[string]eaigwMCPServerConfig, len(servers))
	for _, s := range servers {
		host := s.Host
		if !useHostNetwork {
			host = strings.Replace(host, "127.0.0.1", "host.containers.internal", 1)
			host = strings.Replace(host, "localhost", "host.containers.internal", 1)
		}
		mcpServers[s.Name] = eaigwMCPServerConfig{Type: "http", URL: host}
	}
	// aigw's --mcp-config expects the canonical mcpServers object format
	// (https://aigateway.envoyproxy.io/docs/cli/aigwrun/), not a bare array --
	// a bare array unmarshals with "Mismatch type autoconfig.MCPServers with
	// value array".
	configJSON, err := json.MarshalIndent(eaigwMCPConfig{MCPServers: mcpServers}, "", "  ")
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
	// os.CreateTemp defaults to 0600 (owner-only). Under rootless Podman the
	// container's runtime user is UID-namespace-remapped and does not match
	// the host process's UID, so a 0600 bind-mounted file reads back as
	// "permission denied" inside the container even though the host process
	// that wrote it can read it fine (only observed in CI; same fix pattern
	// as serviceaccount.go's kubeconfig temp files). Contains no secrets --
	// just backend names/hosts for a test-only mock gateway.
	if err := os.Chmod(tmpFile.Name(), 0644); err != nil {
		return nil, fmt.Errorf("chmod EAIGW config file: %w", err)
	}

	_, _ = fmt.Fprintf(writer, "   📝 EAIGW config written to %s: %s\n", tmpFile.Name(), string(configJSON))

	cfg := GenericContainerConfig{
		Name:  "fleet_mcp_gateway_it",
		Image: EAIGWImage,
		Volumes: map[string]string{
			tmpFile.Name(): "/etc/aigw/mcp-servers.json",
		},
		Cmd: []string{"run", "--mcp-config", "/etc/aigw/mcp-servers.json", "--run-id=0"},
	}

	var healthPort, mcpPort int
	if useHostNetwork {
		cfg.Network = "host"
		// No port mapping under --network=host: the container's fixed
		// ports ARE the host ports. Still recorded in cfg.Ports (ignored by
		// StartGenericContainer's -p flag emission when Network=="host" has
		// no effect either way) purely so ContainerInstance.Ports reports
		// the right value back to callers.
		cfg.Ports = map[int]int{EAIGWMCPPort: EAIGWMCPPort, EAIGWAdminPort: EAIGWAdminPort}
		healthPort = EAIGWAdminPort
		mcpPort = EAIGWMCPPort
	} else {
		cfg.Ports = map[int]int{EAIGWMCPPort: 19750, EAIGWAdminPort: 11064}
		cfg.ExtraHosts = []string{"host.containers.internal:host-gateway"}
		healthPort = 11064
		mcpPort = 19750
	}
	cfg.HealthCheck = &HealthCheckConfig{
		URL:     fmt.Sprintf("http://127.0.0.1:%d/health", healthPort),
		Timeout: 45 * time.Second,
	}

	instance, err := StartGenericContainer(cfg, writer)
	if err != nil {
		return nil, err
	}

	// aigw's admin HTTP server (health endpoint) comes up before its Envoy
	// data-plane listener finishes binding -- the HealthCheck above only
	// proves the former. Callers dial the MCP port (mcpPort) immediately
	// after this returns, so block here until it actually accepts
	// connections too, or "initialize" intermittently fails with
	// "connection refused" (observed in CI where the two-stage startup gap
	// is a couple of seconds).
	mcpAddr := fmt.Sprintf("127.0.0.1:%d", mcpPort)
	_, _ = fmt.Fprintf(writer, "   ⏳ Waiting for MCP data-plane port: %s\n", mcpAddr)
	if err := WaitForTCPPort(mcpAddr, 30*time.Second, writer); err != nil {
		logsCmd := exec.Command("podman", "logs", cfg.Name)
		logsCmd.Stdout = writer
		logsCmd.Stderr = writer
		_ = logsCmd.Run()
		_ = StopGenericContainer(instance, writer)
		return nil, fmt.Errorf("MCP data-plane port never became ready: %w", err)
	}
	_, _ = fmt.Fprintf(writer, "   ✅ MCP data-plane port ready\n")

	return instance, nil
}

// StopEAIGWContainer stops and removes the EAIGW IT container.
func StopEAIGWContainer(instance *ContainerInstance, writer io.Writer) error {
	return StopGenericContainer(instance, writer)
}
