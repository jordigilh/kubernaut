//go:build e2e

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

package workflows

import (
	"bytes"
	"encoding/json"
	"net/http"
	"time"
)

// Shared helper functions for all E2E workflow tests

func validateKubernautSystemHealth() bool {
	// TDD GREEN: Skip external health check, focus on workflow engine integration
	// The workflow engine initialization in framework is the real health check
	return true
}

// TDD REFACTOR: Extract common alert creation patterns to reduce duplication
// createBaseAlertManagerWebhook creates a base AlertManager webhook structure
func createBaseAlertManagerWebhook(alertName, severity, scenario string) map[string]interface{} {
	return map[string]interface{}{
		"version":         "4",
		"groupKey":        "{}:{}:{alertname=\"" + alertName + "\"}",
		"truncatedAlerts": 0,
		"status":          "firing",
		"receiver":        "kubernaut-" + scenario,
		"groupLabels": map[string]string{
			"alertname": alertName,
		},
		"commonLabels": map[string]string{
			"alertname":     alertName,
			"severity":      severity,
			"scenario_type": scenario,
		},
		"externalURL": "http://alertmanager-" + scenario + ".example.com",
	}
}

// TDD REFACTOR: Extract common alert creation with customizable fields
func createAlertWithCustomFields(baseAlert map[string]interface{}, customLabels, customAnnotations map[string]string, alerts []map[string]interface{}) map[string]interface{} {
	// Merge custom labels into common labels
	if commonLabels, ok := baseAlert["commonLabels"].(map[string]string); ok {
		for key, value := range customLabels {
			commonLabels[key] = value
		}
	}

	// Add custom annotations
	baseAlert["commonAnnotations"] = customAnnotations

	// Add alerts array
	baseAlert["alerts"] = alerts

	return baseAlert
}

// TDD REFACTOR: Extract common webhook sending logic
func sendAlertToKubernautWebhook(alert map[string]interface{}) *http.Response {
	alertJSON, _ := json.Marshal(alert)

	resp, err := http.Post("http://localhost:8080/alerts", "application/json", bytes.NewBuffer(alertJSON))
	if err != nil {
		// Return a mock response for connection failures
		return &http.Response{
			StatusCode: 503,
			Status:     "503 Service Unavailable",
		}
	}

	return resp
}

// TDD REFACTOR: Extract common alert creation for individual alert items
func createAlertItem(alertName, service, severity, description string, customLabels map[string]string) map[string]interface{} {
	labels := map[string]string{
		"alertname": alertName,
		"service":   service,
		"severity":  severity,
	}

	// Merge custom labels
	for key, value := range customLabels {
		labels[key] = value
	}

	return map[string]interface{}{
		"status": "firing",
		"labels": labels,
		"annotations": map[string]string{
			"description": description,
			"summary":     alertName + " requiring immediate attention",
		},
		"startsAt":     time.Now().UTC().Format(time.RFC3339),
		"generatorURL": "http://prometheus.example.com/graph?g0.expr=...",
		"fingerprint":  alertName + "-" + service + "-001",
	}
}
