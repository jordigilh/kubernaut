package policyread

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	policyScopeCapability         = "kubernaut_read_policy_scope"
	managedResourceListCapability = "kubernaut_list_managed_resources"
	managedResourceGetCapability  = "kubernaut_get_managed_resource"
)

// Service owns the Console projection boundary. It never exposes source
// objects directly and never selects a different cluster when the requested
// source is unavailable.
type Service struct {
	config       Config
	dependencies Dependencies
	threshold    float64
	historyLimit int
}

// NewService validates the operator-rendered projection settings and creates a
// read-only service. An empty PolicyRead block is permitted for legacy callers
// that use only inventory reads; policy reads fail closed until configured.
func NewService(cfg Config, deps Dependencies) (*Service, error) {
	service := &Service{
		config:       cfg,
		dependencies: deps,
		historyLimit: cfg.HistoryLimit,
	}
	if service.dependencies.ReadGuard == nil {
		service.dependencies.ReadGuard = NewReadGuard(DefaultInventoryMaxConcurrentReads)
	}
	if service.config.BasePath == "" {
		service.config.BasePath = "/api/v1/console"
	}
	if service.historyLimit <= 0 {
		service.historyLimit = DefaultHistoryLimit
	}

	configured := cfg.PolicyRead.AIAnalysis.ConfigMapName != "" ||
		cfg.PolicyRead.AIAnalysis.ConfidenceThreshold != "" ||
		cfg.PolicyRead.SignalProcessing.ConfigMapName != ""
	if !configured {
		return service, nil
	}
	if cfg.PolicyRead.AIAnalysis.ConfigMapName == "" || cfg.PolicyRead.SignalProcessing.ConfigMapName == "" {
		return nil, fmt.Errorf("policyRead requires both policy ConfigMap names")
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(cfg.PolicyRead.AIAnalysis.ConfidenceThreshold), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 || value > 1 {
		return nil, fmt.Errorf("policyRead.aiAnalysis.confidenceThreshold must be finite and in range (0.0, 1.0]")
	}
	service.threshold = value
	return service, nil
}

// ListInventory returns a bounded canonical inventory page after capability
// and local target-scope authorization.
func (s *Service) ListInventory(ctx context.Context, user *auth.UserIdentity, filters inventory.Filters) (page inventory.Page, err error) {
	state := inventory.StateUnavailable
	defer func() {
		if !auditBoundaryRequested(ctx) {
			s.emitRead(ctx, user, audit.EventConsoleManagedResourceListRead, filters.ClusterID, state, filterFingerprint(filters), "")
		}
	}()

	normalized, err := filters.Normalize()
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	if err = s.authorize(ctx, user, managedResourceListCapability, normalized, false); err != nil {
		state = StateOf(err)
		return inventory.StateOnly(state), err
	}
	if normalized.ClusterID != LocalClusterID {
		err = stateError(inventory.StateUnavailable, errors.New("remote inventory list unavailable"))
		state = StateOf(err)
		return inventory.StateOnly(state), err
	}

	page, err = s.readInventoryPage(ctx, normalized)
	if err != nil {
		state = StateOf(err)
		return inventory.StateOnly(state), err
	}
	state = page.State
	return page, nil
}

// GetInventory returns one managed canonical resource. A local target denial
// is intentionally mapped to not_found to prevent existence probing.
func (s *Service) GetInventory(ctx context.Context, user *auth.UserIdentity, filters inventory.Filters, resourceID string) (resource inventory.Resource, err error) {
	state := inventory.StateUnavailable
	defer func() {
		if !auditBoundaryRequested(ctx) {
			s.emitRead(ctx, user, audit.EventConsoleManagedResourceGetRead, filters.ClusterID, state, "", resourceFingerprint(resourceID))
		}
	}()

	if err = inventory.ValidateResourceID(resourceID); err != nil {
		return inventory.Resource{}, err
	}
	filters.ResourceID = resourceID
	normalized, err := filters.Normalize()
	if err != nil {
		return inventory.Resource{}, err
	}
	if err = s.authorize(ctx, user, managedResourceGetCapability, normalized, true); err != nil {
		state = StateOf(err)
		return inventory.Resource{}, err
	}

	reader, err := s.readerFor(normalized.ClusterID)
	if err != nil {
		state = inventory.StateUnavailable
		return inventory.Resource{}, err
	}
	resource, err = reader.Get(ctx, normalized.ClusterID, resourceID)
	if err != nil {
		state = stateForInventoryError(err, inventory.StateUnavailable)
		return inventory.Resource{}, stateError(state, err)
	}
	if err := validateResource(resource, normalized.ClusterID, resourceID); err != nil {
		state = stateForInventoryError(err, inventory.StateUnavailable)
		return inventory.Resource{}, stateError(state, err)
	}
	if !resource.Scope.Included {
		state = inventory.StateNotFound
		return inventory.Resource{}, stateError(state, inventory.ErrNotFound)
	}

	if err := s.authorizeInventoryTarget(ctx, user, resource, normalized.ClusterID); err != nil {
		state = StateOf(err)
		return inventory.Resource{}, err
	}

	state = inventory.StateSuccess
	return resource, nil
}

// ListPolicyScope returns current policy metadata and bounded historical
// evaluations joined to each resource in a complete inventory page.
func (s *Service) ListPolicyScope(ctx context.Context, user *auth.UserIdentity, filters inventory.Filters) (result PolicyScopePage, err error) {
	state := inventory.StateUnavailable
	defer func() {
		if !auditBoundaryRequested(ctx) {
			s.emitRead(ctx, user, audit.EventConsolePolicyScopeRead, filters.ClusterID, state, filterFingerprint(filters), "")
		}
	}()

	normalized, err := filters.Normalize()
	if err != nil {
		return stateOnlyPolicyPage(inventory.StateUnavailable), err
	}
	if err = s.authorize(ctx, user, policyScopeCapability, normalized, false); err != nil {
		state = StateOf(err)
		return stateOnlyPolicyPage(state), err
	}
	if normalized.ClusterID != LocalClusterID {
		err = stateError(inventory.StateUnavailable, errors.New("remote policy-scope list unavailable"))
		state = StateOf(err)
		return stateOnlyPolicyPage(state), err
	}

	operationCtx, cancel := context.WithTimeout(ctx, DefaultInventoryOperationTimeout)
	defer cancel()
	guardCtx, release, err := s.dependencies.ReadGuard.Acquire(operationCtx)
	if err != nil {
		state = inventory.StateUnavailable
		return stateOnlyPolicyPage(state), stateError(state, err)
	}
	defer release()
	guardCtx = context.WithValue(guardCtx, workingSetContextKey{}, &retainedWorkingSet{limit: DefaultInventoryWorkingSetBytes})

	page, err := s.readInventoryPage(guardCtx, normalized)
	if err != nil {
		state = StateOf(err)
		return stateOnlyPolicyPage(state), err
	}
	state = page.State
	result = PolicyScopePage{
		APIVersion:   PolicyScopeAPIVersion,
		Kind:         PolicyScopeKind,
		State:        page.State,
		ObservedAt:   page.ObservedAt,
		Completeness: page.Completeness,
		NextCursor:   page.NextCursor,
	}
	if page.State == inventory.StateEmpty {
		return result, nil
	}

	policies, err := s.readCurrentPolicies(guardCtx)
	if err != nil {
		state = inventory.StateUnavailable
		return stateOnlyPolicyPage(state), stateError(state, err)
	}
	history, err := s.readHistory(guardCtx, page.Items)
	if err != nil {
		state = inventory.StateUnavailable
		return stateOnlyPolicyPage(state), stateError(state, err)
	}

	result.Items = make([]PolicyScopeItem, 0, len(page.Items))
	for _, resource := range page.Items {
		result.Items = append(result.Items, PolicyScopeItem{
			ID:                    resource.ID,
			Resource:              resource,
			CurrentPolicies:       clonePolicies(policies),
			HistoricalEvaluations: historyForResource(history, resource, s.historyLimit),
			InventoryLink:         s.inventoryLink(resource),
		})
	}
	return result, nil
}

func (s *Service) readInventoryPage(ctx context.Context, filters inventory.Filters) (inventory.Page, error) {
	reader, err := s.readerFor(filters.ClusterID)
	if err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), err
	}
	page, err := reader.List(ctx, filters)
	if err != nil {
		state := stateForInventoryError(err, inventory.StateUnavailable)
		return inventory.StateOnly(state), stateError(state, err)
	}
	if filters.ClusterID == LocalClusterID && (page.State == inventory.StatePartial ||
		(page.State == inventory.StateSuccess && page.Completeness != inventory.CompletenessComplete)) {
		err := fmt.Errorf("%w: local inventory page is incomplete", inventory.ErrUnavailable)
		return inventory.StateOnly(inventory.StatePartial), stateError(inventory.StatePartial, err)
	}
	if err := page.Validate(); err != nil {
		return inventory.StateOnly(inventory.StateUnavailable), stateError(inventory.StateUnavailable, err)
	}
	if page.State != inventory.StateSuccess && page.State != inventory.StateEmpty {
		return inventory.StateOnly(page.State), stateError(page.State, errors.New(string(page.State)))
	}
	for _, resource := range page.Items {
		if err := validateResource(resource, filters.ClusterID, resource.ID); err != nil || !resource.Scope.Included {
			if err == nil {
				err = inventory.ErrUnavailable
			}
			return inventory.StateOnly(inventory.StateUnavailable), stateError(inventory.StateUnavailable, err)
		}
	}
	return page, nil
}

