package severity

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/go-logr/logr"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
	sharedaudit "github.com/jordigilh/kubernaut/pkg/shared/audit"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const fleetClusterLabelKey = "cluster"

// ErrSeverityUndetermined is returned when correlated Prometheus evidence
// cannot supply one explicit, unambiguous severity for the investigated resource.
//
// DD-AF-010/#1839 removed the no-evidence LLM fallback. DD-AF-016 also
// prohibits LLM-derived severity and local defaults; a source label must be
// present and unambiguous before AF creates an RR.
var ErrSeverityUndetermined = errors.New("cannot determine severity: no correlated alert or rule supplies one explicit, unambiguous severity")

// LLMTriager is retained for source compatibility with existing AF wiring and
// tests. DD-AF-016 makes it deliberately unused by severity triage.
type LLMTriager interface {
	TriageWithRules(ctx context.Context, rules []prom.Rule, input TriageInput) (TriageResult, error)
}

// CrossGroupSiblingChecker reports the API groups in which a resource with
// the same namespace/kind/name exists, other than the target's own group
// (#2658). Implementations must apply the single-group fast path internally
// (a Kind resolved to one group can have no siblings) and treat enumeration
// failure as an error so the caller fails closed.
type CrossGroupSiblingChecker interface {
	CheckCrossGroupSiblings(ctx context.Context, namespace, kind, name, targetGroup string) ([]string, error)
}

// AmbiguousSeverityError is returned by Triage when the only correlating
// evidence found is a cluster-scoped alert with no verified relationship to
// the target resource (DD-AF-012, #2027/#2028). Unlike ErrSeverityUndetermined
// (no evidence at all), here a candidate exists but trusting it silently
// risks attributing an unrelated cluster-wide incident to this resource --
// the caller must surface Candidate to the user and re-call with a matching
// TriageInput.ConfirmedSignalName to proceed.
type AmbiguousSeverityError struct {
	Candidate TriageResult
}

func (e *AmbiguousSeverityError) Error() string {
	switch e.Candidate.AmbiguityReason {
	case AmbiguityReasonGroupUnscopable:
		return "severity triage ambiguous: the correlating evidence cannot be scoped to the target's API group; a resource with the same namespace/kind/name exists in another API group (" +
			formatAPIGroups(e.Candidate.CandidateAPIGroups) + ")"
	case AmbiguityReasonAlertBelongsToSibling:
		return "severity triage ambiguous: the correlated candidate belongs to the sibling resource in API group " +
			formatAPIGroups(e.Candidate.CandidateAPIGroups) + ", not to the target's own API group"
	default:
		return "severity triage ambiguous: only a cluster-scoped alert (" + e.Candidate.AlertName + ") correlates, with no verified relationship to this resource"
	}
}

// formatAPIGroups renders a candidate group list for human/audit text.
func formatAPIGroups(groups []string) string {
	return scope.DisplayAPIGroups(groups, "unavailable")
}

