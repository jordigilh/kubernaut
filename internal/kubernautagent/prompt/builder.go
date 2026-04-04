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

package prompt

import (
	"bytes"
	"embed"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// SignalData contains the signal-level fields rendered into the prompt.
type SignalData struct {
	Name             string
	Namespace        string
	Severity         string
	Message          string
	ResourceKind     string
	ResourceName     string
	ClusterName      string
	Environment      string
	Priority         string
	RiskTolerance    string
	SignalSource     string
	BusinessCategory string
	Description      string
}

// EnrichmentData contains enrichment context injected into the prompt.
type EnrichmentData struct {
	OwnerChain      []string
	DetectedLabels  map[string]string
	HistoryResult   *enrichment.RemediationHistoryResult
	HistoryRendered string
}


// investigationTemplateData maps to fields expected by incident_investigation.tmpl.
type investigationTemplateData struct {
	IncidentSummary             string
	PriorityDescription         string
	Environment                 string
	RiskDescription             string
	SignalName                  string
	Severity                    string
	Namespace                   string
	ResourceKind                string
	ResourceName                string
	ErrorMessage                string
	SignalSource                string
	ClusterName                 string
	Description                 string
	FiringTime                  string
	ReceivedTime                string
	SignalMode                  string
	OwnerChain                  string
	DetectedLabels              string
	RemediationHistory          string
	IsDuplicate                 bool
	OccurrenceCount             int
	DeduplicationWindowMinutes  int
	FirstSeen                   string
	LastSeen                    string
	PDBSignalGuidance           string
	RemediationHistorySection   string
	Priority                    string
	BusinessCategory            string
	RiskTolerance               string
}

// workflowTemplateData maps to fields expected by phase3_workflow_selection.tmpl.
type workflowTemplateData struct {
	Severity            string
	SignalName          string
	Namespace           string
	ResourceKind        string
	ResourceName        string
	ClusterName         string
	SignalMode          string
	PriorityDescription string
	Environment         string
	RiskDescription     string
	RCASummary          string
	EnrichmentContext   string
}

// Builder renders prompt templates with signal and enrichment data.
type Builder struct {
	investigationTmpl *template.Template
	workflowTmpl      *template.Template
}

// NewBuilder creates a prompt builder with embedded templates.
func NewBuilder() (*Builder, error) {
	invTmpl, err := template.ParseFS(templateFS, "templates/incident_investigation.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing investigation template: %w", err)
	}
	wfTmpl, err := template.ParseFS(templateFS, "templates/phase3_workflow_selection.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing workflow selection template: %w", err)
	}
	return &Builder{
		investigationTmpl: invTmpl,
		workflowTmpl:      wfTmpl,
	}, nil
}

// RenderInvestigation renders the Phase 1 investigation prompt.
func (b *Builder) RenderInvestigation(signal SignalData, enrichData *EnrichmentData) (string, error) {
	sanitized := sanitizeSignal(signal)

	data := investigationTemplateData{
		IncidentSummary:     fmt.Sprintf("%s %s in %s: %s", sanitized.Severity, sanitized.Name, sanitized.Namespace, sanitized.Message),
		PriorityDescription: withDefault(sanitized.Priority, inferPriority(sanitized.Severity)),
		Environment:         withDefault(sanitized.Environment, sanitized.Namespace),
		RiskDescription:     withDefault(sanitized.RiskTolerance, inferRisk(sanitized.Severity)),
		SignalName:          sanitized.Name,
		Severity:            sanitized.Severity,
		Namespace:           sanitized.Namespace,
		ResourceKind:        withDefault(sanitized.ResourceKind, "Pod"),
		ResourceName:        withDefault(sanitized.ResourceName, sanitized.Name),
		ErrorMessage:        sanitized.Message,
		SignalSource:        withDefault(sanitized.SignalSource, "kubernaut-gateway"),
		ClusterName:         withDefault(sanitized.ClusterName, "default"),
		Description:         withDefault(sanitized.Description, sanitized.Message),
		FiringTime:          "N/A",
		ReceivedTime:        "N/A",
		SignalMode:          "reactive",
		Priority:            sanitized.Priority,
		BusinessCategory:    sanitized.BusinessCategory,
		RiskTolerance:       sanitized.RiskTolerance,
	}

	if enrichData != nil {
		if len(enrichData.OwnerChain) > 0 {
			data.OwnerChain = strings.Join(enrichData.OwnerChain, " → ")
		}
		if len(enrichData.DetectedLabels) > 0 {
			var labels []string
			for k, v := range enrichData.DetectedLabels {
				labels = append(labels, fmt.Sprintf("%s=%s", k, v))
			}
			data.DetectedLabels = strings.Join(labels, ", ")
		}

		var sections []string
		if data.OwnerChain != "" {
			sections = append(sections, "**Owner Chain**: "+data.OwnerChain)
		}
		if data.DetectedLabels != "" {
			sections = append(sections, "**Detected Labels**: "+data.DetectedLabels)
		}

		if enrichData.HistoryResult != nil && (len(enrichData.HistoryResult.Tier1) > 0 || len(enrichData.HistoryResult.Tier2) > 0) {
			sections = append(sections, BuildRemediationHistorySection(
				enrichData.HistoryResult, RepeatedRemediationEscalationThreshold))
		} else if enrichData.HistoryRendered != "" {
			sections = append(sections, enrichData.HistoryRendered)
		}

		if len(sections) > 0 {
			data.RemediationHistorySection = strings.Join(sections, "\n\n")
		}
	}

	var buf bytes.Buffer
	if err := b.investigationTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering investigation template: %w", err)
	}
	return buf.String(), nil
}

