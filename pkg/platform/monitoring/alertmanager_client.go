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

package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// AlertManagerClient provides real AlertManager API integration
type AlertManagerClient struct {
	endpoint   string
	httpClient *http.Client
	log        *logrus.Logger
}

// NewAlertManagerClient creates a new AlertManager API client
func NewAlertManagerClient(endpoint string, timeout time.Duration, log *logrus.Logger) *AlertManagerClient {
	return &AlertManagerClient{
		endpoint: strings.TrimRight(endpoint, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
		log: log,
	}
}

// AlertManagerAlert represents an alert from AlertManager API
type AlertManagerAlert struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	State       string            `json:"state"` // "active", "suppressed", "unprocessed"
	ActiveAt    *time.Time        `json:"activeAt,omitempty"`
	Value       string            `json:"value"`
}

// AlertManagerResponse represents the response from AlertManager alerts API
type AlertManagerResponse struct {
	Status string              `json:"status"`
	Data   []AlertManagerAlert `json:"data"`
}

// IsAlertResolved checks if an alert has been resolved since the given time
func (c *AlertManagerClient) IsAlertResolved(ctx context.Context, alertName, namespace string, since time.Time) (bool, error) {
	c.log.WithFields(logrus.Fields{
		"alert_name": alertName,
		"namespace":  namespace,
		"since":      since,
	}).Debug("Checking if alert is resolved via AlertManager API")

	// Query AlertManager for active alerts matching our criteria
	activeAlerts, err := c.getActiveAlerts(ctx, alertName, namespace)
	if err != nil {
		return false, fmt.Errorf("failed to query active alerts: %w", err)
	}

	// If no active alerts found, consider it resolved
	if len(activeAlerts) == 0 {
		c.log.WithFields(logrus.Fields{
			"alert_name": alertName,
			"namespace":  namespace,
		}).Debug("No active alerts found, considering resolved")
		return true, nil
	}

	// Check if any active alerts started after our reference time
	// If all active alerts started before 'since', then the original alert was resolved
	for _, alert := range activeAlerts {
		if alert.ActiveAt != nil && alert.ActiveAt.After(since) {
			c.log.WithFields(logrus.Fields{
				"alert_name": alertName,
				"namespace":  namespace,
				"active_at":  alert.ActiveAt,
				"since":      since,
			}).Debug("Found alert active after reference time, not resolved")
			return false, nil
		}
	}

	// All active alerts (if any) started before our reference time
	// This suggests the original alert was resolved and possibly re-fired
	return true, nil
}

// HasAlertRecurred checks if an alert has fired again within the time window
func (c *AlertManagerClient) HasAlertRecurred(ctx context.Context, alertName, namespace string, from, to time.Time) (bool, error) {
	c.log.WithFields(logrus.Fields{
		"alert_name": alertName,
		"namespace":  namespace,
		"from":       from,
		"to":         to,
	}).Debug("Checking if alert has recurred via AlertManager API")

	// Get active alerts to check for recurrence
	activeAlerts, err := c.getActiveAlerts(ctx, alertName, namespace)
	if err != nil {
		return false, fmt.Errorf("failed to query alerts for recurrence check: %w", err)
	}

	// Check if any alerts became active within our time window
	for _, alert := range activeAlerts {
		if alert.ActiveAt != nil {
			activeTime := *alert.ActiveAt
			if activeTime.After(from) && activeTime.Before(to) {
				c.log.WithFields(logrus.Fields{
					"alert_name": alertName,
					"namespace":  namespace,
					"active_at":  activeTime,
					"window":     fmt.Sprintf("%s to %s", from, to),
				}).Info("Alert recurrence detected")
				return true, nil
			}
		}
	}

	c.log.WithFields(logrus.Fields{
		"alert_name": alertName,
		"namespace":  namespace,
	}).Debug("No alert recurrence detected in time window")
	return false, nil
}

