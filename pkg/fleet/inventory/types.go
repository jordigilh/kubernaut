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

// Package inventory defines the bounded, sanitized inventory contract shared
// by the FMC and ACM fleet control-plane projections.
package inventory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

const (
	// DefaultLimit is the bounded page size used when a caller omits limit.
	DefaultLimit = 50
	// MaxLimit prevents a control-plane read from becoming an unbounded list.
	MaxLimit = 100
	// MaxCursorLength bounds opaque continuation tokens at the transport edge.
	MaxCursorLength = 512
	// MaxFilterLength bounds every user-controlled identity filter.
	MaxFilterLength = 253
)

// State describes the result state of a control-plane inventory read.
type State string

const (
	StateSuccess     State = "success"
	StateEmpty       State = "empty"
	StatePartial     State = "partial"
	StateStale       State = "stale"
	StateUnavailable State = "unavailable"
	StateForbidden   State = "forbidden"
	StateNotFound    State = "not_found"
)

// FreshnessState describes the age of a page relative to the configured
// freshness budget.
type FreshnessState string

const (
	FreshnessFresh FreshnessState = "fresh"
	FreshnessStale FreshnessState = "stale"
)

// CompletenessState describes whether the control plane returned a complete
// page rather than a truncated or failed traversal.
type CompletenessState string

const (
	CompletenessComplete CompletenessState = "complete"
	CompletenessPartial  CompletenessState = "partial"
)

// LabelState is the bounded representation of one managed-scope label.
// Invalid values are retained as an internal state but never treated as true.
type LabelState string

const (
	LabelTrue    LabelState = "true"
	LabelFalse   LabelState = "false"
	LabelAbsent  LabelState = "absent"
	LabelInvalid LabelState = "invalid"
)

// ScopeSource identifies the label that determined the effective managed
// state. It intentionally contains no raw labels or arbitrary annotations.
type ScopeSource string

const (
	ScopeSourceResourceLabel  ScopeSource = "resource-label"
	ScopeSourceNamespaceLabel ScopeSource = "namespace-label"
	ScopeSourceNone           ScopeSource = "none"
	ScopeSourceUnavailable    ScopeSource = "unavailable"
)

// Freshness is the typed freshness envelope returned with every complete
// inventory page.
type Freshness struct {
	State FreshnessState `json:"state"`
	AsOf  time.Time      `json:"as_of"`
}

// Scope is the sanitized effective managed-scope projection for one resource.
type Scope struct {
	Included       bool        `json:"included"`
	ResourceLabel  LabelState  `json:"resource_label"`
	NamespaceLabel LabelState  `json:"namespace_label"`
	Source         ScopeSource `json:"source"`
	ClusterScoped  bool        `json:"cluster_scoped"`
}

// Resource is the canonical inventory identity shared by FMC and ACM. It
// deliberately has no arbitrary labels, annotations, object data, or status.
type Resource struct {
	ID         string    `json:"id"`
	ClusterID  string    `json:"cluster_id"`
	Group      string    `json:"group"`
	Version    string    `json:"version"`
	Kind       string    `json:"kind"`
	Namespace  string    `json:"namespace,omitempty"`
	Name       string    `json:"name"`
	Scope      Scope     `json:"scope"`
	ObservedAt time.Time `json:"-"`
}

// Identity converts the canonical resource into the shared scope-checking
// identity used by AF and the fleet control planes.
func (r Resource) Identity() scope.ResourceIdentity {
	return scope.ResourceIdentity{
		ClusterID: r.ClusterID,
		Group:     r.Group,
		Version:   r.Version,
		Kind:      r.Kind,
		Namespace: r.Namespace,
		Name:      r.Name,
	}
}

// Page is the canonical list response. Non-success states must contain no
// resource records; callers can safely serialize the state envelope only.
type Page struct {
	State        State             `json:"state"`
	ObservedAt   time.Time         `json:"observed_at"`
	Freshness    Freshness         `json:"freshness"`
	Completeness CompletenessState `json:"completeness"`
	Items        []Resource        `json:"items,omitempty"`
	NextCursor   string            `json:"next_cursor,omitempty"`
}

// Filters is the only filter surface accepted by the canonical inventory
// readers. ClusterID is required so a remote read cannot silently widen into
// a cross-cluster query.
type Filters struct {
	ClusterID  string
	Namespace  string
	Group      string
	Version    string
	Kind       string
	Name       string
	ResourceID string
	Limit      int
	Cursor     string
}

// Reader is the narrow fleet control-plane contract consumed by AF Console
// reads. Implementations must return only the canonical sanitized model.
type Reader interface {
	List(ctx context.Context, filters Filters) (Page, error)
	Get(ctx context.Context, clusterID, resourceID string) (Resource, error)
}

var (
	// ErrInvalidFilter identifies caller-controlled inventory input that would
	// be ambiguous, malformed, or wider than the contract permits.
	ErrInvalidFilter = errors.New("invalid inventory filter")
	// ErrUnavailable identifies a control-plane read that cannot provide a
	// complete result. The caller must not fall back to a different cluster.
	ErrUnavailable = errors.New("inventory unavailable")
	// ErrNotFound identifies an exact resource ID that is not present.
	ErrNotFound = errors.New("inventory resource not found")
)

