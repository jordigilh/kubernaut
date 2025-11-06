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

package infrastructure

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ContextAPIInfrastructure manages the Context API Service test infrastructure
// This includes the Context API Service itself (requires Redis + Data Storage Service)
type ContextAPIInfrastructure struct {
	ServiceContainer string
	ConfigDir        string
	ServiceURL       string
}

// ContextAPIConfig contains configuration for the Context API Service
type ContextAPIConfig struct {
	RedisPort       string // Default: "6381"
	DataStoragePort string // Default: "8087"
	ServicePort     string // Default: "8088"
}

// DefaultContextAPIConfig returns default configuration
func DefaultContextAPIConfig() *ContextAPIConfig {
	return &ContextAPIConfig{
		RedisPort:       "6381",
		DataStoragePort: "8087",
		ServicePort:     "8088",
	}
}

// StartContextAPIInfrastructure starts the Context API Service
// Prerequisites: Redis and Data Storage Service must already be running
// Returns an infrastructure handle that can be used to stop the service
func StartContextAPIInfrastructure(cfg *ContextAPIConfig, writer io.Writer) (*ContextAPIInfrastructure, error) {
	if cfg == nil {
		cfg = DefaultContextAPIConfig()
	}

	infra := &ContextAPIInfrastructure{
		ServiceContainer: "contextapi-service-test",
		ServiceURL:       fmt.Sprintf("http://localhost:%s", cfg.ServicePort),
	}

	fmt.Fprintln(writer, "🔧 Setting up Context API Service infrastructure (Podman)")

	// 1. Create config directory
	fmt.Fprintln(writer, "📁 Creating config directory...")
	if err := createContextAPIConfigDir(infra, writer); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// 2. Create config file
	fmt.Fprintln(writer, "📝 Creating config file...")
	if err := createContextAPIConfig(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to create config file: %w", err)
	}

	// 3. Build Context API Docker image
	fmt.Fprintln(writer, "🔨 Building Context API Docker image...")
	if err := buildContextAPIImage(writer); err != nil {
		return nil, fmt.Errorf("failed to build Context API image: %w", err)
	}

	// 4. Start Context API Service
	fmt.Fprintln(writer, "🚀 Starting Context API Service container...")
	if err := startContextAPIService(infra, cfg, writer); err != nil {
		return nil, fmt.Errorf("failed to start Context API Service: %w", err)
	}

	// 5. Wait for service to be ready
	fmt.Fprintln(writer, "⏳ Waiting for Context API Service to be ready...")
	if err := waitForContextAPIReady(infra, writer); err != nil {
		return nil, fmt.Errorf("Context API Service failed to become ready: %w", err)
	}

	fmt.Fprintf(writer, "✅ Context API Service ready at %s\n", infra.ServiceURL)
	return infra, nil
}

