package severity

import (
	"fmt"
	"strings"

	prom "github.com/jordigilh/kubernaut/pkg/apifrontend/prometheus"
)

// Source identifies which triage tier determined the severity.
type Source string

// Triage source constants identify which pipeline tier produced the severity.
const (
	SourceFiringAlert  Source = "firing_alert"
	SourcePendingAlert Source = "pending_alert"
	// SourceNSFiringAlert indicates a namespace-scoped firing alert correlation.
	// The alert's namespace label matched the target but no resource-specific
	// key overlap was found. This is a broader correlation than resource-specific
	// matching -- downstream consumers should weigh accordingly.
	SourceNSFiringAlert Source = "ns_firing_alert"
	// SourceNSPendingAlert is the pending equivalent of SourceNSFiringAlert.
	SourceNSPendingAlert Source = "ns_pending_alert"
	// SourceClusterFiringAlert indicates a cluster-scoped firing alert correlation.
	// The alert has no namespace label, so it is matched to any investigation target.
	// This is the broadest correlation tier -- the alert represents a cluster-wide
	// condition that may or may not be related to the specific workload under triage.
	//
	// #2018/#2021: cluster-scoped tiers are the LAST fallback, checked only after BOTH
	// firing and pending resource-scoped and namespace-scoped alerts have been
	// ruled out (bestOverallMatch in triage.go). Specificity always outranks
	// state -- a merely "pending" resource-specific alert still beats a
	// continuously "firing" cluster-scoped alert, since the latter carries no
	// evidence of actually being related to the target workload.
	SourceClusterFiringAlert Source = "cluster_firing_alert"
	// SourceClusterPendingAlert is the pending equivalent of SourceClusterFiringAlert.
	SourceClusterPendingAlert Source = "cluster_pending_alert"
	SourceRuleEval            Source = "rule_evaluation"
	SourceRuleLabel           Source = "rule_label"
	SourceLLMRuleInform       Source = "llm_rule_informed"
	SourceLLMTriage           Source = "llm_triage"
)

// TriageInput holds the resource context for severity triage.
type TriageInput struct {
	Namespace   string
	Kind        string
	Name        string
	Description string
	Labels      map[string]string
	PodNames    []string // Resolved pod names for alert correlation (auto-populated by Triager when PodResolver is set)
	// ClusterID is the MCP Gateway cluster name used to scope fleet alert and
	// rule correlation. It is ignored when FleetMode is false, because a local
	// Prometheus endpoint represents only the local cluster.
	ClusterID string
	// FleetMode enables exact cluster attribution. In fleet mode ClusterID must
	// be non-empty and match the Prometheus "cluster" label; otherwise no alert
	// or rule is eligible. Local mode ignores ClusterID and cluster labels.
	FleetMode bool
	// ConfirmedSignalName, when non-empty and it exactly matches an
	// ambiguous candidate's AlertName, indicates the user has already
	// confirmed that specific weak candidate (DD-AF-012). Triage() bypasses
	// its ambiguity gate only for this exact match -- a different candidate
	// on a later call still fails closed and asks again.
	ConfirmedSignalName string
	// APIGroup is the target resource's own API group (#2658), parsed from
	// the mandatory apiVersion at the call site. Empty means the caller
	// carries no group (legacy inputs) and the group-scope gate does not
	// engage.
	APIGroup string
	// APIGroupKnown tells the triager that APIGroup was actually derived
	// from the target's apiVersion, so the empty string means the core API
	// group rather than "unknown" (#2658). Production callers derive it
	// from the validated, mandatory apiVersion; legacy inputs leave it
	// false and the group-scope gate stays off. Without this flag a
	// core-group target could never be gated, even though core is a
	// legitimate sibling group for a Kind registered in several groups
	// (e.g. Event in core and events.k8s.io).
	APIGroupKnown bool
	// ConfirmedAPIGroup, when non-empty and it normalizes to the target's
	// own APIGroup, indicates the user has confirmed that the ambiguous
	// candidate alert refers to THIS group's resource (#2658). It is the
	// identity-confirmation counterpart of ConfirmedSignalName: a signal
	// name can never resolve a group ambiguity, and a group can never
	// resolve a cluster-scoped ambiguity.
	ConfirmedAPIGroup string
}

// TriageResult holds the outcome of the severity triage pipeline.
type TriageResult struct {
	Severity   string
	Source     Source
	AlertName  string
	RuleName   string
	Confidence float64
	// Ambiguous is true when the only correlating evidence found is a
	// cluster-scoped alert with no verified relationship to the target
	// resource (DD-AF-012, #2027/#2028), or -- #2658 -- a positive match
	// that cannot be scoped to the target's API group because a same
	// namespace/kind/name resource exists in another group. Only Tier 1's
	// cluster-scoped branches and the #2658 group-scope gate set this;
	// unambiguous resource- and namespace-scoped matches are never
	// ambiguous.
	Ambiguous bool
	// AmbiguityReason classifies an Ambiguous result (#2658):
	// cluster_scoped_only (DD-AF-012 lineage, resolvable via
	// ConfirmedSignalName), group_unscopable (a cross-group sibling exists,
	// resolvable only via ConfirmedAPIGroup), or alert_belongs_to_sibling
	// (the user confirmed the candidate belongs to the sibling's group --
	// severity must not be inherited).
	AmbiguityReason string
	// CandidateAPIGroups lists the groups in which cross-group siblings were
	// found (group_unscopable only).
	CandidateAPIGroups []string
	// CandidateIdentifier carries the confirmable candidate identity for
	// ambiguous results. For cluster_scoped_only it mirrors AlertName; for
	// group_unscopable rule-tier results it carries the rule name, because
	// an alert name must never be usable to bypass the group gate.
	CandidateIdentifier string
}

