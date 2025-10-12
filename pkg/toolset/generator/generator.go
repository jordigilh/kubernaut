package generator

import (
	"context"

	"github.com/jordigilh/kubernaut/pkg/toolset"
)

// ToolsetGenerator generates toolset configurations from discovered services
// BR-TOOLSET-027: Toolset generation from discovered services
type ToolsetGenerator interface {
	// GenerateToolset creates a toolset configuration from discovered services
	// Returns JSON string in the appropriate format for the target system
	GenerateToolset(ctx context.Context, services []*toolset.DiscoveredService) (string, error)

	// ValidateToolset validates that a toolset configuration is well-formed
	ValidateToolset(ctx context.Context, toolsetJSON string) error
}
