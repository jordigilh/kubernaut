package severity

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
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

// Triager orchestrates the multi-tier severity triage pipeline.
type Triager struct {
	promClient  prom.Client
	llm         LLMTriager
	config      Config
	logger      logr.Logger
	cache       *RulesCache
	auditor     audit.Emitter
	podResolver PodResolver
}

// TriagerOption configures optional dependencies on Triager.
type TriagerOption func(*Triager)

// WithAuditor injects an audit.Emitter for SOC2 AU-2 compliance.
func WithAuditor(e audit.Emitter) TriagerOption {
	return func(t *Triager) { t.auditor = e }
}

// WithPodResolver injects a PodResolver for workload-to-pod correlation in Tier 1.
// When set, Triage() auto-resolves pod names before running the pipeline.
func WithPodResolver(r PodResolver) TriagerOption {
	return func(t *Triager) { t.podResolver = r }
}

// NewTriager creates a new Triager instance.
// Panics if llm is nil — the pipeline requires an LLM fallback to guarantee a result.
func NewTriager(promClient prom.Client, llm LLMTriager, cfg Config, logger logr.Logger, opts ...TriagerOption) *Triager {
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
	for _, o := range opts {
		o(t)
	}
	return t
}

// Triage runs the severity triage pipeline: Tier 1 -> 1.5 -> 2 -> 2.5/3.
// Returns a zero TriageResult if triage is disabled.
func (t *Triager) Triage(ctx context.Context, input TriageInput) (TriageResult, error) {
	if !t.config.Enabled {
		return TriageResult{}, nil
	}

	if len(input.PodNames) == 0 && t.podResolver != nil {
		pods, err := t.podResolver.ResolvePodNames(ctx, input.Namespace, input.Kind, input.Name)
		if err != nil {
			t.logger.Info("pod resolution failed, continuing without pod correlation", "error", err.Error())
		} else {
			input.PodNames = pods
		}
	}

	result, err := t.triagePipeline(ctx, input)
	if err != nil {
		if t.auditor != nil {
			t.auditor.Emit(ctx, &audit.Event{
				Type: audit.EventSeverityTriageFailed,
				Detail: map[string]string{
					"namespace": input.Namespace,
					"kind":      input.Kind,
					"name":      input.Name,
					"error":     err.Error(),
				},
			})
		}
		return result, err
	}
	if result.Severity != "" && t.auditor != nil {
		t.auditor.Emit(ctx, &audit.Event{
			Type: audit.EventSeverityTriageCompleted,
			Detail: map[string]string{
				"namespace": input.Namespace,
				"kind":      input.Kind,
				"name":      input.Name,
				"severity":  result.Severity,
				"source":    string(result.Source),
			},
		})
	}
	return result, nil
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

	podNameSet := make(map[string]struct{}, len(input.PodNames))
	for _, pn := range input.PodNames {
		podNameSet[pn] = struct{}{}
	}

	// Pass 1: firing alerts (highest priority).
	if result, ok := t.bestAlertMatch(alerts, input.Labels, podNameSet, input.Namespace, "firing", SourceFiringAlert); ok {
		return result, true
	}

	// Pass 2: pending alerts with the same pod correlation (closes the
	// timing race where an alert condition is met but the `for` duration
	// has not elapsed yet).
	if result, ok := t.bestAlertMatch(alerts, input.Labels, podNameSet, input.Namespace, "pending", SourcePendingAlert); ok {
		return result, true
	}

	return TriageResult{}, false
}

// matchCandidate tracks the best alert match at a given priority tier.
type matchCandidate struct {
	found    bool
	severity string
	alert    string
}

func (m *matchCandidate) update(sev, alertName string) {
	if !m.found || CompareSeverity(sev, m.severity) > 0 {
		m.found = true
		m.severity = sev
		m.alert = alertName
	}
}

func (m *matchCandidate) result(source Source) TriageResult {
	return TriageResult{
		Severity:  m.severity,
		Source:    source,
		AlertName: m.alert,
	}
}

// nsSource maps a resource-level source to its namespace-scoped equivalent.
func nsSource(base Source) Source {
	if base == SourcePendingAlert {
		return SourceNSPendingAlert
	}
	return SourceNSFiringAlert
}

// clusterSource maps a resource-level source to its cluster-scoped equivalent.
func clusterSource(base Source) Source {
	if base == SourcePendingAlert {
		return SourceClusterPendingAlert
	}
	return SourceClusterFiringAlert
}

func (t *Triager) bestAlertMatch(alerts []prom.Alert, targetLabels map[string]string, podNameSet map[string]struct{}, targetNamespace, state string, source Source) (TriageResult, bool) {
	var resourceBest, nsBest, clusterBest matchCandidate

	for _, alert := range alerts {
		if alert.State != state {
			continue
		}
		sev := alert.Labels["severity"]

		if labelsOverlap(alert.Labels, targetLabels, podNameSet, targetNamespace) {
			resourceBest.update(sev, alert.Labels["alertname"])
		} else if targetNamespace != "" && alert.Labels["namespace"] == targetNamespace {
			nsBest.update(sev, alert.Labels["alertname"])
		} else if alert.Labels["namespace"] == "" {
			clusterBest.update(sev, alert.Labels["alertname"])
		}
	}

	if resourceBest.found {
		return resourceBest.result(source), true
	}
	if nsBest.found {
		return nsBest.result(nsSource(source)), true
	}
	if clusterBest.found {
		return clusterBest.result(clusterSource(source)), true
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

// labelsOverlap returns true if the alert correlates with the target resource.
//
// Two correlation paths are checked in order:
//
// 1. Key overlap: every key present in both maps (excluding "namespace") must
// have equal values, and at least one such key must exist. This is the
// original path for alerts that carry kind/name labels.
//
// 2. Pod-based correlation (fallback): if key overlap finds no match AND
// podNames is non-empty, checks whether alert.Labels["pod"] matches any
// resolved pod name. Requires alert.Labels["namespace"] == targetNamespace
// to prevent cross-namespace false matches (M3).
//
// The "namespace" key is excluded from key-overlap comparison because the
// signal source (Prometheus alert) fires in the workload namespace (e.g.,
// "default"), while the RR is created in AF's operational namespace (e.g.,
// "kubernaut-system").
func labelsOverlap(alertLabels, targetLabels map[string]string, podNameSet map[string]struct{}, targetNamespace string) bool {
	matched := 0
	for k, v := range targetLabels {
		if k == "namespace" {
			continue
		}
		if alertVal, exists := alertLabels[k]; exists {
			if alertVal != v {
				return false
			}
			matched++
		}
	}
	if matched > 0 {
		return true
	}

	if len(podNameSet) > 0 {
		alertPod := alertLabels["pod"]
		alertNS := alertLabels["namespace"]
		if alertPod != "" && alertNS == targetNamespace {
			if _, ok := podNameSet[alertPod]; ok {
				return true
			}
		}
	}

	return false
}
