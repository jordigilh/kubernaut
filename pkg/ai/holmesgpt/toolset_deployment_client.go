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

package holmesgpt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"

	sharedHTTP "github.com/jordigilh/kubernaut/pkg/shared/http"
)

// ToolsetDeploymentClient defines the interface for HolmesGPT toolset deployment
// Business Requirements: BR-HOLMES-001, BR-HOLMES-002, BR-EXTERNAL-001
type ToolsetDeploymentClient interface {
	// BR-HOLMES-001: MUST provide HolmesGPT with custom Kubernaut toolset for context orchestration
	DeployToolset(ctx context.Context, toolset *ToolsetConfig) (*ToolsetDeploymentResponse, error)

	// BR-HOLMES-003: MUST support toolset function discovery and capability enumeration
	DiscoverToolsets(ctx context.Context) ([]ToolsetInfo, error)
	GetToolsetDetails(ctx context.Context, toolsetName string) (*DetailedToolsetInfo, error)

	// BR-HOLMES-004: MUST provide toolset function documentation and usage examples
	GetToolsetDocumentation(ctx context.Context, toolsetName string) (*ToolsetDocumentation, error)

	// BR-HOLMES-005: MUST enable toolset function chaining for complex context gathering workflows
	ValidateToolChain(ctx context.Context, chain *ToolChainDefinition) (*ToolChainValidationResponse, error)
}

// ToolsetDeploymentResponse represents the response from toolset deployment
// Business Requirement: BR-HOLMES-001 - Deployment confirmation
type ToolsetDeploymentResponse struct {
	Success            bool      `json:"success"`
	ToolsetName        string    `json:"toolset_name"`
	FrameworkVersion   string    `json:"framework_version,omitempty"`  // BR-EXTERNAL-001: v0.13.1+ support
	CompatibilityMode  string    `json:"compatibility_mode,omitempty"` // BR-EXTERNAL-001: Integration mode
	DeployedAt         time.Time `json:"deployed_at"`
	Message            string    `json:"message"`
	Error              string    `json:"error,omitempty"`
	ToolsCount         int       `json:"tools_count"`
	ValidationWarnings []string  `json:"validation_warnings,omitempty"`
}

// ToolsetInfo provides basic information about available toolsets
// Business Requirement: BR-HOLMES-003 - Toolset discovery
type ToolsetInfo struct {
	Name         string   `json:"name"`
	ServiceType  string   `json:"service_type"`
	Capabilities []string `json:"capabilities"`
	ToolCount    int      `json:"tool_count"`
	Status       string   `json:"status"`
	Version      string   `json:"version,omitempty"`
	Priority     int      `json:"priority,omitempty"`
}

// DetailedToolsetInfo provides comprehensive toolset information
// Business Requirement: BR-HOLMES-003 - Capability enumeration
type DetailedToolsetInfo struct {
	Name         string          `json:"name"`
	ServiceType  string          `json:"service_type"`
	Description  string          `json:"description"`
	Version      string          `json:"version"`
	Capabilities []string        `json:"capabilities"`
	Tools        []HolmesGPTTool `json:"tools"`
	Status       string          `json:"status"`
	LastUpdated  time.Time       `json:"last_updated,omitempty"`
	Metadata     ServiceMetadata `json:"metadata,omitempty"`
}

// ToolsetDocumentation provides comprehensive function documentation
// Business Requirement: BR-HOLMES-004 - Function documentation and usage examples
type ToolsetDocumentation struct {
	Name            string                 `json:"name"`
	Description     string                 `json:"description"`
	Version         string                 `json:"version"`
	Tools           []ToolDocumentation    `json:"tools"`
	UsageGuidelines string                 `json:"usage_guidelines"`
	BestPractices   []string               `json:"best_practices"`
	Examples        []WorkflowExample      `json:"examples,omitempty"`
	Troubleshooting []TroubleshootingGuide `json:"troubleshooting,omitempty"`
}