// RenderWorkflowSelection renders the Phase 3 workflow selection prompt.
func (b *Builder) RenderWorkflowSelection(signal SignalData, rcaSummary string, enrichData *EnrichmentData) (string, error) {
	sanitized := sanitizeSignal(signal)
	data := workflowTemplateData{
		Severity:            withDefault(sanitized.Severity, "critical"),
		SignalName:          withDefault(sanitized.Name, "investigation"),
		Namespace:           withDefault(sanitized.Namespace, "default"),
		ResourceKind:        withDefault(sanitized.ResourceKind, "Pod"),
		ResourceName:        withDefault(sanitized.ResourceName, "unknown"),
		ClusterName:         withDefault(sanitized.ClusterName, "default"),
		SignalMode:          "reactive",
		PriorityDescription: withDefault(sanitized.Priority, inferPriority(sanitized.Severity)),
		Environment:         withDefault(sanitized.Environment, "default"),
		RiskDescription:     withDefault(sanitized.RiskTolerance, inferRisk(sanitized.Severity)),
		RCASummary:          rcaSummary,
	}

	if enrichData != nil {
		var parts []string
		if len(enrichData.OwnerChain) > 0 {
			parts = append(parts, "Owner chain: "+strings.Join(enrichData.OwnerChain, " → "))
		}
		if enrichData.HistoryResult != nil {
			t1Count := len(enrichData.HistoryResult.Tier1)
			t2Count := len(enrichData.HistoryResult.Tier2)
			if t1Count+t2Count > 0 {
				parts = append(parts, fmt.Sprintf("Remediation history: %d recent + %d historical entries", t1Count, t2Count))
			}
			if enrichData.HistoryResult.RegressionDetected {
				parts = append(parts, "WARNING: Configuration regression detected")
			}
		}
		data.EnrichmentContext = strings.Join(parts, "\n")
	}

	var buf bytes.Buffer
	if err := b.workflowTmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("rendering workflow selection template: %w", err)
	}
	return buf.String(), nil
}

var injectionPatterns = regexp.MustCompile(
	`(?i)(ignore\s+(all\s+)?previous\s+instructions|you\s+are\s+now|disregard\s+(all\s+)?prior|forget\s+(all\s+)?previous|system\s*:\s*you\s+are)`,
)

func withDefault(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func sanitizeSignal(signal SignalData) SignalData {
	return SignalData{
		Name:             sanitizeField(signal.Name),
		Namespace:        sanitizeField(signal.Namespace),
		Severity:         sanitizeField(signal.Severity),
		Message:          sanitizeField(signal.Message),
		ResourceKind:     sanitizeField(signal.ResourceKind),
		ResourceName:     sanitizeField(signal.ResourceName),
		ClusterName:      sanitizeField(signal.ClusterName),
		Environment:      sanitizeField(signal.Environment),
		Priority:         sanitizeField(signal.Priority),
		RiskTolerance:    sanitizeField(signal.RiskTolerance),
		SignalSource:     sanitizeField(signal.SignalSource),
		BusinessCategory: sanitizeField(signal.BusinessCategory),
		Description:      sanitizeField(signal.Description),
	}
}

func sanitizeField(s string) string {
	return injectionPatterns.ReplaceAllString(s, "[REDACTED]")
}

func inferPriority(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "P1 — Critical: Immediate response required"
	case "warning":
		return "P2 — Warning: Timely investigation needed"
	default:
		return "P3 — Informational: Standard investigation"
	}
}

func inferRisk(severity string) string {
	switch strings.ToLower(severity) {
	case "critical":
		return "Low risk tolerance — prefer proven remediation workflows"
	case "warning":
		return "Medium risk tolerance — standard remediation acceptable"
	default:
		return "Higher risk tolerance — experimental remediation may be appropriate"
	}
}
