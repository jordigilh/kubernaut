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
					Status:      "firing",
					Labels:      alert.Labels,
					Annotations: alert.Annotations,
					Timestamp:   activeTime,
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("AlertManager health check failed with status %d", resp.StatusCode)
	}

	return nil
}