// encodeAPIGroups serializes a candidate group list for the audit Detail map,
// whose values are strings. The store adapter decodes it into the typed
// candidate_api_groups payload field (#2658 D1.9).
func encodeAPIGroups(groups []string) string {
	if len(groups) == 0 {
		return ""
	}
	encoded, err := json.Marshal(groups)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// Config holds configuration for the triage pipeline.
type Config struct {
	Enabled           bool
	MaxQueriesPerCall int
	MaxRulesEvaluated int
	CacheTTLSeconds   int
	// LLMConfidence is retained for configuration compatibility only. Severity
	// triage no longer calls an LLM or applies a confidence-based fallback.
	LLMConfidence float64
}

// DefaultConfig returns the default triage config.
func DefaultConfig() Config {
	return Config{
		Enabled:           true,
		MaxQueriesPerCall: 10,
		MaxRulesEvaluated: 100,
		CacheTTLSeconds:   30,
	}
}

// Triager orchestrates the multi-tier severity triage pipeline.
type Triager struct {
	promClient     prom.Client
	config         Config
	logger         logr.Logger
	cache          *RulesCache
	auditor        audit.Emitter
	podResolver    PodResolver
	siblingChecker CrossGroupSiblingChecker
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

// WithCrossGroupSiblingChecker injects the #2658 group-scope gate's sibling
// checker. When set (and the target carries an APIGroup), a positive
// correlation that cannot be scoped to the target's group fails closed as a
// group_unscopable ambiguity.
func WithCrossGroupSiblingChecker(c CrossGroupSiblingChecker) TriagerOption {
	return func(t *Triager) { t.siblingChecker = c }
}

// NewTriager creates a Prometheus-backed Triager. The llm argument is retained
// for source compatibility but intentionally ignored: severity is sourced
// only from explicit alert/rule labels (DD-AF-016).
func NewTriager(promClient prom.Client, _ LLMTriager, cfg Config, logger logr.Logger, opts ...TriagerOption) *Triager {
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}
	t := &Triager{
		promClient: promClient,
		config:     cfg,
		logger:     logger,
		cache:      NewRulesCache(cfg.CacheTTLSeconds),
	}
	for _, o := range opts {
		o(t)
	}
	return t
}

// Triage runs firing-alert, pending-rule, then correlated-rule severity checks.
// Returns a zero TriageResult if triage is disabled. Returns
// ErrSeverityUndetermined if no explicit, unambiguous source severity exists.
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
	if err == nil && strings.TrimSpace(result.Severity) == "" {
		err = ErrSeverityUndetermined
	}
	if err != nil {
		if t.auditor != nil {
			t.auditor.Emit(ctx, &audit.Event{
				Type: audit.EventSeverityTriageFailed,
				ErrorDetails: sharedaudit.NewErrorDetails(
					"apifrontend", "ERR_UPSTREAM_FAILURE", security.RedactError(err), true,
				),
				Detail: map[string]string{
					"namespace": input.Namespace,
					"kind":      input.Kind,
					"name":      input.Name,
					"error":     security.RedactError(err),
				},
			})
		}
		return result, err
	}

	result, err = t.adjudicate(ctx, input, result)
	if err != nil {
		return result, err
	}

	if result.Severity != "" && t.auditor != nil {
		t.auditor.Emit(ctx, &audit.Event{
			Type: audit.EventSeverityTriageCompleted,
			Detail: map[string]string{
				"namespace":   input.Namespace,
				"kind":        input.Kind,
				"name":        input.Name,
				"severity":    result.Severity,
				"source_tier": string(result.Source),
				"api_group":   input.APIGroup,
			},
		})
	}
	return result, nil
}

// adjudicate applies the two fail-closed gates to a positive triage result and
// returns either the accepted result or the typed ambiguity that must be
// surfaced to the user (#2027/#2028, #2658). Extracted from Triage to keep its
// cognitive complexity within the project's lint budget.
func (t *Triager) adjudicate(ctx context.Context, input TriageInput, result TriageResult) (TriageResult, error) {
	// DD-AF-012/#2027/#2028 + #2658: ambiguity is reason-scoped. A
	// cluster-scoped-only match is a guess with no verified relationship
	// (resolvable via ConfirmedSignalName); a group-unscopable match is a
	// positive correlation that cannot be attributed to the target's API
	// group because a cross-group sibling exists (resolvable only via
	// ConfirmedAPIGroup). Both fail closed; neither confirmation class can
	// bypass the other's gate.
	if result.Ambiguous && !resolvesAmbiguity(result, input) {
		t.emitAmbiguous(ctx, input, result, true)
		return result, &AmbiguousSeverityError{Candidate: result}
	}

	// #2658 group-scope gate: a positive (non-ambiguous) correlation for a
	// target whose group is known must be scoped to that group. If a same
	// namespace/kind/name resource exists in another API group, the alert's
	// severity cannot be attributed to this target without the user
	// confirming which group the alert refers to. A known core group
	// (APIGroup == "" with APIGroupKnown) is gated too: core is a real
	// sibling group for Kinds registered in several groups.
	if result.Ambiguous || strings.TrimSpace(result.Severity) == "" {
		return result, nil
	}
	if input.APIGroup == "" && !input.APIGroupKnown {
		return result, nil
	}
	gated, gateErr := t.applyGroupScopeGate(ctx, input, result)
	if gateErr != nil {
		t.emitAmbiguous(ctx, input, gated, false)
		return gated, gateErr
	}
	return gated, nil
}

