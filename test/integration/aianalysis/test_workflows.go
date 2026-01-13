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

package aianalysis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// TestWorkflow represents a workflow for AIAnalysis integration tests
// These workflows match the Mock LLM responses to enable end-to-end testing
type TestWorkflow struct {
	WorkflowID   string // Must match Mock LLM workflow_id (e.g., "oomkill-increase-memory-v1")
	Name         string
	Description  string
	SignalType   string // Must match test scenarios (e.g., "OOMKilled")
	Severity     string
	Component    string
	Environment  string
	Priority     string
}

// GetAIAnalysisTestWorkflows returns the workflows that Mock LLM expects
// These must be registered in DataStorage before tests run
//
// Pattern: Test data alignment between Mock LLM and DataStorage
// - Mock LLM returns workflow IDs (e.g., "oomkill-increase-memory-v1")
// - HAPI validates workflows via DataStorage API
// - Tests fail if workflows don't exist in catalog
func GetAIAnalysisTestWorkflows() []TestWorkflow {
	return []TestWorkflow{
		{
			WorkflowID:   "oomkill-increase-memory-v1",
			Name:         "OOMKill Recovery - Increase Memory Limits",
			Description:  "Increase memory limits for pods hitting OOMKill",
			SignalType:   "OOMKilled",
			Severity:     "critical",
			Component:    "deployment",
			Environment:  "production",
			Priority:     "P0",
		},
		{
			WorkflowID:   "crashloop-config-fix-v1",
			Name:         "CrashLoopBackOff - Configuration Fix",
			Description:  "Fix missing configuration causing CrashLoopBackOff",
			SignalType:   "CrashLoopBackOff",
			Severity:     "high",
			Component:    "deployment",
			Environment:  "production",
			Priority:     "P1",
		},
		{
			WorkflowID:   "node-drain-reboot-v1",
			Name:         "NodeNotReady - Drain and Reboot",
			Description:  "Drain node and reboot to resolve NodeNotReady",
			SignalType:   "NodeNotReady",
			Severity:     "critical",
			Component:    "node",
			Environment:  "production",
			Priority:     "P0",
		},
		{
			WorkflowID:   "memory-optimize-v1",
			Name:         "Memory Optimization - Alternative Approach",
			Description:  "Optimize memory usage after failed scaling attempt",
			SignalType:   "OOMKilled",
			Severity:     "critical",
			Component:    "deployment",
			Environment:  "production",
			Priority:     "P0",
		},
		{
			WorkflowID:   "generic-restart-v1",
			Name:         "Generic Pod Restart",
			Description:  "Generic pod restart for unknown issues",
			SignalType:   "Unknown",
			Severity:     "medium",
			Component:    "deployment",
			Environment:  "staging",
			Priority:     "P2",
		},
	}
}

// SeedTestWorkflowsInDataStorage registers test workflows in DataStorage
// Called during SynchronizedBeforeSuite Phase 1 to prepare test data
//
// Pattern: DD-TEST-010 Multi-Controller Pattern - Shared Infrastructure Setup
// - Process 1 seeds workflows in DataStorage (shared resource)
// - All processes can reference these workflows during tests
// - Prevents "workflow not found" errors during HAPI validation
func SeedTestWorkflowsInDataStorage(dataStorageURL string, output io.Writer) error {
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	_, _ = fmt.Fprintf(output, "🌱 Seeding Test Workflows in DataStorage\n")
	_, _ = fmt.Fprintf(output, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	workflows := GetAIAnalysisTestWorkflows()
	_, _ = fmt.Fprintf(output, "📋 Registering %d test workflows...\n", len(workflows))

	for _, wf := range workflows {
		if err := registerWorkflowInDataStorage(dataStorageURL, wf, output); err != nil {
			return fmt.Errorf("failed to register workflow %s: %w", wf.WorkflowID, err)
		}
		_, _ = fmt.Fprintf(output, "  ✅ %s\n", wf.WorkflowID)
	}

	_, _ = fmt.Fprintf(output, "✅ All test workflows registered\n\n")
	return nil
}

// registerWorkflowInDataStorage registers a single workflow via DataStorage REST API
// Pattern: BR-STORAGE-014 Workflow Catalog Management
func registerWorkflowInDataStorage(dataStorageURL string, wf TestWorkflow, output io.Writer) error {
	version := "1.0.0"
	content := fmt.Sprintf("# Test workflow %s\nversion: %s\ndescription: %s", wf.WorkflowID, version, wf.Description)
	contentBytes := []byte(content)
	hash := sha256.Sum256(contentBytes)
	contentHash := fmt.Sprintf("%x", hash)

	// Build payload matching DataStorage OpenAPI schema
	// See: test/infrastructure/workflow_bundles.go:261-288 for pattern
	workflowReq := map[string]interface{}{
		"workflow_name":    wf.WorkflowID, // Primary key (workflow_name + version)
		"version":          version,
		"name":             wf.Name,
		"description":      wf.Description,
		"content":          content,
		"content_hash":     contentHash,
		"execution_engine": "tekton",
		"container_image":  fmt.Sprintf("quay.io/jordigilh/test-workflows/%s:%s", wf.WorkflowID, version),
		"labels": map[string]interface{}{
			"signal_type": wf.SignalType,   // Mandatory
			"severity":    wf.Severity,     // Mandatory
			"component":   wf.Component,    // Mandatory
			"environment": wf.Environment,  // Mandatory
			"priority":    wf.Priority,     // Mandatory
		},
		"status": "active",
	}

	jsonPayload, err := json.Marshal(workflowReq)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow payload: %w", err)
	}

	// POST to DataStorage workflow creation endpoint
	// BR-STORAGE-014: Workflow catalog management
	endpoint := fmt.Sprintf("%s/api/v1/workflows", dataStorageURL)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to POST workflow to DataStorage: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DataStorage returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
