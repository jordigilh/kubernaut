package holmesgpt

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/sirupsen/logrus"
)

// ToolsetTemplateEngine provides templating capabilities for toolset generation
// Business Requirement: BR-HOLMES-023 - Toolset configuration templates
type ToolsetTemplateEngine struct {
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
	tmpl, exists := tte.templates[templateName]
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
func (tte *ToolsetTemplateEngine) RenderToolCommand(commandTemplate string, variables map[string]string) (string, error) {
	tmpl, err := template.New("command").Parse(commandTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse command template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, variables); err != nil {
		return "", fmt.Errorf("failed to execute command template: %w", err)
	}

	return buf.String(), nil
}

// AddTemplate adds a custom template to the engine
func (tte *ToolsetTemplateEngine) AddTemplate(name, templateContent string) error {
	tmpl, err := template.New(name).Parse(templateContent)
	if err != nil {
		return fmt.Errorf("failed to parse template %s: %w", name, err)
	}

	tte.templates[name] = tmpl
	tte.log.WithField("template_name", name).Debug("Added custom template")
	return nil
}

// GetAvailableTemplates returns the names of all available templates
func (tte *ToolsetTemplateEngine) GetAvailableTemplates() []string {
	names := make([]string, 0, len(tte.templates))
	for name := range tte.templates {
		names = append(names, name)
	}
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

	templateName := serviceType
	if _, exists := tte.templates[templateName]; !exists {
		templateName = "custom" // Fallback to custom template
	}

	// For now, return a basic config based on the variables
	// In a full implementation, this would render the JSON template and unmarshal it
	config := &ToolsetConfig{
		Name:         fmt.Sprintf("%s-%s-%s", variables.ServiceType, variables.Namespace, variables.ServiceName),
		ServiceType:  variables.ServiceType,
		Description:  fmt.Sprintf("%s tools for %s", strings.Title(variables.ServiceType), variables.ServiceName),
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
func (tte *ToolsetTemplateEngine) ValidateTemplate(templateContent string) error {
	_, err := template.New("validation").Parse(templateContent)
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
		"title": strings.Title,
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
