package generator

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jordigilh/kubernaut/pkg/toolset"
)

// holmesGPTGenerator implements ToolsetGenerator for HolmesGPT format
// BR-TOOLSET-027: HolmesGPT toolset JSON generation
type holmesGPTGenerator struct{}

// NewHolmesGPTGenerator creates a new HolmesGPT toolset generator
func NewHolmesGPTGenerator() ToolsetGenerator {
	return &holmesGPTGenerator{}
}

// HolmesGPTToolset represents the root toolset structure
type HolmesGPTToolset struct {
	Tools []HolmesGPTTool `json:"tools"`
}

// HolmesGPTTool represents a single tool in the HolmesGPT format
// BR-TOOLSET-028: HolmesGPT tool structure requirements
type HolmesGPTTool struct {
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	Endpoint    string            `json:"endpoint"`
	Description string            `json:"description"`
	Namespace   string            `json:"namespace"`
	Metadata    map[string]string `json:"metadata"`
}

// GenerateToolset creates a HolmesGPT toolset JSON from discovered services
func (g *holmesGPTGenerator) GenerateToolset(ctx context.Context, services []*toolset.DiscoveredService) (string, error) {
	// Deduplicate services by name+namespace
	uniqueServices := g.deduplicateServices(services)

	// Convert to HolmesGPT format
	tools := make([]HolmesGPTTool, 0, len(uniqueServices))
	for _, svc := range uniqueServices {
		tool := HolmesGPTTool{
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
	toolset := HolmesGPTToolset{
		Tools: tools,
	}

	// Marshal to JSON
	jsonBytes, err := json.MarshalIndent(toolset, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal toolset to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// ValidateToolset validates that a toolset JSON is well-formed for HolmesGPT
func (g *holmesGPTGenerator) ValidateToolset(ctx context.Context, toolsetJSON string) error {
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
	var toolset HolmesGPTToolset
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
		if tool.Namespace == "" {
			return fmt.Errorf("tool[%d]: missing required field 'namespace'", i)
		}
	}

	return nil
}

// deduplicateServices removes duplicate services based on name+namespace
func (g *holmesGPTGenerator) deduplicateServices(services []*toolset.DiscoveredService) []*toolset.DiscoveredService {
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
func (g *holmesGPTGenerator) generateDescription(svc *toolset.DiscoveredService) string {
	return fmt.Sprintf("%s service in %s namespace (type: %s)",
		svc.Name, svc.Namespace, svc.Type)
}