// GetAlertHistory returns the firing history of an alert
func (c *AlertManagerClient) GetAlertHistory(ctx context.Context, alertName, namespace string, from, to time.Time) ([]AlertEvent, error) {
	c.log.WithFields(logrus.Fields{
		"alert_name": alertName,
		"namespace":  namespace,
		"from":       from,
		"to":         to,
	}).Debug("Getting alert history via AlertManager API")

	// Get current active alerts as a starting point
	activeAlerts, err := c.getActiveAlerts(ctx, alertName, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to query alerts for history: %w", err)
	}

	var events []AlertEvent

	// Convert active alerts to events
	for _, alert := range activeAlerts {
		if alert.ActiveAt != nil {
			activeTime := *alert.ActiveAt
			if activeTime.After(from) && activeTime.Before(to) {
				event := AlertEvent{
					AlertName:   alertName,
					Namespace:   namespace,
					Severity:    alert.Labels["severity"],
					State:       alert.State,
					Status:      "firing",
					Labels:      alert.Labels,
					Annotations: alert.Annotations,
					Timestamp:   activeTime,
					Value:       alert.Value,
				}
				events = append(events, event)
			}
		}
	}

	c.log.WithFields(logrus.Fields{
		"alert_name":  alertName,
		"namespace":   namespace,
		"event_count": len(events),
	}).Debug("Retrieved alert history")

	return events, nil
}

// getActiveAlerts queries AlertManager for active alerts matching the criteria
func (c *AlertManagerClient) getActiveAlerts(ctx context.Context, alertName, namespace string) ([]AlertManagerAlert, error) {
	// Build query URL with filters
	queryURL := fmt.Sprintf("%s/api/v1/alerts", c.endpoint)

	// Add query parameters for filtering
	params := url.Values{}
	if alertName != "" {
		params.Add("filter", fmt.Sprintf("alertname=\"%s\"", alertName))
	}
	if namespace != "" {
		params.Add("filter", fmt.Sprintf("namespace=\"%s\"", namespace))
	}

	if len(params) > 0 {
		queryURL += "?" + params.Encode()
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.log.WithError(err).Error("Failed to close HTTP response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AlertManager API returned status %d", resp.StatusCode)
	}

	// Parse response
	var response AlertManagerResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Status != "success" {
		return nil, fmt.Errorf("AlertManager API returned status: %s", response.Status)
	}

	// Filter out suppressed and unprocessed alerts, keep only active ones
	var activeAlerts []AlertManagerAlert
	for _, alert := range response.Data {
		if alert.State == "active" {
			activeAlerts = append(activeAlerts, alert)
		}
	}

	c.log.WithFields(logrus.Fields{
		"total_alerts":  len(response.Data),
		"active_alerts": len(activeAlerts),
		"alert_name":    alertName,
		"namespace":     namespace,
	}).Debug("Retrieved alerts from AlertManager")

	return activeAlerts, nil
}

// HealthCheck verifies connectivity to AlertManager
func (c *AlertManagerClient) HealthCheck(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.endpoint+"/-/healthy", nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute health check: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.log.WithError(err).Error("Failed to close HTTP response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AlertManager health check failed with status %d", resp.StatusCode)
	}

	return nil
}