func (s *Service) readerFor(clusterID string) (InventoryReader, error) {
	if clusterID == LocalClusterID {
		if s.dependencies.LocalInventory == nil {
			return nil, errors.New("local inventory unavailable")
		}
		return s.dependencies.LocalInventory, nil
	}
	if s.dependencies.RemoteInventory == nil {
		return nil, errors.New("remote inventory unavailable")
	}
	return s.dependencies.RemoteInventory, nil
}

func (s *Service) authorize(ctx context.Context, user *auth.UserIdentity, capability string, filters inventory.Filters, exact bool) error {
	if user == nil || user.Username == "" {
		return stateError(inventory.StateForbidden, errors.New("authentication required"))
	}
	if user.IsServiceAccount {
		return stateError(inventory.StateForbidden, errors.New("service account is not permitted on Console reads"))
	}
	if s.dependencies.Authorizer == nil {
		return stateError(inventory.StateUnavailable, errors.New("console authorizer unavailable"))
	}
	allowed, err := s.dependencies.Authorizer.Check(ctx, user.Username, user.Groups, capability)
	if err != nil {
		return stateError(inventory.StateUnavailable, err)
	}
	if !allowed {
		return stateError(inventory.StateForbidden, errors.New("console capability denied"))
	}

	if filters.ClusterID == LocalClusterID {
		if exact {
			// Exact target authorization is performed after the opaque ID is
			// resolved, because the ID intentionally cannot be decoded locally.
			return nil
		}
		targets := listTargets(filters)
		if len(targets) == 0 {
			return stateError(inventory.StateForbidden, errors.New("console target list denied"))
		}
		for _, target := range targets {
			allowed, err = s.dependencies.Authorizer.CheckTarget(ctx, user.Username, user.Groups, target)
			if err != nil {
				return stateError(inventory.StateUnavailable, err)
			}
			if !allowed {
				return stateError(inventory.StateForbidden, errors.New("console target list denied"))
			}
		}
	}
	return nil
}