// ToolDocumentation provides detailed documentation for individual tools
type ToolDocumentation struct {
	Name        string                   `json:"name"`
	Description string                   `json:"description"`
	Usage       string                   `json:"usage"`
	Parameters  []ParameterDocumentation `json:"parameters"`
	Examples    []ToolExample            `json:"examples"`
	Returns     *ReturnDocumentation     `json:"returns,omitempty"`
	Errors      []ErrorDocumentation     `json:"errors,omitempty"`
	SeeAlso     []string                 `json:"see_also,omitempty"`
}

// ParameterDocumentation documents tool parameters
type ParameterDocumentation struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Type         string   `json:"type"`
	Required     bool     `json:"required"`
	Default      string   `json:"default,omitempty"`
	Examples     []string `json:"examples,omitempty"`
	Validation   string   `json:"validation,omitempty"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// ReturnDocumentation describes tool return values
type ReturnDocumentation struct {
	Type        string            `json:"type"`
	Description string            `json:"description"`
	Schema      map[string]string `json:"schema,omitempty"`
	Examples    []interface{}     `json:"examples,omitempty"`
}

// ErrorDocumentation describes possible tool errors
type ErrorDocumentation struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Description string `json:"description"`
	Resolution  string `json:"resolution,omitempty"`
}

// WorkflowExample provides workflow-level usage examples
type WorkflowExample struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Steps       []string `json:"steps"`
	Tools       []string `json:"tools_used"`
	Scenario    string   `json:"scenario"`
}

// TroubleshootingGuide provides troubleshooting information
type TroubleshootingGuide struct {
	Issue      string   `json:"issue"`
	Symptoms   []string `json:"symptoms"`
	Causes     []string `json:"causes"`
	Solutions  []string `json:"solutions"`
	Prevention string   `json:"prevention,omitempty"`
}

// ToolChainDefinition defines a chain of tool executions
// Business Requirement: BR-HOLMES-005 - Function chaining for complex workflows
type ToolChainDefinition struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Steps       []ToolChainStep  `json:"steps"`
	Conditions  []ChainCondition `json:"conditions,omitempty"`
	ErrorPolicy string           `json:"error_policy"` // "fail_fast", "continue", "retry"
}

// ToolChainStep defines a single step in a tool chain
type ToolChainStep struct {
	StepID     string            `json:"step_id"`
	ToolName   string            `json:"tool_name"`
	Parameters map[string]string `json:"parameters"`
	DependsOn  []string          `json:"depends_on,omitempty"`
	OutputVar  string            `json:"output_var,omitempty"`
	Optional   bool              `json:"optional,omitempty"`
	Retry      *RetryPolicy      `json:"retry,omitempty"`
}

// ChainCondition defines conditional execution logic
type ChainCondition struct {
	If   string   `json:"if"`             // Condition expression
	Then []string `json:"then"`           // Steps to execute if condition is true
	Else []string `json:"else,omitempty"` // Steps to execute if condition is false
}

// RetryPolicy defines retry behavior for tool chain steps
type RetryPolicy struct {
	MaxRetries int           `json:"max_retries"`
	Delay      time.Duration `json:"delay"`
	Backoff    string        `json:"backoff"` // "linear", "exponential"
}

// ToolChainValidationResponse provides validation results for tool chains
type ToolChainValidationResponse struct {
	Valid       bool                   `json:"valid"`
	Errors      []ValidationError      `json:"errors,omitempty"`
	Warnings    []ValidationWarning    `json:"warnings,omitempty"`
	Suggestions []ValidationSuggestion `json:"suggestions,omitempty"`
}

// ValidationError represents a validation error in tool chains
type ValidationError struct {
	StepID  string `json:"step_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Field   string `json:"field,omitempty"`
}