// emitAmbiguous persists the ambiguity decision (BR-SEVERITY-001 AC-8,
// AU-3/CC7.2). withCandidate controls whether the candidate identity/severity
// is included: a gate-side ambiguity may not have a confirmable alert.
func (t *Triager) emitAmbiguous(ctx context.Context, input TriageInput, result TriageResult, withCandidate bool) {
	if t.auditor == nil {
		return
	}
	detail := map[string]string{
		"namespace":            input.Namespace,
		"kind":                 input.Kind,
		"name":                 input.Name,
		"api_group":            input.APIGroup,
		"ambiguity_reason":     result.AmbiguityReason,
		"candidate_api_groups": encodeAPIGroups(result.CandidateAPIGroups),
	}
	if withCandidate {
		detail["candidate_alert"] = result.AlertName
		detail["candidate_severity"] = result.Severity
	}
	t.auditor.Emit(ctx, &audit.Event{Type: audit.EventSeverityTriageAmbiguous, Detail: detail})
}

// resolvesAmbiguity reports whether the caller's confirmation resolves the
// result's specific ambiguity reason (#2658: reason-scoped adjudication). A
// signal-name confirmation only resolves cluster_scoped_only; a group
// confirmation only resolves group_unscopable — neither can bypass the
// other's gate.
func resolvesAmbiguity(result TriageResult, input TriageInput) bool {
	switch result.AmbiguityReason {
	case AmbiguityReasonClusterScopedOnly:
		return result.AlertName != "" && result.AlertName == input.ConfirmedSignalName
	case AmbiguityReasonGroupUnscopable:
		group, state := scope.NormalizeAPIGroup(input.ConfirmedAPIGroup)
		return state == scope.NormalizeStateResolvedGroup && group == input.APIGroup
	default:
		return false
	}
}

// applyGroupScopeGate enforces the #2658 sibling-existence rule on a positive
// correlation. Fleet-mode targets adjudicate as today (no per-cluster reader
// exists; the deviation is logged and audited upstream via the ambiguous
// emission contract) — the maintainer-approved rule is to fail closed only
// when a sibling is actually found, and in fleet mode the check cannot run.
func (t *Triager) applyGroupScopeGate(ctx context.Context, input TriageInput, result TriageResult) (TriageResult, error) {
	// The user's own-group confirmation is the resolution for the whole
	// ambiguity class, so it is honoured before the sibling probe: when the
	// check itself is unavailable, a confirmation is the only way out and
	// must not dead-end (the sibling-vs-target distinction below still
	// applies whenever the probe does run).
	confirmedGroup, confirmedState := scope.NormalizeAPIGroup(input.ConfirmedAPIGroup)
	if confirmedState == scope.NormalizeStateResolvedGroup && confirmedGroup == input.APIGroup {
		return result, nil
	}

	if input.FleetMode && input.ClusterID != "" {
		t.logger.Info("group-scope gate skipped: no per-cluster sibling reader in fleet mode (documented deviation, #2658)",
			"namespace", input.Namespace, "kind", input.Kind, "name", input.Name, "cluster", input.ClusterID)
		return result, nil
	}

	ambiguous := func(reason string, siblings []string) (TriageResult, error) {
		result.Ambiguous = true
		result.AmbiguityReason = reason
		result.CandidateAPIGroups = siblings
		result.CandidateIdentifier = result.AlertName
		if result.RuleName != "" {
			result.CandidateIdentifier = result.RuleName
		}
		// An alert name must never be usable to bypass the group gate
		// (#2658 R3-3): clear it so ConfirmedSignalName cannot match.
		result.AlertName = ""
		return result, &AmbiguousSeverityError{Candidate: result}
	}

	if t.siblingChecker == nil {
		t.logger.Info("group-scope gate fails closed: no sibling checker wired",
			"namespace", input.Namespace, "kind", input.Kind, "name", input.Name)
		return ambiguous(AmbiguityReasonGroupUnscopable, nil)
	}
	siblings, err := t.siblingChecker.CheckCrossGroupSiblings(ctx, input.Namespace, input.Kind, input.Name, input.APIGroup)
	if err != nil {
		t.logger.Info("group-scope gate fails closed: sibling check unavailable",
			"namespace", input.Namespace, "kind", input.Kind, "name", input.Name, "error", err.Error())
		return ambiguous(AmbiguityReasonGroupUnscopable, nil)
	}
	if len(siblings) == 0 {
		return result, nil // unique instance: adjudicate as today
	}

	if confirmedState == scope.NormalizeStateResolvedGroup {
		for _, sibling := range siblings {
			if confirmedGroup == sibling {
				// The user attests the alert refers to the sibling's
				// resource: its severity must not be inherited.
				return ambiguous(AmbiguityReasonAlertBelongsToSibling, siblings)
			}
		}
	}
	return ambiguous(AmbiguityReasonGroupUnscopable, siblings)
}

