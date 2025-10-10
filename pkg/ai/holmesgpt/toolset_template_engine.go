<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package holmesgpt

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"unicode"

	"github.com/sirupsen/logrus"
)

// ToolsetTemplateEngine provides templating capabilities for toolset generation
// Business Requirement: BR-HOLMES-023 - Toolset configuration templates
type ToolsetTemplateEngine struct {
	mu        sync.RWMutex // Protects concurrent access to templates map
	templates map[string]*template.Template
	log       *logrus.Logger
}

// TemplateVariables contains variables for toolset template rendering
type TemplateVariables struct {
	ServiceName  string            `json:"service_name"`
	Namespace    string            `json:"namespace"`
	Endpoints    map[string]string `json:"endpoints"`
	Capabilities []string          `json:"capabilities"`
	ServiceType  string            `json:"service_type"`
	Labels       map[string]string `json:"labels,omitempty"`
	Annotations  map[string]string `json:"annotations,omitempty"`
}

// NewToolsetTemplateEngine creates a new toolset template engine
func NewToolsetTemplateEngine(log *logrus.Logger) *ToolsetTemplateEngine {
	engine := &ToolsetTemplateEngine{
		templates: make(map[string]*template.Template),
		log:       log,
	}

	// Load default templates
	engine.loadDefaultTemplates()

	return engine
}

// RenderTemplate renders a template with the provided variables
func (tte *ToolsetTemplateEngine) RenderTemplate(templateName string, variables TemplateVariables) (string, error) {
	tte.mu.RLock()
	tmpl, exists := tte.templates[templateName]
	tte.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("template %s not found", templateName)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, variables); err != nil {
		return "", fmt.Errorf("failed to execute template %s: %w", templateName, err)
	}

	return buf.String(), nil
}

// RenderToolCommand renders a tool command template
// Uses shell-style variable substitution (${var}) for tool commands per BR-HOLMES-023
func (tte *ToolsetTemplateEngine) RenderToolCommand(commandTemplate string, variables map[string]string) (string, error) {
	// Validate template syntax first
	if err := tte.validateCommandTemplateSyntax(commandTemplate); err != nil {
		return "", fmt.Errorf("failed to parse command template: %w", err)
	}

	// Replace known variables
	result := commandTemplate
	for key, value := range variables {
		placeholder := "${" + key + "}"
		result = strings.ReplaceAll(result, placeholder, value)
	}

	// Handle missing variables by replacing any remaining ${...} with empty strings
	// This supports the test case for missing variables
	// Business Requirement: BR-HOLMES-023 - Toolset configuration templates
	for strings.Contains(result, "${") {
		start := strings.Index(result, "${")
		end := strings.Index(result[start:], "}")
		if end == -1 {
			// This shouldn't happen after validation, but just in case
			return "", fmt.Errorf("failed to parse command template: unclosed variable at position %d", start)
		}
		end += start                             // Convert to absolute position
		result = result[:start] + result[end+1:] // Remove ${var} including the }
	}

	return result, nil
}

// validateCommandTemplateSyntax validates shell-style template syntax
func (tte *ToolsetTemplateEngine) validateCommandTemplateSyntax(template string) error {
	// Check for basic syntax issues in shell-style templates
	pos := 0
	for {
		start := strings.Index(template[pos:], "${")
		if start == -1 {
			break
		}
		start += pos
		end := strings.Index(template[start:], "}")
		if end == -1 {
			return fmt.Errorf("unclosed template variable starting at position %d", start)
		}
		pos = start + end + 1
	}

	return nil
}

// AddTemplate adds a custom template to the engine
func (tte *ToolsetTemplateEngine) AddTemplate(name, templateContent string) error {
	tmpl, err := template.New(name).Funcs(tte.GetTemplateHelpers()).Parse(templateContent)
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", name, err)
	}

	tte.mu.Lock()
	tte.templates[name] = tmpl
	tte.mu.Unlock()

	tte.log.WithField("template_name", name).Debug("Added custom template")
	return nil
}

// GetAvailableTemplates returns the names of all available templates
func (tte *ToolsetTemplateEngine) GetAvailableTemplates() []string {
	tte.mu.RLock()
	names := make([]string, 0, len(tte.templates))
	for name := range tte.templates {
		names = append(names, name)
	}
	tte.mu.RUnlock()
	return names
}

