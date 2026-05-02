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

package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	mcpinternal "github.com/jordigilh/kubernaut/internal/kubernautagent/mcp"
	"github.com/jordigilh/kubernaut/pkg/shared/transport"
)

// EnrichmentRunner abstracts the enrichment call for testability.
type EnrichmentRunner interface {
	Enrich(ctx context.Context, kind, name, namespace, specHash, incidentID string) (*enrichment.EnrichmentResult, error)
}

// EnrichInput defines the input schema for the kubernaut_enrich MCP tool.
type EnrichInput struct {
	RRID       string `json:"rr_id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	SpecHash   string `json:"spec_hash,omitempty"`
	IncidentID string `json:"incident_id,omitempty"`
}

// EnrichOutput defines the output schema for the kubernaut_enrich MCP tool.
type EnrichOutput struct {
	Status string                       `json:"status"`
	Result *enrichment.EnrichmentResult `json:"result,omitempty"`
}

// EnrichTool handles the kubernaut_enrich MCP tool.
// BR-INTERACTIVE-003: enables interactive resource enrichment.
type EnrichTool struct {
	runner   EnrichmentRunner
	sessions mcpinternal.SessionManager
}

// NewEnrichTool creates the tool handler with its dependencies.
func NewEnrichTool(runner EnrichmentRunner, sessions mcpinternal.SessionManager) *EnrichTool {
	return &EnrichTool{runner: runner, sessions: sessions}
}

// Handle executes the enrichment request after validating input and session.
func (t *EnrichTool) Handle(ctx context.Context, input EnrichInput, user mcpinternal.UserInfo) (EnrichOutput, error) {
	if err := validateEnrichInput(input); err != nil {
		return EnrichOutput{}, err
	}

	if !t.sessions.IsDriverActive(input.RRID) {
		return EnrichOutput{}, fmt.Errorf("no active interactive session for rr_id")
	}

	driver, err := t.sessions.GetDriver(input.RRID)
	if err != nil || driver == nil {
		return EnrichOutput{}, fmt.Errorf("no active interactive session for rr_id")
	}

	if driver.ActingUser.Username != user.Username {
		return EnrichOutput{}, fmt.Errorf("caller is not the active driver for this session")
	}

	// SEC-06 (#703): Enrich context with user identity for impersonation.
	ctx = transport.WithImpersonatedUser(ctx, user.Username, user.Groups)

	result, err := t.runner.Enrich(ctx, input.Kind, input.Name, input.Namespace, input.SpecHash, input.IncidentID)
	if err != nil {
		if errors.Is(err, enrichment.ErrRBACForbidden) {
			return EnrichOutput{}, ErrCodeForbidden.WithDetail("namespace", input.Namespace)
		}
		return EnrichOutput{}, fmt.Errorf("enrich failed: %w", err)
	}

	return EnrichOutput{
		Status: "enriched",
		Result: result,
	}, nil
}

func validateEnrichInput(input EnrichInput) error {
	if input.RRID == "" {
		return fmt.Errorf("rr_id is required")
	}
	if input.Kind == "" {
		return fmt.Errorf("kind is required")
	}
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if input.Namespace == "" {
		return fmt.Errorf("namespace is required")
	}
	return nil
}