func (t *Triager) triagePipeline(ctx context.Context, input TriageInput) (TriageResult, error) {
	if err := ctx.Err(); err != nil {
		return TriageResult{}, err
	}

	// Tier 1: Check firing alerts
	result, done := t.runTier1(ctx, input)
	if done {
		if strings.TrimSpace(result.Severity) == "" {
			return result, ErrSeverityUndetermined
		}
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
			if strings.TrimSpace(result.Severity) == "" {
				return result, ErrSeverityUndetermined
			}
			return result, nil
		}
	} else {
		t.logger.Info("skipping Tier 1.5: rules fetch failed", "error", rulesErr.Error())
	}

	// Tier 2: Evaluate matching rules, then use only their explicit labels.
	if rulesErr == nil {
		result, done = t.runTier2(ctx, input, ruleGroups)
		if done {
			if strings.TrimSpace(result.Severity) == "" {
				return result, ErrSeverityUndetermined
			}
			return result, nil
		}
	} else {
		t.logger.Info("skipping Tier 2: rules fetch failed", "error", rulesErr.Error())
	}

	// No matching source evidence can supply severity; do not infer a value.
	return TriageResult{}, ErrSeverityUndetermined
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

	return t.bestOverallMatch(alerts, input.Labels, podNameSet, input.Namespace, input.ClusterID, input.FleetMode)
}

// matchCandidate tracks all alert matches in one specificity/state bucket.
// Severity is usable only when every candidate carries the same non-empty raw
// label; source labels are never ordered by AF's canonical severity table.
type matchCandidate struct {
	found    bool
	severity string
	alert    string
}

func (m *matchCandidate) update(sev, alertName string) {
	if !m.found {
		m.found = true
		m.severity = sev
		m.alert = alertName
		return
	}
	if strings.TrimSpace(sev) == "" || strings.TrimSpace(m.severity) == "" || sev != m.severity {
		m.severity = ""
	}
}

func (m *matchCandidate) result(source Source) TriageResult {
	return TriageResult{
		Severity:  m.severity,
		Source:    source,
		AlertName: m.alert,
	}
}

// bestOverallMatch scans all firing/pending alerts once and returns the
// single best match across both specificity (resource > namespace >
// cluster) and state (firing > pending), with specificity taking strict
// priority over state (#2018/#2021): a resource- or namespace-scoped alert
// -- even only "pending" -- must never lose to a cluster-scoped alert
// merely because the cluster alert happens to be "firing".
//
// Before this fix, runTier1 ran two sequential full-fallback passes (firing
// first, then pending only if firing found nothing at all). A persistently
// firing, unrelated cluster-scoped alert (no namespace label) would win the
// firing pass's cluster slot and return immediately -- before ever reaching
// the pending pass, where the *actual* target's own alert might have been
// found via resourceBest/nsBest but simply hadn't started firing yet.
//
// Within a single tier (resource, namespace, or cluster), firing still
// beats pending -- only the cross-tier priority changed.
// alertTier identifies which specificity bucket an alert was classified into.
type alertTier int

