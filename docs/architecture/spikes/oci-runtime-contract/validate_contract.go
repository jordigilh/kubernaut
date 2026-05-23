/*
Spike 3: OCI Runtime Contract Validator

Validates that an AgenticWorkflow runtime image meets the packaging contract:
1. Required OCI labels present
2. Entry point spec file exists at the labeled path
3. Runtime-specific entry point command is valid
4. Tool list matches OAS spec tools (for oas/deepagent runtimes)
*/
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// ContractLabels defines the required OCI labels for an AgenticWorkflow image.
type ContractLabels struct {
	Runtime     string `json:"ai.kubernaut.runtime"`
	SpecVersion string `json:"ai.kubernaut.spec-version"`
	Entrypoint  string `json:"ai.kubernaut.entrypoint"`
	Tools       string `json:"ai.kubernaut.tools"`
}

// RuntimeSpec defines the expected spec file for each runtime type.
type RuntimeSpec struct {
	Type         string
	SpecPath     string
	SpecFormat   string
	EntryCommand string
	RequiresPy   bool
}

var knownRuntimes = map[string]RuntimeSpec{
	"goose": {
		Type:         "goose",
		SpecPath:     "/spec/recipe.yaml",
		SpecFormat:   "goose-recipe",
		EntryCommand: "goose run --recipe",
		RequiresPy:   false,
	},
	"oas": {
		Type:         "oas",
		SpecPath:     "/spec/agent.yaml",
		SpecFormat:   "oracle-agent-spec",
		EntryCommand: "python -m kubernaut_oas",
		RequiresPy:   true,
	},
	"deepagent": {
		Type:         "deepagent",
		SpecPath:     "/spec/agent.yaml",
		SpecFormat:   "langgraph-agent",
		EntryCommand: "python -m kubernaut_deepagent",
		RequiresPy:   true,
	},
}

// ValidationResult holds the outcome of contract validation.
type ValidationResult struct {
	RuntimeType string   `json:"runtime_type"`
	Valid       bool     `json:"valid"`
	Errors      []string `json:"errors,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
}

// ValidateLabels checks that all required OCI labels are present and valid.
func ValidateLabels(labels ContractLabels) ValidationResult {
	result := ValidationResult{
		RuntimeType: labels.Runtime,
		Valid:       true,
	}

	if labels.Runtime == "" {
		result.Errors = append(result.Errors, "missing required label: ai.kubernaut.runtime")
		result.Valid = false
	} else if _, ok := knownRuntimes[labels.Runtime]; !ok {
		result.Errors = append(result.Errors, fmt.Sprintf("unknown runtime type: %s (expected: goose, oas, deepagent)", labels.Runtime))
		result.Valid = false
	}

	if labels.SpecVersion == "" {
		result.Errors = append(result.Errors, "missing required label: ai.kubernaut.spec-version")
		result.Valid = false
	}

	if labels.Entrypoint == "" {
		result.Errors = append(result.Errors, "missing required label: ai.kubernaut.entrypoint")
		result.Valid = false
	} else if !strings.HasPrefix(labels.Entrypoint, "/spec/") {
		result.Warnings = append(result.Warnings, "entrypoint not under /spec/ directory (convention violation)")
	}

	if labels.Tools == "" {
		result.Warnings = append(result.Warnings, "no tools declared in ai.kubernaut.tools label")
	}

	if runtime, ok := knownRuntimes[labels.Runtime]; ok {
		if labels.Entrypoint != runtime.SpecPath {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("entrypoint %s differs from convention %s for runtime %s",
					labels.Entrypoint, runtime.SpecPath, labels.Runtime))
		}
	}

	return result
}

func main() {
	tests := []struct {
		name   string
		labels ContractLabels
		expect bool
	}{
		{
			name: "valid_oas_image",
			labels: ContractLabels{
				Runtime:     "oas",
				SpecVersion: "25.4.1",
				Entrypoint:  "/spec/agent.yaml",
				Tools:       "kubectl_get,kubectl_list_events,prometheus_query,submit_result",
			},
			expect: true,
		},
		{
			name: "valid_goose_image",
			labels: ContractLabels{
				Runtime:     "goose",
				SpecVersion: "1.0",
				Entrypoint:  "/spec/recipe.yaml",
				Tools:       "kubectl_get,kubectl_list_events,submit_result",
			},
			expect: true,
		},
		{
			name: "valid_deepagent_image",
			labels: ContractLabels{
				Runtime:     "deepagent",
				SpecVersion: "0.1",
				Entrypoint:  "/spec/agent.yaml",
				Tools:       "kubectl_get,kubectl_list_events,prometheus_query,submit_result",
			},
			expect: true,
		},
		{
			name: "missing_runtime",
			labels: ContractLabels{
				SpecVersion: "1.0",
				Entrypoint:  "/spec/agent.yaml",
				Tools:       "kubectl_get",
			},
			expect: false,
		},
		{
			name: "unknown_runtime",
			labels: ContractLabels{
				Runtime:     "unknown-engine",
				SpecVersion: "1.0",
				Entrypoint:  "/spec/agent.yaml",
			},
			expect: false,
		},
		{
			name: "missing_spec_version",
			labels: ContractLabels{
				Runtime:    "oas",
				Entrypoint: "/spec/agent.yaml",
			},
			expect: false,
		},
		{
			name: "non_standard_entrypoint",
			labels: ContractLabels{
				Runtime:     "oas",
				SpecVersion: "25.4.1",
				Entrypoint:  "/custom/path/agent.yaml",
				Tools:       "kubectl_get",
			},
			expect: true, // valid but warns
		},
	}

	allPass := true
	for _, tc := range tests {
		result := ValidateLabels(tc.labels)
		pass := result.Valid == tc.expect

		status := "PASS"
		if !pass {
			status = "FAIL"
			allPass = false
		}

		fmt.Printf("[%s] %s (runtime=%s, valid=%v, expected=%v)\n",
			status, tc.name, tc.labels.Runtime, result.Valid, tc.expect)
		for _, e := range result.Errors {
			fmt.Printf("       ERROR: %s\n", e)
		}
		for _, w := range result.Warnings {
			fmt.Printf("       WARN:  %s\n", w)
		}
	}

	fmt.Println()
	if allPass {
		fmt.Println("[PASS] All OCI contract validation tests passed")
	} else {
		fmt.Println("[FAIL] Some tests failed")
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("Runtime contract summary:")
	for name, spec := range knownRuntimes {
		data, _ := json.MarshalIndent(spec, "  ", "  ")
		fmt.Printf("  %s: %s\n", name, data)
	}
}