// ValidationWarning represents a validation warning
type ValidationWarning struct {
	StepID  string `json:"step_id,omitempty"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Impact  string `json:"impact"` // "low", "medium", "high"
}

// ValidationSuggestion provides optimization suggestions
type ValidationSuggestion struct {
	Type        string `json:"type"` // "optimization", "best_practice", "alternative"
	Message     string `json:"message"`
	Improvement string `json:"improvement,omitempty"`
}

// ToolsetDeploymentClientImpl implements the ToolsetDeploymentClient interface
// Following project guideline: Reuse existing HTTP client patterns
type ToolsetDeploymentClientImpl struct {
	baseURL    string
	httpClient *http.Client
	logger     *logrus.Logger
}

// NewToolsetDeploymentClient creates a new toolset deployment client
// Business Requirements: BR-HOLMES-001, BR-EXTERNAL-001
// Following project guideline: Reuse existing patterns from shared/http
func NewToolsetDeploymentClient(baseURL string, logger *logrus.Logger) ToolsetDeploymentClient {
	// Use existing HTTP client configuration pattern
	config := sharedHTTP.LLMClientConfig(30 * time.Second) // Appropriate timeout for AI services
	httpClient := sharedHTTP.NewClient(config)

	return &ToolsetDeploymentClientImpl{
		baseURL:    baseURL,
		httpClient: httpClient,
		logger:     logger,
	}
}

// DeployToolset deploys a custom toolset to HolmesGPT
// Business Requirement: BR-HOLMES-001 - Custom Kubernaut toolset deployment
func (c *ToolsetDeploymentClientImpl) DeployToolset(ctx context.Context, toolset *ToolsetConfig) (*ToolsetDeploymentResponse, error) {
	c.logger.WithFields(logrus.Fields{
		"toolset_name": toolset.Name,
		"service_type": toolset.ServiceType,
		"tool_count":   len(toolset.Tools),
	}).Info("BR-HOLMES-001: Deploying custom Kubernaut toolset to HolmesGPT")

	// Validate toolset configuration
	if err := c.validateToolsetConfig(toolset); err != nil {
		c.logger.WithError(err).Error("BR-HOLMES-001: Toolset validation failed")
		// Following project guidelines: provide structured response for validation failures
		return &ToolsetDeploymentResponse{
			Success: false,
			Message: fmt.Sprintf("Toolset validation failed: %v", err),
			Error:   "TOOLSET_VALIDATION_ERROR",
		}, fmt.Errorf("toolset validation failed: %w", err)
	}

	// Prepare request payload
	payload, err := json.Marshal(toolset)
	if err != nil {
		c.logger.WithError(err).Error("BR-HOLMES-001: Failed to serialize toolset configuration")
		// Following project guidelines: provide structured response for serialization failures
		return &ToolsetDeploymentResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to serialize toolset: %v", err),
			Error:   "SERIALIZATION_ERROR",
		}, fmt.Errorf("failed to serialize toolset: %w", err)
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/api/v1/toolsets", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		c.logger.WithError(err).Error("BR-HOLMES-001: Failed to create HTTP request")
		// Following project guidelines: provide structured response for request creation failures
		return &ToolsetDeploymentResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to create HTTP request: %v", err),
			Error:   "REQUEST_CREATION_ERROR",
		}, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	// BR-EXTERNAL-001: Set version compatibility header
	req.Header.Set("X-HolmesGPT-API-Version", "0.13.1")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).Error("BR-HOLMES-001: Failed to deploy toolset to HolmesGPT")
		// Following project guidelines: provide structured response even for network failures
		return &ToolsetDeploymentResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to deploy toolset: %v", err),
			Error:   "NETWORK_ERROR",
		}, fmt.Errorf("failed to deploy toolset: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.WithError(err).Debug("Failed to close response body")
		}
	}()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.logger.WithError(err).Error("BR-HOLMES-001: Failed to read deployment response")
		// Following project guidelines: provide structured response even for infrastructure failures
		return &ToolsetDeploymentResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to read response: %v", err),
			Error:   "RESPONSE_READ_ERROR",
		}, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse response
	var response ToolsetDeploymentResponse
	if err := json.Unmarshal(body, &response); err != nil {
		c.logger.WithError(err).Error("BR-HOLMES-001: Failed to parse deployment response")
		// Following project guidelines: provide structured response even for parsing failures
		return &ToolsetDeploymentResponse{
			Success: false,
			Message: fmt.Sprintf("Failed to parse response: %v", err),
			Error:   "RESPONSE_PARSE_ERROR",
		}, fmt.Errorf("failed to parse response: %w", err)
	}

	// Handle deployment failure
	if resp.StatusCode >= 400 {
		c.logger.WithFields(logrus.Fields{
			"status_code": resp.StatusCode,
			"error":       response.Error,
			"message":     response.Message,
		}).Error("BR-HOLMES-001: Toolset deployment failed")

		return &response, fmt.Errorf("deployment failed: %s", response.Message)
	}

	// Log successful deployment
	c.logger.WithFields(logrus.Fields{
		"business_requirement": "BR-HOLMES-001",
		"toolset_name":         response.ToolsetName,
		"tools_deployed":       response.ToolsCount,
		"framework_version":    response.FrameworkVersion,
	}).Info("BR-HOLMES-001: Custom Kubernaut toolset deployed successfully")

	return &response, nil
}

// DiscoverToolsets discovers available toolsets in HolmesGPT
// Business Requirement: BR-HOLMES-003 - Toolset function discovery
func (c *ToolsetDeploymentClientImpl) DiscoverToolsets(ctx context.Context) ([]ToolsetInfo, error) {
	c.logger.Debug("BR-HOLMES-003: Discovering available toolsets from HolmesGPT")

	url := fmt.Sprintf("%s/api/v1/toolsets", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create discovery request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-HolmesGPT-API-Version", "0.13.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).Error("BR-HOLMES-003: Failed to discover toolsets")
		return nil, fmt.Errorf("failed to discover toolsets: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.WithError(err).Debug("Failed to close response body")
		}
	}()

	var result struct {
		Toolsets []ToolsetInfo `json:"toolsets"`
		Total    int           `json:"total"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse discovery response: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"business_requirement": "BR-HOLMES-003",
		"discovered_toolsets":  result.Total,
	}).Info("BR-HOLMES-003: Toolset discovery completed")

	return result.Toolsets, nil
}