func (s *Service) authorizeTarget(ctx context.Context, user *auth.UserIdentity, resource inventory.Resource, exact bool) error {
	if !exact || s.dependencies.Authorizer == nil {
		return errors.New("console target authorizer unavailable")
	}
	allowed, err := s.dependencies.Authorizer.CheckTarget(ctx, user.Username, user.Groups, auth.TargetAccess{
		Verb:      "get",
		Group:     resource.Group,
		Version:   resource.Version,
		Resource:  resourceName(resource.Kind),
		Namespace: resource.Namespace,
		Name:      resource.Name,
	})
	if err != nil {
		return err
	}
	if !allowed {
		return errors.New("console target get denied")
	}
	return nil
}

func (s *Service) authorizeInventoryTarget(ctx context.Context, user *auth.UserIdentity, resource inventory.Resource, clusterID string) error {
	if clusterID == LocalClusterID {
		if err := s.authorizeTarget(ctx, user, resource, true); err != nil {
			return stateError(inventory.StateNotFound, err)
		}
		return nil
	}
	if s.dependencies.ScopeChecker == nil {
		return stateError(inventory.StateUnavailable, errors.New("remote scope checker unavailable"))
	}
	strict, ok := s.dependencies.ScopeChecker.(scope.StrictScopeChecker)
	if !ok {
		return stateError(inventory.StateUnavailable, errors.New("remote strict scope checker unavailable"))
	}
	managed, err := strict.IsManagedResourceStrict(ctx, resource.Identity())
	if err != nil {
		return stateError(inventory.StateUnavailable, err)
	}
	if !managed {
		return stateError(inventory.StateNotFound, inventory.ErrNotFound)
	}
	return nil
}