const (
	tierNone alertTier = iota
	tierResource
	tierNamespace
	tierCluster
)

// classifyAlertTier determines the specificity tier and firing/pending state
// of a single alert, without mutating any shared state. Split out of
// bestOverallMatch to keep the latter's cyclomatic complexity below the
// gocyclo threshold (Issue #1532 baseline).
func classifyAlertTier(alert prom.Alert, targetLabels map[string]string, podNameSet map[string]struct{}, targetNamespace string) (tier alertTier, firing bool, sev, name string) {
	switch alert.State {
	case "firing":
		firing = true
	case "pending":
		firing = false
	default:
		return tierNone, false, "", ""
	}
	sev = alert.Labels["severity"]
	name = alert.Labels["alertname"]

	switch {
	case labelsOverlap(alert.Labels, targetLabels, podNameSet, targetNamespace):
		tier = tierResource
	case targetNamespace != "" && alert.Labels["namespace"] == targetNamespace:
		tier = tierNamespace
	case alert.Labels["namespace"] == "":
		tier = tierCluster
	default:
		tier = tierNone
	}
	return tier, firing, sev, name
}

// updateTierCandidate records a correlated source alert in its specificity
// and state bucket; conflicting source severities make that bucket unusable.
func updateTierCandidate(firing bool, sev, name string, firingCand, pendingCand *matchCandidate) {
	if firing {
		firingCand.update(sev, name)
	} else {
		pendingCand.update(sev, name)
	}
}

func (t *Triager) bestOverallMatch(alerts []prom.Alert, targetLabels map[string]string, podNameSet map[string]struct{}, targetNamespace, clusterID string, fleetMode bool) (TriageResult, bool) {
	var resourceFiring, resourcePending, nsFiring, nsPending, clusterFiring, clusterPending matchCandidate

	for _, alert := range alerts {
		if !matchesCluster(alert.Labels, clusterID, fleetMode) {
			continue
		}
		tier, firing, sev, name := classifyAlertTier(alert, targetLabels, podNameSet, targetNamespace)
		switch tier {
		case tierResource:
			updateTierCandidate(firing, sev, name, &resourceFiring, &resourcePending)
		case tierNamespace:
			updateTierCandidate(firing, sev, name, &nsFiring, &nsPending)
		case tierCluster:
			updateTierCandidate(firing, sev, name, &clusterFiring, &clusterPending)
		case tierNone:
			// Alert matched no tier (or an unrecognized state) -- ignored.
		}
	}

	switch {
	case resourceFiring.found:
		return resourceFiring.result(SourceFiringAlert), true
	case resourcePending.found:
		return resourcePending.result(SourcePendingAlert), true
	case nsFiring.found:
		return nsFiring.result(SourceNSFiringAlert), true
	case nsPending.found:
		return nsPending.result(SourceNSPendingAlert), true
	case clusterFiring.found:
		result := clusterFiring.result(SourceClusterFiringAlert)
		result.Ambiguous = true
		result.AmbiguityReason = AmbiguityReasonClusterScopedOnly
		return result, true
	case clusterPending.found:
		result := clusterPending.result(SourceClusterPendingAlert)
		result.Ambiguous = true
		result.AmbiguityReason = AmbiguityReasonClusterScopedOnly
		return result, true
	}
	return TriageResult{}, false
}

func (t *Triager) runTier15(input TriageInput, ruleGroups []prom.RuleGroup) (TriageResult, bool) {
	matchedRules := make([]prom.Rule, 0)
	for _, g := range ruleGroups {
		for _, r := range g.Rules {
			if matchesRule(r, input, "pending") {
				matchedRules = append(matchedRules, r)
			}
		}
	}
	if len(matchedRules) == 0 {
		return TriageResult{}, false
	}
	result, ok := ruleSeverity(matchedRules, SourcePendingAlert)
	if !ok {
		return TriageResult{}, true
	}
	return result, true
}

