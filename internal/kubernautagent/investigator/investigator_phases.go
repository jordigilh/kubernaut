/*
Copyright 2026 Jordi Gil.

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

package investigator

import (
	"context"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/enrichment"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/parser"
	"github.com/jordigilh/kubernaut/internal/kubernautagent/prompt"
	katypes "github.com/jordigilh/kubernaut/pkg/kubernautagent/types"
)

const maxK8sIdentifierLen = 253

// BuildPhase1Context extracts structured assessment fields from the Phase 1
// InvestigationResult for propagation into Phase 3 (HAPI parity: #715).
func BuildPhase1Context(rcaResult *katypes.InvestigationResult) *prompt.Phase1Data {
	if rcaResult == nil {
		return nil
	}
	return &prompt.Phase1Data{
		Severity:            rcaResult.Severity,
		ContributingFactors: rcaResult.ContributingFactors,
		RemediationTarget: prompt.Phase1RemediationTarget{
			Kind:      rcaResult.RemediationTarget.Kind,
			Name:      rcaResult.RemediationTarget.Name,
			Namespace: rcaResult.RemediationTarget.Namespace,
		},
		InvestigationOutcome:  rcaResult.InvestigationOutcome,
		Confidence:            rcaResult.Confidence,
		InvestigationAnalysis: rcaResult.InvestigationAnalysis,
		CausalChain:           rcaResult.CausalChain,
		DueDiligence:          rcaResult.DueDiligence,
	}
}

// MergePhase1Fallbacks applies Phase 1 assessment fields to the Phase 3 result
// when Phase 3 did not produce them. Matches HAPI's result.setdefault() pattern:
// Phase 3 values always take precedence; Phase 1 fills in gaps only.
func MergePhase1Fallbacks(result *katypes.InvestigationResult, p1 *prompt.Phase1Data) {
	if result == nil || p1 == nil {
		return
	}
	if result.Severity == "" && p1.Severity != "" {
		result.Severity = p1.Severity
	}
	if len(result.ContributingFactors) == 0 && len(p1.ContributingFactors) > 0 {
		result.ContributingFactors = p1.ContributingFactors
	}
	if result.Confidence == 0 && p1.Confidence > 0 {
		result.Confidence = p1.Confidence
	}
	if result.InvestigationOutcome == "" && p1.InvestigationOutcome != "" {
		result.InvestigationOutcome = p1.InvestigationOutcome
		parser.ApplyInvestigationOutcome(result, p1.InvestigationOutcome)
		// #301 defense-in-depth: Phase 1 problem_resolved overrides
		// contradictory HumanReviewNeeded set by Phase 3 (e.g. the
		// SubmitNoWorkflowResult branch hardcodes HumanReviewNeeded=true,
		// but that should not apply when the investigation is resolved).
		if p1.InvestigationOutcome == "problem_resolved" && result.HumanReviewNeeded {
			result.HumanReviewNeeded = false
			result.HumanReviewReason = ""
		}
	}
	if len(result.CausalChain) == 0 && len(p1.CausalChain) > 0 {
		result.CausalChain = p1.CausalChain
	}
	if result.DueDiligence == nil && p1.DueDiligence != nil {
		result.DueDiligence = p1.DueDiligence
	}
}

// isValidK8sIdentifier validates that s is safe for use as a K8s Kind or
// resource name in LLM prompts. Rejects path separators, control characters,
// and values exceeding the K8s name length limit (253 chars). Issue #1061 /
// FedRAMP SI-10.
func isValidK8sIdentifier(s string) bool {
	if strings.TrimSpace(s) == "" {
		return false
	}
	if utf8.RuneCountInString(s) > maxK8sIdentifierLen {
		return false
	}
	if strings.ContainsAny(s, "/\\") || strings.Contains(s, "..") {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// apiVersionPattern matches Kubernetes apiVersion strings: optional "group/"
// prefix followed by a version like "v1", "v1beta1", "v2alpha1". Groups follow
// DNS subdomain rules. Issue #1051 / FedRAMP SI-10.
var apiVersionPattern = regexp.MustCompile(`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\/)?v[0-9]+([a-z][a-z0-9]*)?$`)

// isValidAPIVersion validates that s is a well-formed Kubernetes apiVersion
// string (e.g. "v1", "apps/v1", "route.openshift.io/v1"). Rejects empty,
// overlong, or malformed values. Issue #1051 / FedRAMP SI-10.
func isValidAPIVersion(s string) bool {
	return s != "" && len(s) <= maxK8sIdentifierLen && apiVersionPattern.MatchString(s)
}

// LogLabelOverrideOrRejection emits structured logs when signal labels cause an
// override (FED-1/SRE-1) or when a non-empty label value was rejected by
// validation (SEC-6/FedRAMP AU-2). Called from runRCA and runWorkflowSelection.
func LogLabelOverrideOrRejection(logger logr.Logger, signal katypes.SignalContext, result prompt.SignalData, correlationID, phase string) {
	kindOverridden := result.ResourceKind != signal.ResourceKind
	nameOverridden := result.ResourceName != signal.ResourceName

	if kindOverridden || nameOverridden {
		logger.Info("signal label override applied to "+phase+" prompt",
			"original_kind", signal.ResourceKind,
			"original_name", signal.ResourceName,
			"override_kind", result.ResourceKind,
			"override_name", result.ResourceName,
			"correlation_id", correlationID)
	}

	if signal.SignalLabels == nil {
		return
	}
	if trk := signal.SignalLabels["target_resource_kind"]; trk != "" && trk != signal.ResourceKind && !kindOverridden {
		logger.Info("signal label override rejected: invalid target_resource_kind",
			"rejected_value", trk, "correlation_id", correlationID)
	}
	if trn := signal.SignalLabels["target_resource_name"]; trn != "" && trn != signal.ResourceName && !nameOverridden {
		logger.Info("signal label override rejected: invalid target_resource_name",
			"rejected_value", trn, "correlation_id", correlationID)
	}
}

// SyncSignalFromRCA reconciles the signal context with the RCA-identified
// remediation target. When the RCA identifies a different resource than the
// original alert target (cross-resource RCA), this function updates Kind,
// Name, Namespace, and APIVersion so that ComponentGVK() produces the
// correct GVK for workflow discovery catalog matching.
//
// Parity with the autonomous path at investigator.go L492-509:
//   - Kind, Name, Namespace are always synced from a non-empty RCA target
//   - APIVersion from RCA target is authoritative (overrides signal)
//   - Stale GVK guard: when Kind changes but RCA has no apiVersion,
//     the signal's apiVersion is cleared to prevent invalid combinations
//     (e.g. "security.istio.io/v1/Deployment")
//
// Value semantics: the original signal is not modified.
func SyncSignalFromRCA(signal katypes.SignalContext, target katypes.RemediationTarget) katypes.SignalContext {
	if target.Kind == "" {
		return signal
	}

	if !isValidK8sIdentifier(target.Kind) {
		return signal
	}

	originalKind := signal.ResourceKind
	signal.ResourceKind = target.Kind
	if target.Name != "" && isValidK8sIdentifier(target.Name) {
		signal.ResourceName = target.Name
	}
	signal.Namespace = target.Namespace

	if target.APIVersion != "" && isValidAPIVersion(target.APIVersion) {
		signal.ResourceAPIVersion = target.APIVersion
	} else if signal.ResourceKind != originalKind {
		signal.ResourceAPIVersion = ""
	}
	return signal
}

// FinalizeWorkflowResult applies post-Phase 3 processing to the workflow
// discovery result, mirroring the autonomous path in Investigate() L558-569.
// This ensures interactive discover_workflows results have the same
// completeness as autonomous results: severity backfill, detected labels,
// authoritative remediation target, and TARGET_RESOURCE_* parameters.
//
// enrichData may be nil when the interactive path lacks full enrichment (F5).
// All callees handle nil gracefully.
func FinalizeWorkflowResult(result *katypes.InvestigationResult, signal katypes.SignalContext, rcaResult *katypes.InvestigationResult, enrichData *enrichment.EnrichmentResult) {
	if result == nil {
		return
	}
	backfillSeverity(result, signal)
	attachDetectedLabels(result, enrichData)
	InjectRemediationTarget(result, signal, enrichData)
	if result.RemediationTarget.APIVersion == "" && rcaResult != nil && rcaResult.RemediationTarget.APIVersion != "" {
		result.RemediationTarget.APIVersion = rcaResult.RemediationTarget.APIVersion
	}
	InjectTargetResourceParameters(result)
}

// ApplySignalLabelOverrides returns a copy of signal with ResourceKind,
// ResourceName, and ResourceAPIVersion overridden by corresponding
// target_resource_* signal labels, when present and valid per FedRAMP SI-10.
// The original signal is not modified (value semantics).
//
// Consistency guards (#1051):
//   - When target_resource_kind is overridden without an explicit
//     target_resource_api_version, ResourceAPIVersion is cleared to prevent
//     an invalid GVK combination (stale apiVersion + new kind).
//   - target_resource_api_version is only applied when target_resource_kind
//     is also present and valid, to prevent a semantically inconsistent GVK
//     where the apiVersion does not match the existing kind.
//
// Issue #1064: used by both SignalToPrompt (LLM prompt) and runWorkflowSelection
// (tool context) to ensure consistent override application.
func ApplySignalLabelOverrides(signal katypes.SignalContext) katypes.SignalContext {
	originalKind := signal.ResourceKind
	kindOverridden := false
	if trk := signal.SignalLabels["target_resource_kind"]; trk != "" && isValidK8sIdentifier(trk) {
		signal.ResourceKind = trk
		kindOverridden = true
	}
	if trn := signal.SignalLabels["target_resource_name"]; trn != "" && isValidK8sIdentifier(trn) {
		signal.ResourceName = trn
	}
	if trav := signal.SignalLabels["target_resource_api_version"]; trav != "" && isValidAPIVersion(trav) && kindOverridden {
		signal.ResourceAPIVersion = trav
	} else if kindOverridden && signal.ResourceKind != originalKind {
		signal.ResourceAPIVersion = ""
	}
	return signal
}

// SignalToPrompt converts a SignalContext to prompt.SignalData.
// Issue #1061: when the alert carries explicit target_resource_kind /
// target_resource_name labels, those override the enrichment-resolved
// ResourceKind / ResourceName so the LLM prompt references the actual
// remediation target instead of the namespace container.
// Label values are validated per FedRAMP SI-10 before use.
func SignalToPrompt(s katypes.SignalContext) prompt.SignalData {
	overridden := ApplySignalLabelOverrides(s)
	return prompt.SignalData{
		Name:                       s.Name,
		Namespace:                  s.Namespace,
		Severity:                   s.Severity,
		Message:                    s.Message,
		ResourceKind:               overridden.ResourceKind,
		ResourceName:               overridden.ResourceName,
		ClusterName:                s.ClusterName,
		Environment:                s.Environment,
		Priority:                   s.Priority,
		RiskTolerance:              s.RiskTolerance,
		SignalSource:               s.SignalSource,
		BusinessCategory:           s.BusinessCategory,
		Description:                s.Description,
		SignalMode:                 s.SignalMode,
		FiringTime:                 s.FiringTime,
		ReceivedTime:               s.ReceivedTime,
		IsDuplicate:                s.IsDuplicate,
		OccurrenceCount:            s.OccurrenceCount,
		DeduplicationWindowMinutes: s.DeduplicationWindowMinutes,
		FirstSeen:                  s.FirstSeen,
		LastSeen:                   s.LastSeen,
		SignalAnnotations:          s.SignalAnnotations,
	}
}

func toPromptEnrichment(data *enrichment.EnrichmentResult) *prompt.EnrichmentData {
	if data == nil {
		return nil
	}
	pe := &prompt.EnrichmentData{}

	for _, entry := range data.OwnerChain {
		if entry.Namespace != "" {
			pe.OwnerChain = append(pe.OwnerChain, entry.Kind+"/"+entry.Name+"("+entry.Namespace+")")
		} else {
			pe.OwnerChain = append(pe.OwnerChain, entry.Kind+"/"+entry.Name)
		}
	}

	if data.DetectedLabels != nil {
		pe.DetectedLabels = detectedLabelsToPromptMap(data.DetectedLabels)
	}

	if len(data.QuotaDetails) > 0 {
		pe.QuotaDetails = data.QuotaDetails
	}

	pe.HistoryResult = data.RemediationHistory
	return pe
}

func detectedLabelsToPromptMap(dl *enrichment.DetectedLabels) map[string]string {
	m := make(map[string]string, 14)
	if dl.GitOpsManaged {
		m["gitOpsManaged"] = "true"
		if dl.GitOpsTool != "" {
			m["gitOpsTool"] = dl.GitOpsTool
		}
	}
	if dl.HPAEnabled {
		m["hpaEnabled"] = "true"
	}
	if dl.PDBProtected {
		m["pdbProtected"] = "true"
	}
	if dl.Stateful {
		m["stateful"] = "true"
	}
	if dl.HelmManaged {
		m["helmManaged"] = "true"
	}
	if dl.NetworkIsolated {
		m["networkIsolated"] = "true"
	}
	if dl.ServiceMesh != "" {
		m["serviceMesh"] = dl.ServiceMesh
	}
	if dl.ResourceQuotaConstrained {
		m["resourceQuotaConstrained"] = "true"
	}
	if dl.VirtualMachine {
		m["virtualMachine"] = "true"
	}
	if dl.LiveMigratable {
		m["liveMigratable"] = "true"
	}
	if dl.CDIManaged {
		m["cdiManaged"] = "true"
	}
	if dl.StorageBackend != "" {
		m["storageBackend"] = dl.StorageBackend
	}
	if len(dl.FailedDetections) > 0 {
		m["failedDetections"] = strings.Join(dl.FailedDetections, ",")
	}
	return m
}

func detectedLabelsToResult(dl *enrichment.DetectedLabels) map[string]interface{} {
	m := make(map[string]interface{}, 14)
	m["gitOpsManaged"] = dl.GitOpsManaged
	if dl.GitOpsTool != "" {
		m["gitOpsTool"] = dl.GitOpsTool
	}
	m["hpaEnabled"] = dl.HPAEnabled
	m["pdbProtected"] = dl.PDBProtected
	m["stateful"] = dl.Stateful
	m["helmManaged"] = dl.HelmManaged
	m["networkIsolated"] = dl.NetworkIsolated
	if dl.ServiceMesh != "" {
		m["serviceMesh"] = dl.ServiceMesh
	}
	m["resourceQuotaConstrained"] = dl.ResourceQuotaConstrained
	m["virtualMachine"] = dl.VirtualMachine
	m["liveMigratable"] = dl.LiveMigratable
	m["cdiManaged"] = dl.CDIManaged
	if dl.StorageBackend != "" {
		m["storageBackend"] = dl.StorageBackend
	}
	if len(dl.FailedDetections) > 0 {
		m["failedDetections"] = dl.FailedDetections
	}
	return m
}

func attachDetectedLabels(result *katypes.InvestigationResult, enrichData *enrichment.EnrichmentResult) {
	if result == nil || enrichData == nil || enrichData.DetectedLabels == nil {
		return
	}
	result.DetectedLabels = detectedLabelsToResult(enrichData.DetectedLabels)
}

// InjectRemediationTarget resolves the authoritative remediation target.
//
// Root owner resolution priority:
//  1. Owner chain last entry (most common: Pod → RS → Deployment)
//  2. Enrichment source identity (after re-enrichment, the chain is empty
//     because the enriched resource IS the root — see #694)
//  3. Signal identity (fallback when enrichData is nil)
//
// LLM target handling:
//   - Kind == "" or same as root: override with K8s-verified root identity
//   - Different Kind (cross-type): preserve the LLM's target
func InjectRemediationTarget(result *katypes.InvestigationResult, signal katypes.SignalContext, enrichData *enrichment.EnrichmentResult) {
	if result == nil {
		return
	}
	rootKind := signal.ResourceKind
	rootName := signal.ResourceName
	rootNS := signal.Namespace
	if enrichData != nil && len(enrichData.OwnerChain) > 0 {
		root := enrichData.OwnerChain[len(enrichData.OwnerChain)-1]
		rootKind = root.Kind
		rootName = root.Name
		rootNS = root.Namespace
	} else if enrichData != nil && enrichData.ResourceKind != "" {
		rootKind = enrichData.ResourceKind
		rootName = enrichData.ResourceName
		rootNS = enrichData.ResourceNamespace
	}

	llmKind := result.RemediationTarget.Kind
	llmAPIVersion := result.RemediationTarget.APIVersion
	if llmKind == "" || llmKind == rootKind {
		// Same kind as root: preserve LLM's apiVersion (same kind = same API group). #1040
		apiVersion := llmAPIVersion
		if apiVersion == "" && enrichData != nil && len(enrichData.OwnerChain) > 0 {
			apiVersion = enrichData.OwnerChain[len(enrichData.OwnerChain)-1].APIVersion
		}
		result.RemediationTarget = katypes.RemediationTarget{
			Kind:       rootKind,
			Name:       rootName,
			Namespace:  rootNS,
			APIVersion: apiVersion,
		}
		return
	}

	// BR-496 v2 / BR-HAPI-261 AC#5: if the LLM's kind is a descendant
	// in the ownership hierarchy (e.g. Pod when root is Deployment),
	// resolve upward to the K8s-verified root owner. Only preserve
	// the LLM's target when its kind is genuinely cross-type (not in
	// the owner chain at all, e.g. Node vs Deployment).
	if enrichData != nil && isKindInOwnerChain(llmKind, signal.ResourceKind, enrichData.OwnerChain) {
		// Use root owner's APIVersion from the chain if available. #1040
		rootAPIVersion := ""
		if len(enrichData.OwnerChain) > 0 {
			rootAPIVersion = enrichData.OwnerChain[len(enrichData.OwnerChain)-1].APIVersion
		}
		result.RemediationTarget = katypes.RemediationTarget{
			Kind:       rootKind,
			Name:       rootName,
			Namespace:  rootNS,
			APIVersion: rootAPIVersion,
		}
		return
	}
}

func isKindInOwnerChain(kind, signalKind string, chain []enrichment.OwnerChainEntry) bool {
	if strings.EqualFold(kind, signalKind) {
		return true
	}
	for _, entry := range chain {
		if strings.EqualFold(entry.Kind, kind) {
			return true
		}
	}
	return false
}

// InjectTargetResourceParameters ensures the decomposed TARGET_RESOURCE_*
// environment variables are present in result.Parameters, derived from
// the authoritative RemediationTarget. Safe to call multiple times.
func InjectTargetResourceParameters(result *katypes.InvestigationResult) {
	if result == nil || result.RemediationTarget.Kind == "" {
		return
	}
	if result.Parameters == nil {
		result.Parameters = make(map[string]interface{})
	}
	result.Parameters["TARGET_RESOURCE_NAME"] = result.RemediationTarget.Name
	result.Parameters["TARGET_RESOURCE_KIND"] = result.RemediationTarget.Kind
	result.Parameters["TARGET_RESOURCE_NAMESPACE"] = result.RemediationTarget.Namespace
	if result.RemediationTarget.APIVersion != "" {
		result.Parameters["TARGET_RESOURCE_API_VERSION"] = result.RemediationTarget.APIVersion
	}
}

func allLabelDetectionsFailed(labels *enrichment.DetectedLabels) bool {
	if labels == nil {
		return false
	}
	return len(labels.FailedDetections) >= len(enrichment.AllDetectionCategories)
}

// EnrichmentCacheKey returns the dedup cache key for a (kind, name, namespace) tuple (#764).
func EnrichmentCacheKey(kind, name, namespace string) string {
	return kind + "/" + name + "/" + namespace
}

func (inv *Investigator) resolveEnrichmentCached(ctx context.Context, cache map[string]*enrichment.EnrichmentResult, kind, name, namespace, incidentID string) *enrichment.EnrichmentResult {
	key := EnrichmentCacheKey(kind, name, namespace)
	if cached, ok := cache[key]; ok {
		inv.logger.Info("enrichment cache hit, reusing cached result",
			"kind", kind, "name", name, "namespace", namespace)
		return cached
	}
	result := inv.resolveEnrichment(ctx, kind, name, namespace, incidentID)
	cache[key] = result
	return result
}

func (inv *Investigator) normalizeNamespace(kind, namespace string) string {
	if inv.scopeResolver == nil {
		return namespace
	}
	isCluster, err := inv.scopeResolver.IsClusterScoped(kind)
	if err != nil {
		inv.logger.Error(err, "ScopeResolver error, preserving namespace",
			"kind", kind)
		return namespace
	}
	if isCluster {
		return ""
	}
	return namespace
}

type mapperScopeResolver struct {
	mapper meta.RESTMapper
}

// NewMapperScopeResolver creates a ScopeResolver backed by a RESTMapper.
func NewMapperScopeResolver(mapper meta.RESTMapper) ScopeResolver {
	return &mapperScopeResolver{mapper: mapper}
}

func (r *mapperScopeResolver) IsClusterScoped(kind string) (bool, error) {
	plural := strings.ToLower(kind) + "s"
	gvr, err := r.mapper.ResourceFor(schema.GroupVersionResource{Resource: plural})
	if err != nil {
		return false, err
	}
	gvk, err := r.mapper.KindFor(gvr)
	if err != nil {
		return false, err
	}
	mapping, err := r.mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return false, err
	}
	return mapping.Scope.Name() != meta.RESTScopeNameNamespace, nil
}

// IsAmbiguousKind returns true when more than one API group defines resources
// for the given kind, along with all matching GVRs. This indicates a CRD kind
// collision where the REST mapper cannot resolve a unique GVR without an
// explicit apiVersion. Issue #1044.
func (r *mapperScopeResolver) IsAmbiguousKind(kind string) (bool, []schema.GroupVersionResource, error) {
	if kind == "" {
		return false, nil, nil
	}
	resource := strings.ToLower(kind)
	gvrs, err := r.mapper.ResourcesFor(schema.GroupVersionResource{Resource: resource})
	if err != nil {
		if meta.IsNoMatchError(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	groups := make(map[string]struct{})
	for _, gvr := range gvrs {
		groups[gvr.Group] = struct{}{}
	}
	return len(groups) > 1, gvrs, nil
}