func listTargets(filters inventory.Filters) []auth.TargetAccess {
	if filters.Kind != "" {
		for _, kind := range DefaultLocalResourceKinds {
			if kind.Kind == filters.Kind && (filters.Group == "" || filters.Group == kind.Group) && (filters.Version == "" || filters.Version == kind.Version) {
				return []auth.TargetAccess{listTargetForKind(filters, kind)}
			}
		}
		return nil
	}

	targets := make([]auth.TargetAccess, 0, len(DefaultLocalResourceKinds))
	for _, kind := range DefaultLocalResourceKinds {
		if filters.Group != "" && filters.Group != kind.Group {
			continue
		}
		if filters.Version != "" && filters.Version != kind.Version {
			continue
		}
		targets = append(targets, listTargetForKind(filters, kind))
	}
	return targets
}

func listTargetForKind(filters inventory.Filters, kind LocalResourceKind) auth.TargetAccess {
	return auth.TargetAccess{
		Verb:      "list",
		Group:     kind.Group,
		Version:   kind.Version,
		Resource:  kind.Resource,
		Namespace: filters.Namespace,
		Name:      filters.Name,
	}
}

func resourceName(kind string) string {
	if kind == "" {
		return ""
	}
	known := map[string]string{
		"Pod": "pods", "Deployment": "deployments", "StatefulSet": "statefulsets",
		"DaemonSet": "daemonsets", "ReplicaSet": "replicasets", "Job": "jobs",
		"CronJob": "cronjobs", "Service": "services", "ConfigMap": "configmaps",
		"Node": "nodes", "PersistentVolume": "persistentvolumes", "Namespace": "namespaces",
	}
	if resource, ok := known[kind]; ok {
		return resource
	}
	return strings.ToLower(kind) + "s"
}

func validateResource(resource inventory.Resource, clusterID, resourceID string) error {
	if resource.ID == "" || resource.ID != resourceID || resource.ClusterID != clusterID || resource.Name == "" || resource.Kind == "" || resource.Version == "" {
		return inventory.ErrUnavailable
	}
	return nil
}

func stateForInventoryError(err error, fallback inventory.State) inventory.State {
	if errors.Is(err, inventory.ErrNotFound) {
		return inventory.StateNotFound
	}
	if errors.Is(err, inventory.ErrUnavailable) {
		return inventory.StateUnavailable
	}
	if errors.Is(err, inventory.ErrInvalidFilter) {
		return inventory.StateUnavailable
	}
	return fallback
}

func stateOnlyPolicyPage(state inventory.State) PolicyScopePage {
	return PolicyScopePage{APIVersion: PolicyScopeAPIVersion, Kind: PolicyScopeKind, State: state, Completeness: inventory.CompletenessPartial}
}