func (t *Triager) runTier2(ctx context.Context, input TriageInput, ruleGroups []prom.RuleGroup) (TriageResult, bool) {
	queryCount := 0
	matchedRules, queryMatched, activeRuleName, maxRulesExceeded := t.collectTier2Rules(ctx, input, ruleGroups, &queryCount)
	if len(matchedRules) == 0 {
		return TriageResult{}, false
	}
	if maxRulesExceeded {
		return TriageResult{}, true
	}

	source := SourceRuleLabel
	if queryMatched > 0 {
		source = SourceRuleEval
	}
	result, ok := ruleSeverity(matchedRules, source)
	if !ok {
		return TriageResult{}, true
	}
	if queryMatched == 1 {
		result.RuleName = activeRuleName
	}
	return result, true
}

func (t *Triager) collectTier2Rules(ctx context.Context, input TriageInput, ruleGroups []prom.RuleGroup, queryCount *int) ([]prom.Rule, int, string, bool) {
	matchedRules := make([]prom.Rule, 0)
	queryMatched := 0
	activeRuleName := ""

	for _, group := range ruleGroups {
		for _, rule := range group.Rules {
			if !matchesRule(rule, input, "inactive") {
				continue
			}
			if len(matchedRules) >= t.config.MaxRulesEvaluated {
				return matchedRules, queryMatched, activeRuleName, true
			}
			matchedRules = append(matchedRules, rule)
			result, _, found := t.evaluateTier2Rule(ctx, rule, input, queryCount)
			if found {
				queryMatched++
				if activeRuleName == "" {
					activeRuleName = result.RuleName
				}
			}
		}
	}
	return matchedRules, queryMatched, activeRuleName, false
}

func matchesRule(r prom.Rule, input TriageInput, state string) bool {
	if r.State != state || !matchesCluster(r.Labels, input.ClusterID, input.FleetMode) {
		return false
	}
	matchers, err := prom.ExtractLabelMatchers(r.Query)
	return err == nil && prom.MatchesResource(matchers, input.Labels)
}

func ruleSeverity(candidates []prom.Rule, source Source) (TriageResult, bool) {
	if len(candidates) == 0 {
		return TriageResult{}, false
	}
	severity := candidates[0].Labels["severity"]
	if strings.TrimSpace(severity) == "" {
		return TriageResult{}, false
	}
	for _, candidate := range candidates[1:] {
		candidateSeverity := candidate.Labels["severity"]
		if strings.TrimSpace(candidateSeverity) == "" || candidateSeverity != severity {
			return TriageResult{}, false
		}
	}
	result := TriageResult{Severity: severity, Source: source}
	if len(candidates) == 1 {
		result.RuleName = candidates[0].Name
	}
	return result, true
}

// evaluateTier2Rule checks whether a single Prometheus alerting rule
// correlates with input (Tier 2 label-matcher correlation), and if so,
// queries its expression to see if it is currently firing. matched
// indicates the rule should be recorded in matchedRules regardless of
// query outcome; found indicates a firing match was found and result is
// populated with the resolved severity.
func (t *Triager) evaluateTier2Rule(ctx context.Context, r prom.Rule, input TriageInput, queryCount *int) (result TriageResult, matched, found bool) {
	if !matchesRule(r, input, "inactive") {
		return TriageResult{}, false, false
	}

	if *queryCount >= t.config.MaxQueriesPerCall {
		return TriageResult{}, true, false
	}
	*queryCount++

	qr, qErr := t.promClient.InstantQuery(ctx, r.Query)
	if qErr != nil {
		t.logger.Info("Tier 2 query failed", "rule", r.Name, "error", qErr.Error())
		return TriageResult{}, true, false
	}
	if len(qr.Samples) == 0 {
		return TriageResult{}, true, false
	}

	return TriageResult{
		Severity: r.Labels["severity"],
		Source:   SourceRuleEval,
		RuleName: r.Name,
	}, true, true
}

// matchesCluster enforces exact Gateway attribution in fleet mode. A local
// Prometheus endpoint is already scoped to the local cluster, so local triage
// deliberately ignores any cluster ID or cluster label it happens to carry.
func matchesCluster(labels map[string]string, clusterID string, fleetMode bool) bool {
	if !fleetMode {
		return true
	}
	return clusterID != "" && labels[fleetClusterLabelKey] == clusterID
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