// createContextAPIConfigDir creates a temporary directory for config files
func createContextAPIConfigDir(infra *ContextAPIInfrastructure, writer io.Writer) error {
	tmpDir, err := os.MkdirTemp("", "contextapi-test-config-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	infra.ConfigDir = tmpDir
	fmt.Fprintf(writer, "   Config directory: %s\n", tmpDir)
	return nil
}

// createContextAPIConfig creates the config.yaml file for Context API
func createContextAPIConfig(infra *ContextAPIInfrastructure, cfg *ContextAPIConfig, writer io.Writer) error {
	configContent := fmt.Sprintf(`# Context API Test Configuration (ADR-032: No direct DB access)
server:
  port: %s
  host: "0.0.0.0"
  read_timeout: 30s
  write_timeout: 30s

cache:
  redis_addr: "host.containers.internal:%s"
  redis_db: 0
  lru_size: 1000
  default_ttl: 5m

data_storage:
  base_url: "http://host.containers.internal:%s"
  timeout: 30s

logging:
  level: debug
  format: json
`, cfg.ServicePort, cfg.RedisPort, cfg.DataStoragePort)

	configPath := filepath.Join(infra.ConfigDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Fprintf(writer, "   Config file: %s\n", configPath)
	return nil
}

// buildContextAPIImage builds the Context API Docker image
func buildContextAPIImage(writer io.Writer) error {
	// Find workspace root
	workspaceRoot, err := findWorkspaceRoot()
	if err != nil {
		return fmt.Errorf("failed to find workspace root: %w", err)
	}

	// Build Docker image
	buildCmd := exec.Command("podman", "build",
		"-t", "contextapi-test:latest",
		"-f", "docker/context-api.Dockerfile",
		".",
	)
	buildCmd.Dir = workspaceRoot
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Run(); err != nil {
		return fmt.Errorf("podman build failed: %w", err)
	}

	return nil
}

// startContextAPIService starts the Context API Service container
func startContextAPIService(infra *ContextAPIInfrastructure, cfg *ContextAPIConfig, writer io.Writer) error {
	// Stop and remove existing container
	stopCmd := exec.Command("podman", "rm", "-f", infra.ServiceContainer)
	stopCmd.Stdout = writer
	stopCmd.Stderr = writer
	_ = stopCmd.Run() // Ignore error if container doesn't exist

	// Start new container
	startCmd := exec.Command("podman", "run",
		"-d",
		"--name", infra.ServiceContainer,
		"-p", fmt.Sprintf("%s:8091", cfg.ServicePort), // Map service port to host
		"-p", "9090:9090",                             // Map metrics port
		"-v", fmt.Sprintf("%s:/etc/contextapi:ro", infra.ConfigDir),
		"-e", "CONFIG_FILE=/etc/contextapi/config.yaml",
		"contextapi-test:latest",
	)
	startCmd.Stdout = writer
	startCmd.Stderr = writer

	if err := startCmd.Run(); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}

	fmt.Fprintf(writer, "   Container: %s\n", infra.ServiceContainer)
	return nil
}

// waitForContextAPIReady waits for the Context API Service to be ready
func waitForContextAPIReady(infra *ContextAPIInfrastructure, writer io.Writer) error {
	healthURL := infra.ServiceURL + "/health"
	maxAttempts := 30
	delay := 1 * time.Second

	for i := 0; i < maxAttempts; i++ {
		resp, err := http.Get(healthURL)
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			fmt.Fprintf(writer, "   Health check passed after %d attempts\n", i+1)
			return nil
		}
		if resp != nil {
			resp.Body.Close()
		}

		if i < maxAttempts-1 {
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("Context API Service did not become ready after %d attempts", maxAttempts)
}

// Stop stops the Context API Service infrastructure
func (infra *ContextAPIInfrastructure) Stop(writer io.Writer) {
	fmt.Fprintln(writer, "🧹 Stopping Context API Service infrastructure...")

	// Stop and remove container
	if infra.ServiceContainer != "" {
		stopCmd := exec.Command("podman", "rm", "-f", infra.ServiceContainer)
		stopCmd.Stdout = writer
		stopCmd.Stderr = writer
		if err := stopCmd.Run(); err != nil {
			fmt.Fprintf(writer, "⚠️  Failed to stop container %s: %v\n", infra.ServiceContainer, err)
		} else {
			fmt.Fprintf(writer, "   Stopped container: %s\n", infra.ServiceContainer)
		}
	}

	// Remove config directory
	if infra.ConfigDir != "" {
		if err := os.RemoveAll(infra.ConfigDir); err != nil {
			fmt.Fprintf(writer, "⚠️  Failed to remove config directory %s: %v\n", infra.ConfigDir, err)
		} else {
			fmt.Fprintf(writer, "   Removed config directory: %s\n", infra.ConfigDir)
		}
	}

	fmt.Fprintln(writer, "✅ Context API Service infrastructure stopped")
}

// Note: findWorkspaceRoot is defined in datastorage.go (shared helper)

// IsContextAPIReady checks if the Context API Service is ready
func IsContextAPIReady(serviceURL string) error {
	resp, err := http.Get(serviceURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// GetContextAPILogs retrieves logs from the Context API Service container
func GetContextAPILogs(containerName string) (string, error) {
	cmd := exec.Command("podman", "logs", containerName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}
