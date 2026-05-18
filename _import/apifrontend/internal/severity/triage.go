package severity

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"

	prom "github.com/jordigilh/kubernaut-apifrontend/internal/prometheus"
)

// LLMTriager defines the interface for LLM-based severity classification.
type LLMTriager interface {
	TriageWithRules(ctx context.Context, rules []prom.Rule, input TriageInput) (TriageResult, error)
	TriagePure(ctx context.Context, input TriageInput) (TriageResult, error)
}

// Config holds configuration for the triage pipeline.
type Config struct {
	Enabled           bool
	MaxQueriesPerCall int
	MaxRulesEvaluated int
	CacheTTLSeconds   int
	LLMConfidence     float64
}

// DefaultConfig returns the default triage config.
func DefaultConfig() Config {
	return Config{
		Enabled:           true,
		MaxQueriesPerCall: 10,
		MaxRulesEvaluated: 100,
		CacheTTLSeconds:   30,
		LLMConfidence:     0.7,
	}
}

// TriagerMetrics holds optional Prometheus collectors for triage observability.
type TriagerMetrics struct {
	Total    *prometheus.CounterVec
	Duration *prometheus.HistogramVec
	Errors   *prometheus.CounterVec
}

// Triager orchestrates the multi-tier severity triage pipeline.
type Triager struct {
	promClient prom.Client
	llm        LLMTriager
	config     Config
	logger     logr.Logger
	cache      *RulesCache
	metrics    *TriagerMetrics
}

// NewTriager creates a new Triager instance.
// Panics if llm is nil — the pipeline requires an LLM fallback to guarantee a result.
// An optional TriagerMetrics may be passed to enable Prometheus instrumentation.
func NewTriager(promClient prom.Client, llm LLMTriager, cfg Config, logger logr.Logger, m ...*TriagerMetrics) *Triager {
	if llm == nil {
		panic("NewTriager: LLMTriager must not be nil — the triage pipeline requires an LLM fallback")
	}
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	t := &Triager{
		promClient: promClient,
		llm:        llm,
		config:     cfg,
		logger:     logger,
		cache:      NewRulesCache(cfg.CacheTTLSeconds),
	}
	if len(m) > 0 && m[0] != nil {
		t.metrics = m[0]
	}
	return t
}

// Triage runs the severity triage pipeline: Tier 1 -> 1.5 -> 2 -> 2.5/3.
// Returns a zero TriageResult if triage is disabled.
func (t *Triager) Triage(ctx context.Context, input TriageInput) (TriageResult, error) {
	if !t.config.Enabled {
		return TriageResult{}, nil
	}

	start := time.Now()
	result, err := t.triagePipeline(ctx, input)
	t.recordMetrics(result, err, time.Since(start))
	return result, err
}

func (t *Triager) recordMetrics(result TriageResult, err error, elapsed time.Duration) {
	if t.metrics == nil {
		return
	}
	tier := string(result.Source)
	if err != nil {
		if tier == "" {
			tier = string(SourceLLMTriage)
		}
		if t.metrics.Errors != nil {
			t.metrics.Errors.WithLabelValues(tier, "llm_failure").Inc()
		}
		return
	}
	if tier == "" {
		return
	}
	if t.metrics.Total != nil {
		t.metrics.Total.WithLabelValues(tier, result.Severity).Inc()
	}
	if t.metrics.Duration != nil {
		t.metrics.Duration.WithLabelValues(tier).Observe(elapsed.Seconds())
	}
}

func (t *Triager) triagePipeline(ctx context.Context, input TriageInput) (TriageResult, error) {
	if err := ctx.Err(); err != nil {
		return TriageResult{}, err
	}

	// Tier 1: Check firing alerts
	result, done := t.runTier1(ctx, input)
	if done {
		return result, nil
	}

	if err := ctx.Err(); err != nil {
		return TriageResult{}, err
	}

	// Fetch rules (cached or fresh) — shared by Tier 1.5 and Tier 2
	ruleGroups, rulesErr := t.fetchRules(ctx)

	// Tier 1.5: Check pending alerts from rules
	if rulesErr == nil {
		result, done = t.runTier15(input, ruleGroups)
		if done {
			return result, nil
		}
	} else {
		t.logger.Info("skipping Tier 1.5: rules fetch failed", "error", rulesErr.Error())
	}

	// Tier 2: Evaluate inactive matching rules
	var matchedRules []prom.Rule
	if rulesErr == nil {
		result, matchedRules, done = t.runTier2(ctx, input, ruleGroups)
		if done {
			return result, nil
		}
	} else {
		t.logger.Info("skipping Tier 2: rules fetch failed", "error", rulesErr.Error())
	}

	// Tier 2.5: LLM with rule context (only if rules matched but data was empty)
	if len(matchedRules) > 0 {
		result, done = t.runTier25(ctx, input, matchedRules)
		if done {
			return result, nil
		}
	}

	// Tier 3: Pure LLM fallback
	return t.runTier3(ctx, input)
}