// GetToolsetDetails retrieves detailed information about a specific toolset
// Business Requirement: BR-HOLMES-003 - Capability enumeration
func (c *ToolsetDeploymentClientImpl) GetToolsetDetails(ctx context.Context, toolsetName string) (*DetailedToolsetInfo, error) {
	c.logger.WithField("toolset_name", toolsetName).Debug("BR-HOLMES-003: Retrieving detailed toolset information")

	url := fmt.Sprintf("%s/api/v1/toolsets/%s", c.baseURL, toolsetName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create details request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-HolmesGPT-API-Version", "0.13.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).Error("BR-HOLMES-003: Failed to retrieve toolset details")
		return nil, fmt.Errorf("failed to retrieve toolset details: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.WithError(err).Debug("Failed to close response body")
		}
	}()

	var details DetailedToolsetInfo
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to parse details response: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"business_requirement": "BR-HOLMES-003",
		"toolset_name":         details.Name,
		"tool_count":           len(details.Tools),
		"capabilities":         details.Capabilities,
	}).Info("BR-HOLMES-003: Toolset details retrieved successfully")

	return &details, nil
}

// GetToolsetDocumentation retrieves comprehensive documentation for a toolset
// Business Requirement: BR-HOLMES-004 - Function documentation and usage examples
func (c *ToolsetDeploymentClientImpl) GetToolsetDocumentation(ctx context.Context, toolsetName string) (*ToolsetDocumentation, error) {
	c.logger.WithField("toolset_name", toolsetName).Debug("BR-HOLMES-004: Retrieving toolset documentation")

	url := fmt.Sprintf("%s/api/v1/toolsets/%s/documentation", c.baseURL, toolsetName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create documentation request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-HolmesGPT-API-Version", "0.13.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).Error("BR-HOLMES-004: Failed to retrieve toolset documentation")
		return nil, fmt.Errorf("failed to retrieve documentation: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.WithError(err).Debug("Failed to close response body")
		}
	}()

	var docs ToolsetDocumentation
	if err := json.NewDecoder(resp.Body).Decode(&docs); err != nil {
		return nil, fmt.Errorf("failed to parse documentation response: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"business_requirement": "BR-HOLMES-004",
		"toolset_name":         docs.Name,
		"documented_tools":     len(docs.Tools),
		"best_practices":       len(docs.BestPractices),
	}).Info("BR-HOLMES-004: Toolset documentation retrieved successfully")

	return &docs, nil
}