func (s *Service) readCurrentPolicies(ctx context.Context) ([]PolicyMetadata, error) {
	if s.dependencies.Policies == nil || s.threshold <= 0 {
		return nil, errors.New("policy source or required threshold unavailable")
	}
	aiValue, err := s.dependencies.Policies.Read(ctx, s.config.Namespace, s.config.PolicyRead.AIAnalysis.ConfigMapName, "approval.rego")
	if err != nil {
		return nil, fmt.Errorf("read AIAnalysis policy metadata: %w", err)
	}
	spValue, err := s.dependencies.Policies.Read(ctx, s.config.Namespace, s.config.PolicyRead.SignalProcessing.ConfigMapName, "policy.rego")
	if err != nil {
		return nil, fmt.Errorf("read SignalProcessing policy metadata: %w", err)
	}
	return []PolicyMetadata{
		{
			Type:          "aiAnalysis",
			ConfigMapName: s.config.PolicyRead.AIAnalysis.ConfigMapName,
			ConfigMapKey:  "approval.rego",
			Revision:      revision(aiValue),
			Threshold:     &Threshold{Value: s.threshold, Source: "operator-config", Kind: "rego-approval"},
		},
		{
			Type:          "signalProcessing",
			ConfigMapName: s.config.PolicyRead.SignalProcessing.ConfigMapName,
			ConfigMapKey:  "policy.rego",
			Revision:      revision(spValue),
		},
	}, nil
}

type historicalRecord struct {
	target     scope.ResourceIdentity
	evaluation HistoricalEvaluation
	size       int64
}

type historyIndex map[string][]historicalRecord

func (s *Service) readHistory(ctx context.Context, resources []inventory.Resource) (historyIndex, error) {
	if s.dependencies.History == nil {
		return nil, errors.New("history source unavailable")
	}
	targets := make(map[string]struct{}, len(resources))
	for _, resource := range resources {
		targets[historyKey(resource.Identity())] = struct{}{}
	}
	workingSet := &retainedWorkingSet{limit: DefaultInventoryWorkingSetBytes}
	if inherited, ok := ctx.Value(workingSetContextKey{}).(*retainedWorkingSet); ok && inherited != nil {
		workingSet = inherited
	}
	index := make(historyIndex, len(targets))
	if streaming, ok := s.dependencies.History.(StreamingHistorySource); ok {
		return s.readStreamingHistory(ctx, streaming, targets, index, workingSet)
	}
	return s.readLegacyHistory(ctx, targets, index, workingSet)
}

func (s *Service) readStreamingHistory(ctx context.Context, source StreamingHistorySource, targets map[string]struct{}, index historyIndex, workingSet *retainedWorkingSet) (historyIndex, error) {
	if err := source.ForEachAIAnalysis(ctx, func(record aianalysisv1.AIAnalysis) error {
		return s.addAIHistoryRecord(record, targets, index, workingSet)
	}); err != nil {
		return nil, fmt.Errorf("list AIAnalysis history: %w", err)
	}
	if err := source.ForEachSignalProcessing(ctx, func(record spv1.SignalProcessing) error {
		return s.addSignalProcessingHistoryRecord(record, targets, index, workingSet)
	}); err != nil {
		return nil, fmt.Errorf("list SignalProcessing history: %w", err)
	}
	return index, nil
}

func (s *Service) readLegacyHistory(ctx context.Context, targets map[string]struct{}, index historyIndex, workingSet *retainedWorkingSet) (historyIndex, error) {
	aiRecords, err := s.dependencies.History.ListAIAnalysis(ctx)
	if err != nil {
		return nil, fmt.Errorf("list AIAnalysis history: %w", err)
	}
	for _, record := range aiRecords {
		if err := s.addAIHistoryRecord(record, targets, index, workingSet); err != nil {
			return nil, err
		}
	}
	spRecords, err := s.dependencies.History.ListSignalProcessing(ctx)
	if err != nil {
		return nil, fmt.Errorf("list SignalProcessing history: %w", err)
	}
	for _, record := range spRecords {
		if err := s.addSignalProcessingHistoryRecord(record, targets, index, workingSet); err != nil {
			return nil, err
		}
	}
	return index, nil
}