var filterValuePattern = regexp.MustCompile(`^[A-Za-z0-9._:/-]+$`)
var cursorPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)
var resourceIDPattern = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)

// Normalize validates filters and applies the bounded default page size.
func (f Filters) Normalize() (Filters, error) {
	if f.ClusterID == "" {
		return Filters{}, fmt.Errorf("%w: cluster_id is required", ErrInvalidFilter)
	}
	if f.Limit == 0 {
		f.Limit = DefaultLimit
	}
	if err := validateLimit(f.Limit); err != nil {
		return Filters{}, err
	}
	if err := validateResourceIDCombination(f); err != nil {
		return Filters{}, err
	}
	for field, value := range filterValues(f) {
		if err := validateFilterValue(field, value); err != nil {
			return Filters{}, err
		}
	}
	if err := validateCursor(f.Cursor); err != nil {
		return Filters{}, err
	}
	return f, nil
}

func validateLimit(limit int) error {
	if limit < 1 || limit > MaxLimit {
		return fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidFilter, MaxLimit)
	}
	return nil
}

func validateResourceIDCombination(filters Filters) error {
	if filters.ResourceID == "" {
		return nil
	}
	if filters.Namespace != "" || filters.Group != "" || filters.Version != "" || filters.Kind != "" || filters.Name != "" || filters.Cursor != "" {
		return fmt.Errorf("%w: resource_id cannot be combined with identity filters or cursor", ErrInvalidFilter)
	}
	return nil
}

func filterValues(filters Filters) map[string]string {
	return map[string]string{
		"cluster_id":  filters.ClusterID,
		"namespace":   filters.Namespace,
		"group":       filters.Group,
		"version":     filters.Version,
		"kind":        filters.Kind,
		"name":        filters.Name,
		"resource_id": filters.ResourceID,
	}
}

func validateCursor(cursor string) error {
	if cursor != "" && (len(cursor) > MaxCursorLength || !cursorPattern.MatchString(cursor)) {
		return fmt.Errorf("%w: cursor is not a valid opaque token", ErrInvalidFilter)
	}
	return nil
}

func validateFilterValue(field, value string) error {
	if value == "" {
		return nil
	}
	if len(value) > MaxFilterLength || !filterValuePattern.MatchString(value) {
		return fmt.Errorf("%w: %s contains unsupported characters or is too long", ErrInvalidFilter, field)
	}
	return nil
}

// ValidateResourceID validates the opaque ID accepted by exact inventory
// reads. Resource IDs never contain path separators or arbitrary query text.
func ValidateResourceID(resourceID string) error {
	if resourceID == "" || len(resourceID) > MaxFilterLength || !resourceIDPattern.MatchString(resourceID) {
		return fmt.Errorf("%w: resource_id is not a valid opaque ID", ErrInvalidFilter)
	}
	return nil
}

// Validate checks the safety invariant for a decoded page. It rejects any
// response that would leak records alongside an error/state-only result.
func (p Page) Validate() error {
	switch p.State {
	case StateSuccess:
		if p.Completeness != CompletenessComplete {
			return fmt.Errorf("%w: successful page is incomplete", ErrUnavailable)
		}
	case StateEmpty:
		if len(p.Items) != 0 {
			return fmt.Errorf("%w: empty page contains items", ErrUnavailable)
		}
	case StatePartial, StateStale, StateUnavailable, StateForbidden, StateNotFound:
		if len(p.Items) != 0 || p.NextCursor != "" {
			return fmt.Errorf("%w: state-only page contains records or cursor", ErrUnavailable)
		}
	default:
		return fmt.Errorf("%w: unsupported page state %q", ErrUnavailable, p.State)
	}
	return nil
}

// StateOnly returns a safe page envelope for backend failures.
func StateOnly(state State) Page {
	return Page{
		State:        state,
		Completeness: CompletenessPartial,
	}
}

// CanonicalResourceID returns an opaque, stable ID derived from the complete
// resource identity. It is intentionally one-way so IDs do not expose target
// names or namespaces when used in audit details or links.
func CanonicalResourceID(identity scope.ResourceIdentity) string {
	canonical := strings.Join([]string{
		identity.ClusterID,
		identity.Group,
		identity.Version,
		identity.Kind,
		identity.Namespace,
		identity.Name,
	}, "\x00")
	digest := sha256.Sum256([]byte(canonical))
	return "r-" + hex.EncodeToString(digest[:])
}

// SortResources orders resources deterministically for cursor-compatible page
// construction and stable Console output.
func SortResources(resources []Resource) {
	sort.Slice(resources, func(i, j int) bool {
		left, right := resources[i], resources[j]
		for _, pair := range [][2]string{
			{left.ClusterID, right.ClusterID},
			{left.Group + "/" + left.Version, right.Group + "/" + right.Version},
			{left.Kind, right.Kind},
			{left.Namespace, right.Namespace},
			{left.Name, right.Name},
			{left.ID, right.ID},
		} {
			if pair[0] == pair[1] {
				continue
			}
			return pair[0] < pair[1]
		}
		return false
	})
}
