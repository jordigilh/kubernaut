package generator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/toolset"
)

// jsonToolsetGenerator implements ToolsetGenerator for generic JSON format
// BR-TOOLSET-027: JSON toolset generation
// This format is generic and can be consumed by any tool (HolmesGPT API, UI, CLI, etc.)
type jsonToolsetGenerator struct{}

// NewJSONGenerator creates a new JSON toolset generator
func NewJSONGenerator() ToolsetGenerator {
	return &jsonToolsetGenerator{}
}

// Toolset represents the root toolset structure in generic JSON format
type Toolset struct {
	Tools []Tool `json:"tools"`
}

// Tool represents a single discovered tool/service in generic JSON format
// BR-TOOLSET-028: Tool structure requirements
type Tool struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Endpoint    string            `json:"endpoint"`
	Description string            `json:"description"`
	Namespace   string            `json:"namespace"`
	Metadata    map[string]string `json:"metadata"`
}

// GenerateToolset creates a generic JSON toolset from discovered services
func (g *jsonToolsetGenerator) GenerateToolset(ctx context.Context, services []*toolset.DiscoveredService) (string, error) {
	// Deduplicate services by name+namespace
	uniqueServices := g.deduplicateServices(services)

	// Convert to generic JSON format
	tools := make([]Tool, 0, len(uniqueServices))
	for _, svc := range uniqueServices {
		tool := Tool{
			Name:        svc.Name,
			Type:        svc.Type,
			Endpoint:    svc.Endpoint,
			Description: g.generateDescription(svc),
			Namespace:   svc.Namespace,
			Metadata:    svc.Metadata,
		}

		// Ensure metadata is never nil
		if tool.Metadata == nil {
			tool.Metadata = make(map[string]string)
		}

		tools = append(tools, tool)
	}

	// Create toolset structure
	toolset := Toolset{
		Tools: tools,
	}

	// Marshal to JSON
	jsonBytes, err := json.MarshalIndent(toolset, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal toolset to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// ValidateToolset validates that a toolset JSON is well-formed
func (g *jsonToolsetGenerator) ValidateToolset(ctx context.Context, toolsetJSON string) error {
	// First check if JSON has required structure
	var rawMap map[string]interface{}
	if err := json.Unmarshal([]byte(toolsetJSON), &rawMap); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// Check if "tools" key exists
	if _, ok := rawMap["tools"]; !ok {
		return fmt.Errorf("missing required field 'tools'")
	}

	// Parse into struct
	var toolset Toolset
	if err := json.Unmarshal([]byte(toolsetJSON), &toolset); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate required fields for each tool
	for i, tool := range toolset.Tools {
		if tool.Name == "" {
			return fmt.Errorf("tool[%d]: missing required field 'name'", i)
		}
		if tool.Type == "" {
			return fmt.Errorf("tool[%d]: missing required field 'type'", i)
		}
		if tool.Endpoint == "" {
			return fmt.Errorf("tool[%d]: missing required field 'endpoint'", i)
		}
		if tool.Description == "" {
			return fmt.Errorf("tool[%d]: missing required field 'description'", i)
		}
		// Namespace is optional - it's included for Kubernetes services but may not be present for all tool types
	}

	return nil
}

// deduplicateServices removes duplicate services based on name+namespace
func (g *jsonToolsetGenerator) deduplicateServices(services []*toolset.DiscoveredService) []*toolset.DiscoveredService {
	seen := make(map[string]bool)
	unique := make([]*toolset.DiscoveredService, 0, len(services))

	for _, svc := range services {
		key := svc.Namespace + "/" + svc.Name
		if !seen[key] {
			seen[key] = true
			unique = append(unique, svc)
		}
	}

	return unique
}

// generateDescription creates a human-readable description for a service
// BR-TOOLSET-028: Generate human-readable descriptions
func (g *jsonToolsetGenerator) generateDescription(svc *toolset.DiscoveredService) string {
	return fmt.Sprintf("%s service in %s namespace (type: %s)",
		svc.Name, svc.Namespace, svc.Type)
}