func (s *Service) addAIHistoryRecord(record aianalysisv1.AIAnalysis, targets map[string]struct{}, index historyIndex, workingSet *retainedWorkingSet) error {
	target := record.Spec.AnalysisRequest.SignalContext.TargetResource
	identity := identityFromAI(record, target)
	evaluation := HistoricalEvaluation{Source: "AIAnalysis", ObservedAt: objectTime(record.CreationTimestamp, record.Status.CompletedAt)}
	if record.Status.Approval != nil && record.Status.Approval.ApprovalContext != nil && record.Status.Approval.ApprovalContext.PolicyEvaluation != nil {
		policy := record.Status.Approval.ApprovalContext.PolicyEvaluation
		evaluation.PolicyHash = bounded(policy.PolicyHash, 128)
		evaluation.MatchedRules = boundedStrings(policy.MatchedRules, 16, 128)
		evaluation.Decision = string(policy.Decision)
	}
	return s.addHistoricalRecord(index, targets, historicalRecord{target: identity, evaluation: evaluation}, workingSet)
}

func (s *Service) addSignalProcessingHistoryRecord(record spv1.SignalProcessing, targets map[string]struct{}, index historyIndex, workingSet *retainedWorkingSet) error {
	target := record.Spec.Signal.TargetResource
	identity := identityFromSignalProcessing(record, target)
	evaluation := HistoricalEvaluation{Source: "SignalProcessing", ObservedAt: objectTime(record.CreationTimestamp, record.Status.CompletionTime)}
	if record.Status.SignalClassification != nil {
		evaluation.PolicyHash = bounded(record.Status.SignalClassification.PolicyHash, 128)
	}
	return s.addHistoricalRecord(index, targets, historicalRecord{target: identity, evaluation: evaluation}, workingSet)
}

func (s *Service) addHistoricalRecord(index historyIndex, targets map[string]struct{}, record historicalRecord, workingSet *retainedWorkingSet) error {
	key := historyKey(record.target)
	if _, ok := targets[key]; !ok {
		return nil
	}
	size, err := workingSet.retainValue(record.evaluation)
	if err != nil {
		return err
	}
	record.size = size
	index[key] = append(index[key], record)
	bucket := index[key]
	sort.SliceStable(bucket, func(i, j int) bool {
		if bucket[i].evaluation.ObservedAt.Equal(bucket[j].evaluation.ObservedAt) {
			return bucket[i].evaluation.Source < bucket[j].evaluation.Source
		}
		return bucket[i].evaluation.ObservedAt.After(bucket[j].evaluation.ObservedAt)
	})
	if len(bucket) > s.historyLimit {
		workingSet.release(bucket[len(bucket)-1].size)
		bucket = bucket[:s.historyLimit]
	}
	index[key] = bucket
	return nil
}

func identityFromAI(record aianalysisv1.AIAnalysis, target aianalysisv1.TargetResource) scope.ResourceIdentity {
	group, version := parseAPIVersion(target.APIVersion)
	if group == "" && version == "" {
		gvk := scope.InferGVK(target.Kind)
		group, version = gvk.Group, gvk.Version
	}
	clusterID := record.Spec.ClusterID
	if clusterID == "" {
		clusterID = LocalClusterID
	}
	return scope.ResourceIdentity{ClusterID: clusterID, Group: group, Version: version, Kind: target.Kind, Namespace: target.Namespace, Name: target.Name}
}

func identityFromSignalProcessing(record spv1.SignalProcessing, target spv1.ResourceIdentifier) scope.ResourceIdentity {
	gvk := scope.InferGVK(target.Kind)
	clusterID := record.Spec.Signal.ClusterID
	if clusterID == "" {
		clusterID = LocalClusterID
	}
	return scope.ResourceIdentity{ClusterID: clusterID, Group: gvk.Group, Version: gvk.Version, Kind: target.Kind, Namespace: target.Namespace, Name: target.Name}
}

func parseAPIVersion(apiVersion string) (string, string) {
	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) == 1 {
		return "", parts[0]
	}
	return parts[0], parts[1]
}

func objectTime(created metav1.Time, completed *metav1.Time) time.Time {
	if completed != nil && !completed.IsZero() {
		return completed.Time
	}
	return created.Time
}

func historyForResource(records historyIndex, resource inventory.Resource, limit int) []HistoricalEvaluation {
	bucket := records[historyKey(resource.Identity())]
	if len(bucket) > limit {
		bucket = bucket[:limit]
	}
	result := make([]HistoricalEvaluation, 0, len(bucket))
	for _, record := range bucket {
		result = append(result, record.evaluation)
	}
	return result
}

