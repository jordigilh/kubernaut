// Package policyread implements the authenticated, read-only Console policy
// and managed-resource projections described by DD-AF-018.
package policyread

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	spv1 "github.com/jordigilh/kubernaut/api/signalprocessing/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/auth"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/config"
	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const (
	// LocalClusterID is the explicit identity used for AF's hub cluster.
	LocalClusterID = "local"
	// PolicyScopeAPIVersion is the stable response version for Console policy reads.
	PolicyScopeAPIVersion = "console.kubernaut.ai/v1"
	// PolicyScopeKind is the stable response kind for Console policy reads.
	PolicyScopeKind = "PolicyScopeList"
	// DefaultHistoryLimit bounds historical records joined to one resource.
	DefaultHistoryLimit = 20
)

// Config contains startup-only policy projection settings. Policy metadata is
// read from the operator-generated service ConfigMaps at request time; AF does
// not maintain a second operator-owned policy metadata source.
type Config struct {
	Namespace string
	// PolicyRead is retained so older callers and generated operator config can
	// be decoded during migration. It is ignored; CurrentPolicySource reads the
	// authoritative runtime ConfigMaps instead.
	PolicyRead   config.PolicyReadConfig
	BasePath     string
	HistoryLimit int
}

// Dependencies are the narrow source and authorization seams used by Service.
type Dependencies struct {
	LocalInventory  InventoryReader
	RemoteInventory InventoryReader
	Policies        PolicySource
	CurrentPolicies CurrentPolicySource
	History         HistorySource
	Authorizer      Authorizer
	ScopeChecker    scope.ScopeChecker
	ReadGuard       *ReadGuard
	Auditor         audit.Emitter
	Logger          logr.Logger
}

// InventoryReader is the sanitized inventory contract consumed by AF. It is
// intentionally narrower than any Kubernetes or MCP client.
type InventoryReader interface {
	List(ctx context.Context, filters inventory.Filters) (inventory.Page, error)
	Get(ctx context.Context, clusterID, resourceID string) (inventory.Resource, error)
}

// PolicySource returns the bytes of one explicitly configured policy key. The
// service hashes the bytes and never places them in a response or audit event.
type PolicySource interface {
	Read(ctx context.Context, namespace, name, key string) ([]byte, error)
}

// CurrentPolicySource reads the current policy projection metadata from the
// operator-generated AIAnalysis and SignalProcessing runtime ConfigMaps. The
// implementation must read through an uncached source for each operation and
// must never return policy source text.
type CurrentPolicySource interface {
	ReadCurrentPolicies(ctx context.Context, namespace string) ([]PolicyMetadata, error)
}

// HistorySource returns the existing CRD records used for bounded, exact-target
// historical projection. Implementations must not fetch arbitrary objects.
type HistorySource interface {
	ListAIAnalysis(ctx context.Context) ([]aianalysisv1.AIAnalysis, error)
	ListSignalProcessing(ctx context.Context) ([]spv1.SignalProcessing, error)
}

// StreamingHistorySource is the bounded history seam used by the production
// Kubernetes source. Each callback receives one decoded record and may stop
// the traversal with an error; implementations must page and discard backend
// lists rather than retain the complete source collection.
type StreamingHistorySource interface {
	ForEachAIAnalysis(ctx context.Context, visit func(aianalysisv1.AIAnalysis) error) error
	ForEachSignalProcessing(ctx context.Context, visit func(spv1.SignalProcessing) error) error
}

// Authorizer combines named capability SARs with exact local target SARs.
// auth.SARChecker implements this interface after the target-SAR extension.
type Authorizer interface {
	Check(ctx context.Context, user string, groups []string, capability string) (bool, error)
	CheckTarget(ctx context.Context, user string, groups []string, target auth.TargetAccess) (bool, error)
}

// PolicyScopePage is the typed, sanitized policy/scope list response.
type PolicyScopePage struct {
	APIVersion   string                      `json:"apiVersion"`
	Kind         string                      `json:"kind"`
	State        inventory.State             `json:"state"`
	ObservedAt   time.Time                   `json:"observedAt"`
	Freshness    inventory.Freshness         `json:"freshness"`
	Completeness inventory.CompletenessState `json:"completeness"`
	Items        []PolicyScopeItem           `json:"items,omitempty"`
	NextCursor   string                      `json:"nextCursor,omitempty"`
}

// PolicyScopeItem joins current operator policy metadata with bounded history
// for one canonical managed-resource identity.
type PolicyScopeItem struct {
	ID                    string                 `json:"id"`
	Resource              inventory.Resource     `json:"resource"`
	CurrentPolicies       []PolicyMetadata       `json:"currentPolicies"`
	HistoricalEvaluations []HistoricalEvaluation `json:"historicalEvaluations,omitempty"`
	InventoryLink         string                 `json:"inventoryLink"`
}

// PolicyMetadata is identity/revision metadata only; source text is excluded.
type PolicyMetadata struct {
	Type          string     `json:"type"`
	ConfigMapName string     `json:"configMapName"`
	ConfigMapKey  string     `json:"configMapKey"`
	Revision      string     `json:"revision"`
	Threshold     *Threshold `json:"threshold,omitempty"`
}

// Threshold is the operator-supplied AIAnalysis approval threshold. It is not
// a claim about every rule in custom Rego.
type Threshold struct {
	Value  float64 `json:"value"`
	Source string  `json:"source"`
	Kind   string  `json:"kind"`
}

// HistoricalEvaluation contains only fields already persisted by the source CRD.
type HistoricalEvaluation struct {
	Source       string    `json:"source"`
	ObservedAt   time.Time `json:"observedAt"`
	PolicyHash   string    `json:"policyHash,omitempty"`
	MatchedRules []string  `json:"matchedRules,omitempty"`
	Decision     string    `json:"decision,omitempty"`
}

// StateError carries a safe wire state while retaining an internal cause for
// tests and handler classification. Handlers must serialize only State.
type StateError struct {
	State inventory.State
	Err   error
}

func (e *StateError) Error() string {
	if e == nil || e.Err == nil {
		return string(e.State)
	}
	return fmt.Sprintf("%s: %v", e.State, e.Err)
}

func (e *StateError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// StateOf returns a bounded read state for handler and test classification.
func StateOf(err error) inventory.State {
	var stateErr *StateError
	if errors.As(err, &stateErr) && stateErr != nil {
		return stateErr.State
	}
	return inventory.StateUnavailable
}

func stateError(state inventory.State, err error) error {
	if err == nil {
		err = errors.New(string(state))
	}
	return &StateError{State: state, Err: err}
}