// CreateSilence creates a silence in AlertManager (BR-MET-011: Must provide monitoring operations)
func (c *AlertManagerClient) CreateSilence(ctx context.Context, silence *SilenceRequest) (*SilenceResponse, error) {
	c.log.WithFields(logrus.Fields{
		"matchers":   silence.Matchers,
		"starts_at":  silence.StartsAt,
		"ends_at":    silence.EndsAt,
		"created_by": silence.CreatedBy,
		"comment":    silence.Comment,
	}).Info("Creating silence via AlertManager API")

	// Prepare AlertManager silence payload
	silencePayload := map[string]interface{}{
		"matchers":  silence.Matchers,
		"startsAt":  silence.StartsAt.Format(time.RFC3339),
		"endsAt":    silence.EndsAt.Format(time.RFC3339),
		"createdBy": silence.CreatedBy,
		"comment":   silence.Comment,
	}

	payload, err := json.Marshal(silencePayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal silence payload: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint+"/api/v1/silences", strings.NewReader(string(payload)))
	if err != nil {
		return nil, fmt.Errorf("failed to create silence request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute silence request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.log.WithError(err).Error("Failed to close HTTP response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AlertManager silence API returned status %d", resp.StatusCode)
	}

	// Parse response
	var response struct {
		Status string `json:"status"`
		Data   struct {
			SilenceID string `json:"silenceID"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode silence response: %w", err)
	}

	if response.Status != "success" {
		return nil, fmt.Errorf("AlertManager silence API returned status: %s", response.Status)
	}

	c.log.WithFields(logrus.Fields{
		"silence_id": response.Data.SilenceID,
		"matchers":   silence.Matchers,
		"duration":   silence.EndsAt.Sub(silence.StartsAt),
	}).Info("Successfully created silence")

	return &SilenceResponse{
		SilenceID: response.Data.SilenceID,
	}, nil
}

// DeleteSilence removes an existing silence from AlertManager
func (c *AlertManagerClient) DeleteSilence(ctx context.Context, silenceID string) error {
	c.log.WithField("silence_id", silenceID).Info("Deleting silence via AlertManager API")

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "DELETE", fmt.Sprintf("%s/api/v1/silence/%s", c.endpoint, silenceID), nil)
	if err != nil {
		return fmt.Errorf("failed to create silence deletion request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute silence deletion request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.log.WithError(err).Error("Failed to close HTTP response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AlertManager silence deletion returned status %d", resp.StatusCode)
	}

	c.log.WithField("silence_id", silenceID).Info("Successfully deleted silence")
	return nil
}

// GetSilences returns active silences matching criteria
func (c *AlertManagerClient) GetSilences(ctx context.Context, matchers []SilenceMatcher) ([]Silence, error) {
	c.log.WithField("matcher_count", len(matchers)).Debug("Getting silences via AlertManager API")

	// Build query URL
	queryURL := c.endpoint + "/api/v1/silences"

	// Add query parameters for filtering (if supported by AlertManager)
	params := url.Values{}
	for _, matcher := range matchers {
		params.Add("filter", fmt.Sprintf("%s=\"%s\"", matcher.Name, matcher.Value))
	}

	if len(params) > 0 {
		queryURL += "?" + params.Encode()
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", queryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create get silences request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute get silences request: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			c.log.WithError(err).Error("Failed to close HTTP response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AlertManager get silences API returned status %d", resp.StatusCode)
	}

	// Parse response
	var response struct {
		Status string    `json:"status"`
		Data   []Silence `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode silences response: %w", err)
	}

	if response.Status != "success" {
		return nil, fmt.Errorf("AlertManager get silences API returned status: %s", response.Status)
	}

	c.log.WithFields(logrus.Fields{
		"silence_count": len(response.Data),
		"matcher_count": len(matchers),
	}).Debug("Retrieved silences from AlertManager")

	return response.Data, nil
}

// AcknowledgeAlert acknowledges an alert (BR-ALERT-012: Alert acknowledgment support)
func (c *AlertManagerClient) AcknowledgeAlert(ctx context.Context, alertID string, acknowledgedBy string) error {
	c.log.WithFields(logrus.Fields{
		"alert_id":        alertID,
		"acknowledged_by": acknowledgedBy,
	}).Info("Acknowledging alert via AlertManager API")

	// Note: AlertManager doesn't have a native acknowledgment API
	// This is typically handled by creating a silence or through external systems
	// For now, we'll create a short-term silence as an acknowledgment

	// Create a 1-hour silence as acknowledgment
	silenceRequest := &SilenceRequest{
		Matchers: []SilenceMatcher{
			{
				Name:    "alertname",
				Value:   alertID,
				IsRegex: false,
			},
		},
		StartsAt:  time.Now(),
		EndsAt:    time.Now().Add(time.Hour),
		CreatedBy: acknowledgedBy,
		Comment:   fmt.Sprintf("Acknowledged by %s via Kubernaut", acknowledgedBy),
	}

	_, err := c.CreateSilence(ctx, silenceRequest)
	if err != nil {
		return fmt.Errorf("failed to acknowledge alert via silence: %w", err)
	}

	c.log.WithFields(logrus.Fields{
		"alert_id":        alertID,
		"acknowledged_by": acknowledgedBy,
	}).Info("Successfully acknowledged alert")

	return nil
}