func historyKey(identity scope.ResourceIdentity) string {
	if identity.ClusterID == "" {
		identity.ClusterID = LocalClusterID
	}
	return inventory.CanonicalResourceID(identity)
}

func (s *Service) inventoryLink(resource inventory.Resource) string {
	return strings.TrimRight(s.config.BasePath, "/") + "/inventory/managed-resources/" + url.PathEscape(resource.ID) + "?cluster_id=" + url.QueryEscape(resource.ClusterID)
}

func clonePolicies(policies []PolicyMetadata) []PolicyMetadata {
	cloned := make([]PolicyMetadata, len(policies))
	copy(cloned, policies)
	return cloned
}

func revision(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func bounded(value string, maxLength int) string {
	if len(value) > maxLength {
		return value[:maxLength]
	}
	return value
}

func boundedStrings(values []string, maxValues, maxLength int) []string {
	if len(values) > maxValues {
		values = values[:maxValues]
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, bounded(value, maxLength))
	}
	return result
}

func filterFingerprint(filters inventory.Filters) string {
	value := strings.Join([]string{filters.ClusterID, filters.Namespace, filters.Group, filters.Version, filters.Kind, filters.Name, filters.ResourceID, strconv.Itoa(filters.Limit), filters.Cursor}, "\x00")
	return fingerprint(value)
}

func resourceFingerprint(resourceID string) string {
	return fingerprint(resourceID)
}

func fingerprint(value string) string {
	digest := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func (s *Service) emitRead(ctx context.Context, user *auth.UserIdentity, eventType audit.EventType, clusterID string, state inventory.State, filterHash, resourceHash string) {
	if s.dependencies.Auditor == nil {
		return
	}
	detail := map[string]string{
		"result_state": string(state),
		"operation":    "read",
	}
	if clusterID != "" {
		detail["cluster_id"] = clusterID
	}
	if filterHash != "" {
		detail["filter_fingerprint"] = filterHash
	}
	if resourceHash != "" {
		detail["resource_fingerprint"] = resourceHash
	}
	requestID := requestIDFromContext(ctx)
	event := &audit.Event{Type: eventType, RequestID: requestID, CorrelationID: requestID, ClusterID: clusterID, Detail: detail}
	if user != nil {
		event.UserID = user.Username
	}
	s.dependencies.Auditor.Emit(ctx, event)
}

// EmitReadAudit records the final outcome of an HTTP-served Console read.
// Handler callers use this after response serialization so the audit state
// includes malformed requests and response-size failures. Direct service
// callers retain the existing deferred service audit behavior.
func (s *Service) EmitReadAudit(ctx context.Context, user *auth.UserIdentity, eventType audit.EventType, filters inventory.Filters, resourceID string, state inventory.State) {
	filterHash := ""
	resourceHash := ""
	if resourceID != "" {
		resourceHash = resourceFingerprint(resourceID)
	} else {
		filterHash = filterFingerprint(filters)
	}
	s.emitRead(ctx, user, eventType, filters.ClusterID, state, filterHash, resourceHash)
}

func requestIDFromContext(ctx context.Context) string {
	// Avoid importing the HTTP request package into the projection core. The
	// handler passes the request ID through this private context key adapter in
	// future wiring; direct service tests remain valid with an empty ID.
	if value, ok := ctx.Value(requestIDContextKey{}).(string); ok {
		return value
	}
	return ""
}

// WithRequestID carries the authenticated HTTP request ID into projection
// audit events without coupling this package to the HTTP request type.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if requestID == "" {
		return ctx
	}
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

type auditBoundaryContextKey struct{}

// WithAuditBoundary marks a request as handler-audited. Service methods skip
// their internal deferred event so the handler can emit exactly one event for
// the final HTTP outcome.
func WithAuditBoundary(ctx context.Context) context.Context {
	return context.WithValue(ctx, auditBoundaryContextKey{}, true)
}

func auditBoundaryRequested(ctx context.Context) bool {
	requested, _ := ctx.Value(auditBoundaryContextKey{}).(bool)
	return requested
}

type requestIDContextKey struct{}