// ValidateToolChain validates a tool chain definition
// Business Requirement: BR-HOLMES-005 - Function chaining for complex workflows
func (c *ToolsetDeploymentClientImpl) ValidateToolChain(ctx context.Context, chain *ToolChainDefinition) (*ToolChainValidationResponse, error) {
	c.logger.WithFields(logrus.Fields{
		"chain_name": chain.Name,
		"step_count": len(chain.Steps),
	}).Debug("BR-HOLMES-005: Validating tool chain definition")

	payload, err := json.Marshal(chain)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize tool chain: %w", err)
	}

	url := fmt.Sprintf("%s/api/v1/toolchains/validate", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create validation request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-HolmesGPT-API-Version", "0.13.1")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.WithError(err).Error("BR-HOLMES-005: Failed to validate tool chain")
		return nil, fmt.Errorf("failed to validate tool chain: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.logger.WithError(err).Debug("Failed to close response body")
		}
	}()

	var validation ToolChainValidationResponse
	if err := json.NewDecoder(resp.Body).Decode(&validation); err != nil {
		return nil, fmt.Errorf("failed to parse validation response: %w", err)
	}

	c.logger.WithFields(logrus.Fields{
		"business_requirement": "BR-HOLMES-005",
		"chain_name":           chain.Name,
		"valid":                validation.Valid,
		"error_count":          len(validation.Errors),
		"warning_count":        len(validation.Warnings),
	}).Info("BR-HOLMES-005: Tool chain validation completed")

	return &validation, nil
}

// validateToolsetConfig validates toolset configuration before deployment
// Following project guideline: Proper error handling and validation
func (c *ToolsetDeploymentClientImpl) validateToolsetConfig(toolset *ToolsetConfig) error {
	if toolset.Name == "" {
		return fmt.Errorf("toolset name is required")
	}

	if toolset.ServiceType == "" {
		return fmt.Errorf("service type is required")
	}

	if len(toolset.Tools) == 0 {
		return fmt.Errorf("at least one tool is required")
	}

	// Validate individual tools
	for i, tool := range toolset.Tools {
		if tool.Name == "" {
			return fmt.Errorf("tool %d: name is required", i)
		}
		if tool.Description == "" {
			return fmt.Errorf("tool %d (%s): description is required", i, tool.Name)
		}
		if tool.Category == "" {
			return fmt.Errorf("tool %d (%s): category is required", i, tool.Name)
		}

		// Validate parameters
		for j, param := range tool.Parameters {
			if param.Name == "" {
				return fmt.Errorf("tool %d (%s), parameter %d: name is required", i, tool.Name, j)
			}
			if param.Type == "" {
				return fmt.Errorf("tool %d (%s), parameter %d (%s): type is required", i, tool.Name, j, param.Name)
			}
		}
	}

	return nil
}
