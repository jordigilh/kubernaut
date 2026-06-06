package tools

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
)

// ErrPromUnavailable is returned when the Prometheus client is nil.
var ErrPromUnavailable = errors.New("alert service unavailable — Prometheus not configured")

var validStates = map[string]bool{
	"firing":  true,
	"pending": true,
}

var validAlertSeverities = map[string]bool{
	"critical": true,
	"high":     true,
	"medium":   true,
	"low":      true,
	"info":     true,
	"warning":  true,
}

// sensitiveAlertKeys lists label keys stripped before returning alert data to
// the LLM (FedRAMP SI-10). Mirrors severity.sensitiveKeys but applied at
// the tool boundary.
var sensitiveAlertKeys = map[string]bool{
	"password":   true,
	"token":      true,
	"secret":     true,
	"key":        true,
	"credential": true,
	"bearer":     true,
}

var ipPortPattern = regexp.MustCompile(`\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}(:\d+)?`)

// ListAlertsArgs defines the input for list_alerts.
type ListAlertsArgs struct {
	Namespace string `json:"namespace,omitempty"`
	Severity  string `json:"severity,omitempty"`
	State     string `json:"state,omitempty"`
}

// AlertSummary is a redacted view of a Prometheus alert safe for LLM consumption.
type AlertSummary struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations,omitempty"`
	State       string            `json:"state"`
	ActiveAt    time.Time         `json:"active_at,omitempty"`
}

// ListAlertsResult is the output of list_alerts.
type ListAlertsResult struct {
	Alerts []AlertSummary `json:"alerts"`
	Count  int            `json:"count"`
}

// GetAlertDetailsArgs defines the input for get_alert_details.
type GetAlertDetailsArgs struct {
	AlertName string `json:"alert_name"`
	Namespace string `json:"namespace,omitempty"`
}

// GetAlertDetailsResult is the output of get_alert_details.
type GetAlertDetailsResult struct {
	Alerts []AlertSummary `json:"alerts"`
	Count  int            `json:"count"`
}

// HandleListAlerts implements the list_alerts logic.
// AC-6: GetAlerts returns all cluster alerts; filtering is client-side.
func HandleListAlerts(ctx context.Context, client prom.Client, args ListAlertsArgs) (ListAlertsResult, error) {
	if client == nil {
		return ListAlertsResult{}, ErrPromUnavailable
	}
	if args.Severity != "" && !validAlertSeverities[strings.ToLower(args.Severity)] {
		return ListAlertsResult{}, fmt.Errorf("%w: severity must be one of critical, high, medium, low, info, warning", ErrInvalidInput)
	}
	if args.State != "" && !validStates[strings.ToLower(args.State)] {
		return ListAlertsResult{}, fmt.Errorf("%w: state must be firing or pending", ErrInvalidInput)
	}

	alerts, err := client.GetAlerts(ctx)
	if err != nil {
		return ListAlertsResult{}, errors.New(security.RedactError(err))
	}

	result := make([]AlertSummary, 0, len(alerts))
	for i := range alerts {
		a := &alerts[i]
		if args.Namespace != "" && a.Labels["namespace"] != args.Namespace {
			continue
		}
		if args.Severity != "" && !strings.EqualFold(a.Labels["severity"], args.Severity) {
			continue
		}
		if args.State != "" && !strings.EqualFold(a.State, args.State) {
			continue
		}
		result = append(result, AlertSummary{
			Labels:      redactAlertLabels(a.Labels),
			Annotations: a.Annotations,
			State:       a.State,
			ActiveAt:    a.ActiveAt,
		})
	}

	return ListAlertsResult{Alerts: result, Count: len(result)}, nil
}

// HandleGetAlertDetails implements the get_alert_details logic.
func HandleGetAlertDetails(ctx context.Context, client prom.Client, args GetAlertDetailsArgs) (GetAlertDetailsResult, error) {
	if client == nil {
		return GetAlertDetailsResult{}, ErrPromUnavailable
	}
	if args.AlertName == "" {
		return GetAlertDetailsResult{}, fmt.Errorf("%w: alert_name is required", ErrInvalidInput)
	}

	alerts, err := client.GetAlerts(ctx)
	if err != nil {
		return GetAlertDetailsResult{}, errors.New(security.RedactError(err))
	}

	result := make([]AlertSummary, 0)
	for i := range alerts {
		a := &alerts[i]
		if a.Labels["alertname"] != args.AlertName {
			continue
		}
		if args.Namespace != "" && a.Labels["namespace"] != args.Namespace {
			continue
		}
		result = append(result, AlertSummary{
			Labels:      redactAlertLabels(a.Labels),
			Annotations: a.Annotations,
			State:       a.State,
			ActiveAt:    a.ActiveAt,
		})
	}

	return GetAlertDetailsResult{Alerts: result, Count: len(result)}, nil
}

// redactAlertLabels returns a copy of labels with sensitive keys removed and
// IP:port patterns in instance values redacted (FedRAMP SI-10).
func redactAlertLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		if sensitiveAlertKeys[strings.ToLower(k)] {
			continue
		}
		if k == "instance" {
			v = ipPortPattern.ReplaceAllString(v, "[REDACTED]")
		}
		out[k] = v
	}
	return out
}

// NewListAlertsTool creates the list_alerts ADK tool.
func NewListAlertsTool(client prom.Client) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "list_alerts",
		Description: "List currently firing or pending Prometheus/Thanos alerts, optionally filtered by namespace, severity, or state",
	}, func(ctx tool.Context, args ListAlertsArgs) (ListAlertsResult, error) {
		return HandleListAlerts(ctx, client, args)
	})
}

// NewGetAlertDetailsTool creates the get_alert_details ADK tool.
func NewGetAlertDetailsTool(client prom.Client) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "get_alert_details",
		Description: "Get details of a specific Prometheus/Thanos alert by name, optionally filtered by namespace",
	}, func(ctx tool.Context, args GetAlertDetailsArgs) (GetAlertDetailsResult, error) {
		return HandleGetAlertDetails(ctx, client, args)
	})
}