// loadDefaultTemplates loads the default toolset templates
func (tte *ToolsetTemplateEngine) loadDefaultTemplates() {
	// Prometheus toolset template
	prometheusTemplate := `{
	"name": "prometheus-{{.Namespace}}-{{.ServiceName}}",
	"service_type": "prometheus",
	"description": "Prometheus metrics analysis tools for {{.ServiceName}}",
	"version": "1.0.0",
	"endpoints": {
		"query": "{{index .Endpoints "query"}}",
		"query_range": "{{index .Endpoints "query_range"}}",
		"targets": "{{index .Endpoints "targets"}}"
	},
	"capabilities": {{.Capabilities}},
	"enabled": true
}`

	// Grafana toolset template
	grafanaTemplate := `{
	"name": "grafana-{{.Namespace}}-{{.ServiceName}}",
	"service_type": "grafana",
	"description": "Grafana dashboard and visualization tools for {{.ServiceName}}",
	"version": "1.0.0",
	"endpoints": {
		"api": "{{index .Endpoints "api"}}",
		"dashboards": "{{index .Endpoints "dashboards"}}",
		"datasources": "{{index .Endpoints "datasources"}}"
	},
	"capabilities": {{.Capabilities}},
	"enabled": true
}`

	// Jaeger toolset template
	jaegerTemplate := `{
	"name": "jaeger-{{.Namespace}}-{{.ServiceName}}",
	"service_type": "jaeger",
	"description": "Jaeger distributed tracing analysis tools for {{.ServiceName}}",
	"version": "1.0.0",
	"endpoints": {
		"api": "{{index .Endpoints "api"}}",
		"traces": "{{index .Endpoints "traces"}}",
		"services": "{{index .Endpoints "services"}}"
	},
	"capabilities": {{.Capabilities}},
	"enabled": true
}`

	// Custom toolset template
	customTemplate := `{
	"name": "{{.ServiceType}}-{{.Namespace}}-{{.ServiceName}}",
	"service_type": "{{.ServiceType}}",
	"description": "Custom {{.ServiceType}} tools for {{.ServiceName}}",
	"version": "1.0.0",
	"endpoints": {{.Endpoints}},
	"capabilities": {{.Capabilities}},
	"enabled": true
}`

	// Add templates to the engine
	templates := map[string]string{
		"prometheus":    prometheusTemplate,
		"grafana":       grafanaTemplate,
		"jaeger":        jaegerTemplate,
		"elasticsearch": customTemplate, // Reuse custom template for elasticsearch
		"custom":        customTemplate,
	}

	for name, content := range templates {
		if err := tte.AddTemplate(name, content); err != nil {
			tte.log.WithError(err).WithField("template_name", name).Error("Failed to load default template")
		}
	}

	tte.log.WithField("template_count", len(templates)).Debug("Loaded default templates")
}

// GenerateToolsetConfig generates a toolset configuration using templates
func (tte *ToolsetTemplateEngine) GenerateToolsetConfig(serviceType string, variables TemplateVariables) (*ToolsetConfig, error) {
	// This is a simplified version - in practice, you would render the template
	// and then unmarshal it into a ToolsetConfig struct

	// Guideline #14: Remove ineffectual assignments - template name logic removed since not used
	// TODO: Implement actual template rendering using template name when needed

	// For now, return a basic config based on the variables
	// In a full implementation, this would render the JSON template and unmarshal it

	// Handle hyphenated service types by capitalizing each part
	serviceTypeTitle := strings.ReplaceAll(toTitle(strings.ReplaceAll(variables.ServiceType, "-", " ")), " ", "-")

	config := &ToolsetConfig{
		Name:         fmt.Sprintf("%s-%s-%s", variables.ServiceType, variables.Namespace, variables.ServiceName),
		ServiceType:  variables.ServiceType,
		Description:  fmt.Sprintf("%s tools for %s", serviceTypeTitle, variables.ServiceName),
		Version:      "1.0.0",
		Endpoints:    variables.Endpoints,
		Capabilities: variables.Capabilities,
		Enabled:      true,
		ServiceMeta: ServiceMetadata{
			Namespace:   variables.Namespace,
			ServiceName: variables.ServiceName,
			Labels:      variables.Labels,
			Annotations: variables.Annotations,
		},
	}

	return config, nil
}

// ValidateTemplate validates a template for syntax errors
// Business Requirement: BR-HOLMES-023 - Comprehensive template validation
func (tte *ToolsetTemplateEngine) ValidateTemplate(templateContent string) error {
	// Use Go template parser for comprehensive validation
	// This catches all syntax errors including unclosed braces, invalid constructs, etc.
	_, err := template.New("validation").Funcs(tte.GetTemplateHelpers()).Parse(templateContent)
	if err != nil {
		return fmt.Errorf("template validation failed: %w", err)
	}
	return nil
}

// GetTemplateHelpers returns template helper functions
func (tte *ToolsetTemplateEngine) GetTemplateHelpers() template.FuncMap {
	return template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		// Guideline #14: Replace deprecated strings.Title with simple title case
		"title": toTitle,
		"contains": func(s, substr string) bool {
			return strings.Contains(s, substr)
		},
		"replace": func(s, old, new string) string {
			return strings.ReplaceAll(s, old, new)
		},
		"joinStrings": func(elems []string, sep string) string {
			return strings.Join(elems, sep)
		},
	}
}

// toTitle provides simple title case functionality to replace deprecated strings.Title
// Guideline #14: Use simple, dependency-free implementations when possible
func toTitle(s string) string {
	if len(s) == 0 {
		return s
	}

	// Split into words and capitalize each word
	words := strings.Fields(s)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = string(unicode.ToUpper(rune(word[0]))) + strings.ToLower(word[1:])
		}
	}

	return strings.Join(words, " ")
}
