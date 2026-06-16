package tools

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/severity"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
)

// Infrastructure-level patterns for redacting URLs and IPs from alert
// label/annotation values (FedRAMP SI-10). These complement
// security.RedactText which handles JWTs, bearer tokens, and base64.
var (
	alertURLPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"']+`)
	alertIPPattern  = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}(?::\d{1,5})?\b`)
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

// SensitiveAlertKeys lists label keys stripped before returning alert data to
// the LLM (FedRAMP SI-10). Must stay in sync with severity.SensitiveKeys;
// consistency enforced by UT-AF-1367-F4 (#1367 F4).
var SensitiveAlertKeys = map[string]bool{
	"password":   true,
	"token":      true,
	"secret":     true,
	"key":        true,
	"credential": true,
	"bearer":     true,
}

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

// PrioritizedAlerts holds index-based references into the sorted alerts[] array.
// SelectedIndex is the highest-severity, longest-firing (FIFO) alert.
// TiedIndices contains indices of other alerts at the same severity as Selected.
// AlsoActiveStart is the index where lower-severity alerts begin.
type PrioritizedAlerts struct {
	SelectedIndex   int   `json:"selected_index"`
	TiedIndices     []int `json:"tied_indices,omitempty"`
	AlsoActiveStart int   `json:"also_active_start,omitempty"`
}

// ListAlertsResult is the output of list_alerts.
type ListAlertsResult struct {
	Alerts      []AlertSummary   `json:"alerts"`
	Count       int              `json:"count"`
	TotalCount  int              `json:"total_count,omitempty"`
	Truncated   bool             `json:"truncated,omitempty"`
	Prioritized *PrioritizedAlerts `json:"prioritized,omitempty"`
}

// PrioritizeAlerts sorts alerts by severity (descending) then ActiveAt (ascending, FIFO)
// and returns index-based metadata identifying the selected alert, tied peers, and
// where lower-severity alerts begin. The alerts slice is sorted in-place.
func PrioritizeAlerts(alerts []AlertSummary) PrioritizedAlerts {
	if len(alerts) == 0 {
		return PrioritizedAlerts{}
	}
	slices.SortStableFunc(alerts, func(a, b AlertSummary) int {
		sevCmp := severity.CompareSeverity(b.Labels["severity"], a.Labels["severity"])
		if sevCmp != 0 {
			return sevCmp
		}
		return cmp.Compare(a.ActiveAt.UnixNano(), b.ActiveAt.UnixNano())
	})

	selectedSev := alerts[0].Labels["severity"]
	var tiedIndices []int
	alsoActiveStart := len(alerts)
	for i := 1; i < len(alerts); i++ {
		if severity.CompareSeverity(alerts[i].Labels["severity"], selectedSev) == 0 {
			tiedIndices = append(tiedIndices, i)
		} else {
			alsoActiveStart = i
			break
		}
	}
	return PrioritizedAlerts{
		SelectedIndex:   0,
		TiedIndices:     tiedIndices,
		AlsoActiveStart: alsoActiveStart,
	}
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
	if args.Namespace != "" {
		if err := validate.Namespace(args.Namespace); err != nil {
			return ListAlertsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
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
			Annotations: redactAnnotations(a.Annotations),
			State:       a.State,
			ActiveAt:    a.ActiveAt,
		})
	}

	totalCount := len(result)

	var prioritized *PrioritizedAlerts
	if len(result) > 0 {
		p := PrioritizeAlerts(result)
		prioritized = &p
	}

	out := ListAlertsResult{
		Alerts:      result,
		Count:       len(result),
		TotalCount:  totalCount,
		Prioritized: prioritized,
	}

	out = trimResultToFit(out)
	return out, nil
}

// trimResultToFit serializes the full ListAlertsResult and removes alerts from
// the tail (lowest priority) until the JSON fits within maxToolOutputBytes.
// This accounts for the full struct size including the prioritized metadata,
// preventing the session-level trimmer from replacing the response with a stub.
func trimResultToFit(r ListAlertsResult) ListAlertsResult {
	raw, err := json.Marshal(r)
	if err != nil || len(raw) <= maxToolOutputBytes {
		return r
	}
	r.Truncated = true
	for len(r.Alerts) > 1 && len(raw) > maxToolOutputBytes {
		r.Alerts = r.Alerts[:len(r.Alerts)-1]
		r.Count = len(r.Alerts)
		if r.Prioritized != nil {
			r.Prioritized = recalcPrioritizedIndices(r.Alerts, r.Prioritized)
		}
		raw, err = json.Marshal(r)
		if err != nil {
			break
		}
	}
	return r
}

// recalcPrioritizedIndices adjusts tied/also_active indices after trimming.
func recalcPrioritizedIndices(alerts []AlertSummary, p *PrioritizedAlerts) *PrioritizedAlerts {
	if len(alerts) == 0 {
		return nil
	}
	selectedSev := alerts[0].Labels["severity"]
	var tied []int
	alsoActiveStart := len(alerts)
	for i := 1; i < len(alerts); i++ {
		if severity.CompareSeverity(alerts[i].Labels["severity"], selectedSev) == 0 {
			tied = append(tied, i)
		} else {
			alsoActiveStart = i
			break
		}
	}
	return &PrioritizedAlerts{
		SelectedIndex:   0,
		TiedIndices:     tied,
		AlsoActiveStart: alsoActiveStart,
	}
}

// HandleGetAlertDetails implements the get_alert_details logic.
func HandleGetAlertDetails(ctx context.Context, client prom.Client, args GetAlertDetailsArgs) (GetAlertDetailsResult, error) {
	if client == nil {
		return GetAlertDetailsResult{}, ErrPromUnavailable
	}
	if args.AlertName == "" {
		return GetAlertDetailsResult{}, fmt.Errorf("%w: alert_name is required", ErrInvalidInput)
	}
	if args.Namespace != "" {
		if err := validate.Namespace(args.Namespace); err != nil {
			return GetAlertDetailsResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
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
			Annotations: redactAnnotations(a.Annotations),
			State:       a.State,
			ActiveAt:    a.ActiveAt,
		})
	}

	return GetAlertDetailsResult{Alerts: result, Count: len(result)}, nil
}

// redactAlertValue applies URL, IP, and secret redaction to a single value.
// Combines infrastructure-level patterns (URLs, IPs) with security.RedactText
// (JWTs, bearer tokens, base64 secrets) for defense-in-depth (FedRAMP SI-10).
func redactAlertValue(v string) string {
	v = alertURLPattern.ReplaceAllString(v, "[URL_REDACTED]")
	v = alertIPPattern.ReplaceAllString(v, "[HOST_REDACTED]")
	return security.RedactText(v)
}

// redactAlertLabels returns a copy of labels with sensitive keys removed and
// all values passed through redactAlertValue (FedRAMP SI-10).
func redactAlertLabels(labels map[string]string) map[string]string {
	out := make(map[string]string, len(labels))
	for k, v := range labels {
		if SensitiveAlertKeys[strings.ToLower(k)] {
			continue
		}
		out[k] = redactAlertValue(v)
	}
	return out
}

// redactAnnotations returns a copy of annotations with all values passed
// through redactAlertValue. Annotations can contain runbook_url with
// internal hostnames and description templates with interpolated values
// (FedRAMP SI-10).
func redactAnnotations(annotations map[string]string) map[string]string {
	if len(annotations) == 0 {
		return annotations
	}
	out := make(map[string]string, len(annotations))
	for k, v := range annotations {
		out[k] = redactAlertValue(v)
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