func (t *Triager) runTier1(ctx context.Context, input TriageInput) (TriageResult, bool) {
	if len(input.Labels) == 0 {
		return TriageResult{}, false
	}

	alerts, err := t.promClient.GetAlerts(ctx)
	if err != nil {
		t.logger.Info("Tier 1 failed, continuing", "error", err.Error())
		return TriageResult{}, false
	}

	var bestSeverity string
	var bestAlert string
	for _, alert := range alerts {
		if alert.State != "firing" {
			continue
		}
		if !labelsOverlap(alert.Labels, input.Labels) {
			continue
		}
		sev := alert.Labels["severity"]
		if bestSeverity == "" || CompareSeverity(sev, bestSeverity) > 0 {
			bestSeverity = sev
			bestAlert = alert.Labels["alertname"]
		}
	}

	if bestSeverity != "" {
		return TriageResult{
			Severity:  bestSeverity,
			Source:    SourceFiringAlert,
			AlertName: bestAlert,
		}, true
	}
	return TriageResult{}, false
}

func (t *Triager) runTier15(input TriageInput, ruleGroups []prom.RuleGroup) (TriageResult, bool) {
	var bestSeverity string
	var bestRule string
	for _, g := range ruleGroups {
		for _, r := range g.Rules {
			if r.State != "pending" {
				continue
			}
			matchers, err := prom.ExtractLabelMatchers(r.Query)
			if err != nil {
				continue
			}
			if !prom.MatchesResource(matchers, input.Labels) {
				continue
			}
			sev := r.Labels["severity"]
			if bestSeverity == "" || CompareSeverity(sev, bestSeverity) > 0 {
				bestSeverity = sev
				bestRule = r.Name
			}
		}
	}
	if bestSeverity != "" {
		return TriageResult{
			Severity: bestSeverity,
			Source:   SourcePendingAlert,
			RuleName: bestRule,
		}, true
	}
	return TriageResult{}, false
}

func (t *Triager) runTier2(ctx context.Context, input TriageInput, ruleGroups []prom.RuleGroup) (TriageResult, []prom.Rule, bool) {
	var matchedRules []prom.Rule
	queryCount := 0

	for _, g := range ruleGroups {
		for _, r := range g.Rules {
			if len(matchedRules) >= t.config.MaxRulesEvaluated {
				break
			}
			if r.State != "inactive" {
				continue
			}
			matchers, err := prom.ExtractLabelMatchers(r.Query)
			if err != nil {
				continue
			}
			if !prom.MatchesResource(matchers, input.Labels) {
				continue
			}
			matchedRules = append(matchedRules, r)

			if queryCount >= t.config.MaxQueriesPerCall {
				continue
			}
			queryCount++

			qr, qErr := t.promClient.InstantQuery(ctx, r.Query)
			if qErr != nil {
				t.logger.Info("Tier 2 query failed", "rule", r.Name, "error", qErr.Error())
				continue
			}
			if len(qr.Samples) > 0 {
				return TriageResult{
					Severity: r.Labels["severity"],
					Source:   SourceRuleEval,
					RuleName: r.Name,
				}, matchedRules, true
			}
		}
	}
	return TriageResult{}, matchedRules, false
}

func (t *Triager) runTier25(ctx context.Context, input TriageInput, matchedRules []prom.Rule) (TriageResult, bool) {
	result, err := t.llm.TriageWithRules(ctx, matchedRules, input)
	if err != nil {
		t.logger.Info("Tier 2.5 LLM failed", "error", err.Error())
		return TriageResult{}, false
	}
	result.Source = SourceLLMRuleInform
	if result.Confidence > 0 && result.Confidence < t.config.LLMConfidence {
		t.logger.Info("LLM confidence below threshold, defaulting to medium",
			"tier", "2.5", "confidence", result.Confidence, "threshold", t.config.LLMConfidence)
		result.Severity = "medium"
	}
	return result, true
}

func (t *Triager) runTier3(ctx context.Context, input TriageInput) (TriageResult, error) {
	result, err := t.llm.TriagePure(ctx, input)
	if err != nil {
		return TriageResult{}, fmt.Errorf("tier 3 LLM triage failed: %w", err)
	}
	result.Source = SourceLLMTriage
	if result.Confidence > 0 && result.Confidence < t.config.LLMConfidence {
		t.logger.Info("LLM confidence below threshold, defaulting to medium",
			"tier", "3", "confidence", result.Confidence, "threshold", t.config.LLMConfidence)
		result.Severity = "medium"
	}
	return result, nil
}

func (t *Triager) fetchRules(ctx context.Context) ([]prom.RuleGroup, error) {
	if cached := t.cache.Get(); cached != nil {
		return cached, nil
	}
	groups, err := t.promClient.GetRules(ctx)
	if err != nil {
		return nil, err
	}
	t.cache.Set(groups)
	return groups, nil
}

// labelsOverlap returns true if every key present in both maps has an equal
// value and at least one such key exists. A single mismatched key rejects the
// pair, preventing cross-namespace false positives (e.g. kind=Deployment alone).
func labelsOverlap(alertLabels, targetLabels map[string]string) bool {
	matched := 0
	for k, v := range targetLabels {
		if alertVal, exists := alertLabels[k]; exists {
			if alertVal != v {
				return false
			}
			matched++
		}
	}
	return matched > 0
}
