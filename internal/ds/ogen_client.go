package ds

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/ogenx"

	"github.com/jordigilh/kubernaut-apifrontend/internal/audit"
)

// OgenClient implements the Client interface using the ogen-generated DS client.
// It wraps the generated client with resilience transport (CB + retry + metrics)
// injected via the http.Client passed at construction time.
type OgenClient struct {
	client *ogenclient.Client
}

// OgenClientConfig holds configuration for the ogen DS client adapter.
type OgenClientConfig struct {
	BaseURL   string
	Transport http.RoundTripper
	Timeout   time.Duration
}

// NewOgenClient creates a new DS client backed by the ogen-generated API client.
// The resilience transport (circuit breaker + retry) should be injected via cfg.Transport.
func NewOgenClient(cfg OgenClientConfig) (*OgenClient, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("ds: baseURL cannot be empty")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}

	transport := cfg.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   cfg.Timeout,
	}

	client, err := ogenclient.NewClient(cfg.BaseURL, ogenclient.WithClient(httpClient))
	if err != nil {
		return nil, fmt.Errorf("ds: create ogen client: %w", err)
	}

	return &OgenClient{client: client}, nil
}

// ListWorkflows queries the DS for workflows matching the given options.
func (c *OgenClient) ListWorkflows(ctx context.Context, opts ListWorkflowsOpts) ([]Workflow, error) {
	params := ogenclient.ListWorkflowsParams{}
	if opts.Kind != "" {
		params.Component = ogenclient.NewOptString(opts.Kind)
	}

	resp, err := c.client.ListWorkflows(ctx, params)
	err = ogenx.ToError(resp, err)
	if err != nil {
		return nil, fmt.Errorf("ds: list workflows: %w", err)
	}

	result, ok := resp.(*ogenclient.WorkflowListResponse)
	if !ok {
		return nil, fmt.Errorf("ds: unexpected response type %T", resp)
	}

	workflows := make([]Workflow, 0, len(result.Workflows))
	for i := range result.Workflows {
		w := &result.Workflows[i]
		wf := Workflow{
			Name: w.WorkflowName,
			Kind: w.ActionType,
		}
		if w.WorkflowId.Set {
			wf.ID = w.WorkflowId.Value.String()
		}
		workflows = append(workflows, wf)
	}
	return workflows, nil
}

// GetRemediationHistory queries the DS for remediation history.
func (c *OgenClient) GetRemediationHistory(ctx context.Context, opts HistoryOpts) ([]HistoricalRemediation, error) {
	params := ogenclient.GetRemediationHistoryContextParams{
		TargetKind:      opts.Kind,
		TargetName:      opts.Name,
		TargetNamespace: opts.Namespace,
	}
	if opts.Since != "" {
		params.Tier1Window = ogenclient.NewOptString(opts.Since)
	}

	resp, err := c.client.GetRemediationHistoryContext(ctx, params)
	err = ogenx.ToError(resp, err)
	if err != nil {
		return nil, fmt.Errorf("ds: get remediation history: %w", err)
	}

	result, ok := resp.(*ogenclient.RemediationHistoryContext)
	if !ok {
		return nil, fmt.Errorf("ds: unexpected response type %T", resp)
	}

	remediations := make([]HistoricalRemediation, 0, len(result.Tier1.Chain))
	for i := range result.Tier1.Chain {
		r := &result.Tier1.Chain[i]
		remediations = append(remediations, HistoricalRemediation{
			ID:        r.RemediationUID,
			Namespace: opts.Namespace,
			Phase:     r.Outcome.Value,
			CreatedAt: r.CompletedAt.Format(time.RFC3339),
			Workflow:  r.ActionType.Value,
		})
	}
	return remediations, nil
}

// GetEffectiveness queries the DS for workflow effectiveness scores.
func (c *OgenClient) GetEffectiveness(ctx context.Context, opts EffectivenessOpts) (*EffectivenessReport, error) {
	params := ogenclient.GetEffectivenessScoreParams{
		CorrelationID: opts.WorkflowID,
	}

	resp, err := c.client.GetEffectivenessScore(ctx, params)
	err = ogenx.ToError(resp, err)
	if err != nil {
		return nil, fmt.Errorf("ds: get effectiveness: %w", err)
	}

	result, ok := resp.(*ogenclient.EffectivenessScoreResponse)
	if !ok {
		return nil, fmt.Errorf("ds: unexpected response type %T", resp)
	}

	report := &EffectivenessReport{
		WorkflowID: opts.WorkflowID,
	}
	if result.Score.Set && !result.Score.Null {
		report.SuccessRate = result.Score.Value
	}
	return report, nil
}

// GetAuditTrail queries the DS for audit events matching the given options.
func (c *OgenClient) GetAuditTrail(ctx context.Context, opts AuditTrailOpts) ([]AuditEvent, error) {
	params := ogenclient.QueryAuditEventsParams{}
	if opts.RRID != "" {
		params.CorrelationID = ogenclient.NewOptString(opts.RRID)
	}
	if opts.EventType != "" {
		params.EventType = ogenclient.NewOptString(opts.EventType)
	}

	resp, err := c.client.QueryAuditEvents(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("ds: query audit events: %w", err)
	}

	events := make([]AuditEvent, 0, len(resp.Data))
	for i := range resp.Data {
		e := &resp.Data[i]
		events = append(events, AuditEvent{
			Timestamp: e.EventTimestamp.Format(time.RFC3339),
			EventType: e.EventType,
			Actor:     e.ActorID.Value,
			Detail:    e.EventAction,
		})
	}
	return events, nil
}

// WriteAuditEvents sends a batch of audit events to the DS audit endpoint.
func (c *OgenClient) WriteAuditEvents(ctx context.Context, events []*audit.Event) error {
	if len(events) == 0 {
		return nil
	}

	batch := make([]ogenclient.AuditEventRequest, 0, len(events))
	for _, evt := range events {
		req := ogenclient.AuditEventRequest{
			Version:        "1.0",
			EventType:      string(evt.Type),
			EventTimestamp: evt.Timestamp,
			EventCategory:  ogenclient.AuditEventRequestEventCategoryGateway,
			EventAction:    detailValue(evt.Detail, "action", string(evt.Type)),
			EventOutcome:   eventOutcome(evt.Type),
		}
		if evt.UserID != "" {
			req.ActorID = ogenclient.NewOptString(evt.UserID)
			req.ActorType = ogenclient.NewOptString("user")
		}
		if evt.RequestID != "" {
			req.CorrelationID = evt.RequestID
		}
		batch = append(batch, req)
	}

	_, err := c.client.CreateAuditEventsBatch(ctx, batch)
	if err != nil {
		return fmt.Errorf("ds: write audit events batch: %w", err)
	}
	return nil
}

func eventOutcome(t audit.EventType) ogenclient.AuditEventRequestEventOutcome {
	switch t {
	case audit.EventAuthFailure,
		audit.EventA2ATaskFailed,
		audit.EventRateLimitDenied,
		audit.EventCircuitBreakerTrip,
		audit.EventConfigRejected,
		audit.EventSessionAutoCancelled,
		audit.EventRBACDenied:
		return ogenclient.AuditEventRequestEventOutcomeFailure
	default:
		return ogenclient.AuditEventRequestEventOutcomeSuccess
	}
}

func detailValue(detail map[string]string, key, fallback string) string {
	if v, ok := detail[key]; ok && v != "" {
		return v
	}
	return fallback
}

// Compile-time interface check.
var _ Client = (*OgenClient)(nil)