// Ambiguity reason values (#2658). All are new surfaces: before #2658 the
// ambiguous path carried no reason dimension.
const (
	// AmbiguityReasonClusterScopedOnly marks the DD-AF-012 ambiguity class:
	// only a cluster-scoped alert correlates, with no verified relationship.
	// Resolvable via ConfirmedSignalName.
	AmbiguityReasonClusterScopedOnly = "cluster_scoped_only"
	// AmbiguityReasonGroupUnscopable marks the #2658 ambiguity class: a
	// positive correlation cannot be scoped to the target's API group
	// because a same-namespace/kind/name sibling exists in another group
	// (or the sibling check was unavailable and failed closed). Resolvable
	// only via ConfirmedAPIGroup.
	AmbiguityReasonGroupUnscopable = "group_unscopable"
	// AmbiguityReasonAlertBelongsToSibling marks the resolved-negative case:
	// the user confirmed the candidate alert refers to the sibling group's
	// resource, so its severity must not be inherited by this target.
	AmbiguityReasonAlertBelongsToSibling = "alert_belongs_to_sibling"
)

// Canonical severity values (ADR-066). Severity is kept as a plain string
// (not a named type) on TriageResult for JSON/API compatibility.
const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityWarning  = "warning"
	SeverityInfo     = "info"
)

var severityRank = map[string]int{
	SeverityCritical: 5,
	SeverityHigh:     4,
	SeverityWarning:  3,
	SeverityInfo:     1,
}

var validSeverities = map[string]bool{
	SeverityCritical: true,
	SeverityHigh:     true,
	SeverityWarning:  true,
	SeverityInfo:     true,
}

// ValidateSeverity checks if the string is a valid canonical severity value.
func ValidateSeverity(s string) bool {
	return validSeverities[s]
}

// NormalizeSeverity lowercases and validates the severity string.
// Returns SeverityWarning as default for invalid/empty input (ADR-066).
func NormalizeSeverity(s string) string {
	lower := strings.TrimSpace(strings.ToLower(s))
	if validSeverities[lower] {
		return lower
	}
	return SeverityWarning
}

// CompareSeverity returns > 0 if a is higher severity than b, < 0 if lower, 0 if equal.
func CompareSeverity(a, b string) int {
	return severityRank[a] - severityRank[b]
}

// HighestSeverity returns the highest severity string from a slice.
// Returns empty string for empty input.
func HighestSeverity(severities []string) string {
	if len(severities) == 0 {
		return ""
	}
	best := severities[0]
	for _, s := range severities[1:] {
		if CompareSeverity(s, best) > 0 {
			best = s
		}
	}
	return best
}

// SensitiveKeys lists label keys that should not appear in LLM prompts.
// Exported for consistency testing against tools.sensitiveAlertKeys (#1367 F4).
var SensitiveKeys = map[string]bool{
	"password": true, "token": true, "secret": true,
	"key": true, "credential": true, "bearer": true,
}

// BuildTriagePrompt constructs the LLM prompt for severity triage.
// Filters sensitive labels from the input.
func BuildTriagePrompt(input TriageInput, rules interface{}) string {
	var sb strings.Builder
	sb.WriteString("Classify the severity of the following Kubernetes incident.\n\n")
	fmt.Fprintf(&sb, "Resource: %s/%s in namespace %s\n", input.Kind, input.Name, input.Namespace)
	fmt.Fprintf(&sb, "Description: %s\n\n", input.Description)

	sb.WriteString("Resource labels:\n")
	for k, v := range input.Labels {
		if SensitiveKeys[strings.ToLower(k)] {
			continue
		}
		fmt.Fprintf(&sb, "  %s: %s\n", k, v)
	}

	if ruleSlice, ok := rules.([]prom.Rule); ok && len(ruleSlice) > 0 {
		sb.WriteString("\nMatching alerting rules (could not evaluate due to insufficient data):\n")
		for _, r := range ruleSlice {
			fmt.Fprintf(&sb, "  - %s: %s (configured severity: %s)\n", r.Name, r.Query, r.Labels["severity"])
			if summary, exists := r.Annotations["summary"]; exists {
				fmt.Fprintf(&sb, "    Summary: %s\n", summary)
			}
		}
	}

	sb.WriteString("\nRespond with exactly one of: critical, high, warning, info\n")
	return sb.String()
}
