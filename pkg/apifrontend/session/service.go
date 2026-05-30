package session

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut/api/investigationsession/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/audit"
	"github.com/jordigilh/kubernaut/pkg/apifrontend/security"
)

// validCRDName matches a DNS label (lowercase alphanumeric and '-', must
// start/end with alphanumeric, max 253 chars). Dots are intentionally excluded
// to keep session IDs as simple labels rather than full DNS subdomains.
var validCRDName = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,251}[a-z0-9])?$`)

// Label keys used on InvestigationSession CRDs.
const (
	LabelUser      = "kubernaut.ai/user"
	LabelRRName    = "kubernaut.ai/rr-name"
	LabelPhase     = "kubernaut.ai/phase"
	LabelManagedBy = "app.kubernetes.io/managed-by"
)

// FieldIndexRRName is the field index path for InvestigationSession's
// spec.remediationRequestRef.name, used for MatchingFields queries.
const FieldIndexRRName = "spec.remediationRequestRef.name"

// RegisterFieldIndexes registers required field indexes on the given manager's
// cache for InvestigationSession lookups by spec.remediationRequestRef.name.
func RegisterFieldIndexes(ctx context.Context, indexer client.FieldIndexer) error {
	return indexer.IndexField(ctx, &v1alpha1.InvestigationSession{}, FieldIndexRRName,
		func(obj client.Object) []string {
			is := obj.(*v1alpha1.InvestigationSession)
			if is.Spec.RemediationRequestRef.Name == "" {
				return nil
			}
			return []string{is.Spec.RemediationRequestRef.Name}
		},
	)
}

// StateKeyCreateConfig is the session state key used to pass CRD creation
// parameters from the caller into the Create method. The value must be a
// *CreateConfig. The key uses the "temp:" prefix so ADK strips it after
// the invocation completes.
const StateKeyCreateConfig = "temp:af_create_config"

// CreateConfig holds the parameters needed to create an InvestigationSession
// CRD alongside the ADK in-memory session.
type CreateConfig struct {
	OwnerRef       metav1.OwnerReference
	A2ATaskID      string
	UserIdentity   v1alpha1.SessionUser
	JoinMode       v1alpha1.SessionJoinMode
	RemediationRef v1alpha1.ObjectRef
}

// CRDSessionService wraps ADK's InMemoryService as a delegate, syncing
// InvestigationSession CRD metadata on each session lifecycle operation.
// Session objects returned by Create/Get/List are the delegate's native types,
// which satisfies the InMemoryService.AppendEvent type assertion on *session.
type CRDSessionService struct {
	delegate       adksession.Service
	client         client.Client
	apiReader      client.Reader
	scheme         *runtime.Scheme
	namespace      string
	logger         logr.Logger
	auditor        audit.Emitter
	sessionsActive *prometheus.GaugeVec

	mu             sync.RWMutex
	crdIndex       map[string]string        // sessionID -> CRD name
	pendingConfigs map[string]*CreateConfig  // sessionID -> deferred CreateConfig; cleaned on Delete(). Since #1293, MaterializeCRD is deprecated so entries persist until session deletion (bounded by active session count).
}

// NewCRDSessionService creates a new CRDSessionService. The delegate should
// typically be adksession.InMemoryService(). The auditor may be nil to disable
// audit emission (e.g. in tests).
func NewCRDSessionService(delegate adksession.Service, c client.Client, scheme *runtime.Scheme, ns string, opts ...Option) *CRDSessionService {
	svc := &CRDSessionService{
		delegate:       delegate,
		client:         c,
		scheme:         scheme,
		namespace:      ns,
		logger:         logr.Discard(),
		crdIndex:       make(map[string]string),
		pendingConfigs: make(map[string]*CreateConfig),
	}
	for _, o := range opts {
		o(svc)
	}
	return svc
}

// Option configures optional dependencies on CRDSessionService.
type Option func(*CRDSessionService)

// WithLogger injects a logr.Logger for structured diagnostics.
func WithLogger(l logr.Logger) Option {
	return func(s *CRDSessionService) {
		if l.GetSink() != nil {
			s.logger = l
		}
	}
}

// WithAuditor injects an audit.Emitter for FedRAMP AU-2/AU-12 compliance.
func WithAuditor(e audit.Emitter) Option {
	return func(s *CRDSessionService) { s.auditor = e }
}

// WithSessionsActive injects the af_sessions_active gauge for observability.
func WithSessionsActive(g *prometheus.GaugeVec) Option {
	return func(s *CRDSessionService) { s.sessionsActive = g }
}

// WithAPIReader injects a cache-bypassing reader (DD-STATUS-001 pattern from
// kubernaut). When set, UpdatePhase uses it for the initial Get to avoid
// stale-cache reads that break optimistic locking.
func WithAPIReader(r client.Reader) Option {
	return func(s *CRDSessionService) { s.apiReader = r }
}

// getReader returns the cache-bypassing apiReader if available, falling back
// to the cached client. This mirrors kubernaut's DD-STATUS-001 pattern where
// all read-before-write operations use an uncached reader.
func (s *CRDSessionService) getReader() client.Reader {
	if s.apiReader != nil {
		return s.apiReader
	}
	return s.client
}

// Create delegates session creation to the in-memory service and stores the
// CRD creation config for later materialization. No K8s CRD is created until
// MaterializeCRD is called (typically after kubernaut_remediate produces a real RR).
// The CRD creation config is read from req.State[StateKeyCreateConfig].
func (s *CRDSessionService) Create(ctx context.Context, req *adksession.CreateRequest) (*adksession.CreateResponse, error) {
	var cfg *CreateConfig
	if req.State != nil {
		if v, ok := req.State[StateKeyCreateConfig]; ok {
			cfg, ok = v.(*CreateConfig)
			if !ok {
				return nil, fmt.Errorf("invalid create config type: %T", v)
			}
		}
	}

	crdName := req.SessionID
	if crdName == "" {
		crdName = fmt.Sprintf("isess-%d", time.Now().UnixNano())
	}
	if !validCRDName.MatchString(crdName) {
		return nil, fmt.Errorf("invalid session ID %q: must be a valid RFC 1123 subdomain", crdName)
	}

	// CRD creation is deferred until MaterializeCRD, which is called by the
	// kubernaut_remediate after-callback once a real RemediationRequest exists.
	// A2A sessions exist to remediate; no CRD is created for sessions that
	// never produce an RR (incomplete/error/misuse).
	resp, err := s.delegate.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("delegate create: %w", err)
	}

	s.mu.Lock()
	s.crdIndex[resp.Session.ID()] = crdName
	if cfg != nil {
		s.pendingConfigs[resp.Session.ID()] = cfg
	}
	s.mu.Unlock()

	s.logger.Info("session created (CRD deferred until kubernaut_remediate)",
		"session_id", resp.Session.ID(),
		"crd_name", crdName,
		"user", req.UserID,
	)
	auditDetail := map[string]string{
		"session_id": resp.Session.ID(),
		"crd_name":   crdName,
		"phase":      string(v1alpha1.SessionPhaseActive),
	}
	if cfg != nil {
		auditDetail["a2a_task_id"] = cfg.A2ATaskID
		auditDetail["join_mode"] = string(cfg.JoinMode)
		auditDetail["user_identity"] = cfg.UserIdentity.Username
	}
	s.emitAudit(ctx, audit.EventSessionCreated, req.UserID, auditDetail)
	s.incSessionGauge(string(v1alpha1.SessionPhaseActive))
	return resp, nil
}

// Get delegates to the in-memory service. Sessions are invalidated on pod
// restart since the in-memory delegate is not hydrated from CRDs. CRD
// reconciliation will transition orphaned sessions to Disconnected via the
// TTL controller. Full session hydration is deferred to PR7.
func (s *CRDSessionService) Get(ctx context.Context, req *adksession.GetRequest) (*adksession.GetResponse, error) {
	return s.delegate.Get(ctx, req)
}

// List delegates to the in-memory service.
func (s *CRDSessionService) List(ctx context.Context, req *adksession.ListRequest) (*adksession.ListResponse, error) {
	return s.delegate.List(ctx, req)
}

// Delete removes the InvestigationSession CRD and delegates deletion to the
// in-memory service. CRD deletion is attempted even if the delegate has no
// state (orphan cleanup after restart).
func (s *CRDSessionService) Delete(ctx context.Context, req *adksession.DeleteRequest) error {
	s.mu.RLock()
	crdName, hasCRD := s.crdIndex[req.SessionID]
	s.mu.RUnlock()

	if !hasCRD {
		crdName = req.SessionID
	}

	nn := types.NamespacedName{Name: crdName, Namespace: s.namespace}
	reader := s.getReader()
	var existing v1alpha1.InvestigationSession
	phase := string(v1alpha1.SessionPhaseActive) // fallback
	if err := reader.Get(ctx, nn, &existing); err == nil {
		phase = string(existing.Status.Phase)
	}

	crd := &v1alpha1.InvestigationSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: s.namespace,
		},
	}
	if err := s.client.Delete(ctx, crd); err != nil {
		s.logger.V(0).Info("CRD delete failed",
			"session_id", req.SessionID,
			"crd_name", crdName,
			"error", security.RedactError(err),
		)
	}

	s.mu.Lock()
	delete(s.crdIndex, req.SessionID)
	delete(s.pendingConfigs, req.SessionID)
	s.mu.Unlock()

	s.emitAudit(ctx, audit.EventSessionDeleted, req.UserID, map[string]string{
		"session_id": req.SessionID,
		"crd_name":   crdName,
	})
	s.decSessionGauge(phase)
	return s.delegate.Delete(ctx, req)
}

// AppendEvent trims large FunctionResponse parts, then delegates to the
// in-memory service for event storage and temp: key stripping. After
// successful delegation, updates the CRD status timestamp.
func (s *CRDSessionService) AppendEvent(ctx context.Context, sess adksession.Session, event *adksession.Event) error {
	trimEventFunctionResponses(event)

	if err := s.delegate.AppendEvent(ctx, sess, event); err != nil {
		return err
	}

	// Best-effort CRD status update (event is stored even if this fails)
	s.mu.RLock()
	crdName, ok := s.crdIndex[sess.ID()]
	s.mu.RUnlock()

	if ok {
		reader := s.getReader()
		var crd v1alpha1.InvestigationSession
		if err := reader.Get(ctx, types.NamespacedName{Name: crdName, Namespace: s.namespace}, &crd); err == nil {
			_ = s.client.Status().Update(ctx, &crd)
		}
	}

	return nil
}

// GetSessionPhase returns the CRD phase for a session by reading the
// InvestigationSession CRD from the API server.
func (s *CRDSessionService) GetSessionPhase(ctx context.Context, sessionID string) (v1alpha1.SessionPhase, error) {
	s.mu.RLock()
	crdName, ok := s.crdIndex[sessionID]
	s.mu.RUnlock()

	if !ok {
		crdName = sessionID
	}

	reader := s.getReader()
	var crd v1alpha1.InvestigationSession
	if err := reader.Get(ctx, types.NamespacedName{Name: crdName, Namespace: s.namespace}, &crd); err != nil {
		return "", fmt.Errorf("get session phase: %w", err)
	}
	return crd.Status.Phase, nil
}

var _ adksession.Service = (*CRDSessionService)(nil)

func (s *CRDSessionService) emitAudit(ctx context.Context, eventType audit.EventType, userID string, detail map[string]string) {
	if s.auditor == nil {
		return
	}
	s.auditor.Emit(ctx, &audit.Event{
		Type:   eventType,
		UserID: userID,
		Detail: detail,
	})
}

func (s *CRDSessionService) incSessionGauge(phase string) {
	if s.sessionsActive != nil {
		s.sessionsActive.WithLabelValues(phase).Inc()
	}
}

func (s *CRDSessionService) decSessionGauge(phase string) {
	if s.sessionsActive != nil {
		s.sessionsActive.WithLabelValues(phase).Dec()
	}
}

// PruneTerminalEntries removes crdIndex entries for sessions whose CRD is in
// a terminal phase. Call periodically (e.g. from the TTL reconciler) to bound
// map growth.
func (s *CRDSessionService) PruneTerminalEntries(ctx context.Context) int {
	s.mu.RLock()
	snapshot := make(map[string]string, len(s.crdIndex))
	for k, v := range s.crdIndex {
		snapshot[k] = v
	}
	s.mu.RUnlock()

	var pruned int
	for sessionID, crdName := range snapshot {
		var crd v1alpha1.InvestigationSession
		err := s.client.Get(ctx, types.NamespacedName{Name: crdName, Namespace: s.namespace}, &crd)
		if err != nil && !apierrors.IsNotFound(err) {
			continue
		}
		if err != nil || IsTerminal(crd.Status.Phase) {
			s.mu.Lock()
			delete(s.crdIndex, sessionID)
			s.mu.Unlock()
			pruned++
		}
	}
	if pruned > 0 {
		s.logger.Info("pruned terminal crdIndex entries", "count", pruned)
	}
	return pruned
}

// Deprecated: MaterializeCRD is the legacy path that created the IS CRD on
// kubernaut_remediate. Since #1293, IS CRD creation is deferred to explicit
// kubernaut_takeover via InitializeSessionByRR. This method is retained for
// backward compatibility with any remaining callers but is not invoked in
// the current production flow.
//
// MaterializeCRD creates the K8s InvestigationSession CRD for a previously
// deferred session. Idempotent: returns nil if the CRD was already materialized.
func (s *CRDSessionService) MaterializeCRD(ctx context.Context, sessionID string, rrRef v1alpha1.ObjectRef) error {
	s.mu.Lock()
	cfg, isPending := s.pendingConfigs[sessionID]
	crdName, hasCRD := s.crdIndex[sessionID]
	if !isPending {
		s.mu.Unlock()
		if hasCRD {
			return nil // already materialized (idempotent)
		}
		return fmt.Errorf("session %q not found in pending configs", sessionID)
	}
	delete(s.pendingConfigs, sessionID)
	s.mu.Unlock()

	crd := &v1alpha1.InvestigationSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: s.namespace,
			Labels: map[string]string{
				LabelManagedBy: "kubernaut-apifrontend",
			},
		},
	}

	if cfg != nil {
		if cfg.OwnerRef.Name != "" {
			crd.OwnerReferences = []metav1.OwnerReference{cfg.OwnerRef}
		}
		crd.Labels[LabelUser] = sanitizeLabelValue(cfg.UserIdentity.Username)
		cfg.RemediationRef = rrRef
		crd.Labels[LabelRRName] = sanitizeLabelValue(rrRef.Name)
		crd.Spec = v1alpha1.InvestigationSessionSpec{
			RemediationRequestRef: rrRef,
			A2ATaskID:             cfg.A2ATaskID,
			UserIdentity:          cfg.UserIdentity,
			JoinMode:              cfg.JoinMode,
		}
	}

	// BR-INTERACTIVE-004: reject creation when an active IS CRD for the same
	// RR already exists with a different user (single-driver enforcement).
	// The Lease is authoritative; this is a best-effort guard (TOCTOU acceptable).
	// Same-user re-creation is allowed for reconnection scenarios.
	if rrRef.Name != "" && cfg != nil {
		var existingList v1alpha1.InvestigationSessionList
		if listErr := s.client.List(ctx, &existingList,
			client.InNamespace(s.namespace),
			client.MatchingFields{FieldIndexRRName: rrRef.Name},
		); listErr == nil {
			for i := range existingList.Items {
				existing := &existingList.Items[i]
				if !IsTerminal(existing.Status.Phase) &&
					existing.Name != crdName &&
					existing.Spec.UserIdentity.Username != cfg.UserIdentity.Username {
					s.mu.Lock()
					s.pendingConfigs[sessionID] = cfg
					s.mu.Unlock()
					return fmt.Errorf("session_active: an active investigation session already exists for RR %s/%s (held by %s)",
						rrRef.Namespace, rrRef.Name, existing.Spec.UserIdentity.Username)
				}
			}
		}
	}

	if err := s.client.Create(ctx, crd); err != nil {
		s.mu.Lock()
		s.pendingConfigs[sessionID] = cfg
		s.mu.Unlock()
		return fmt.Errorf("create InvestigationSession CRD: %w", err)
	}

	crd.Status.Phase = v1alpha1.SessionPhaseActive
	crd.Status.Message = "session materialized"
	if err := s.client.Status().Update(ctx, crd); err != nil {
		s.logger.Error(err, "failed to set initial phase to Active after CRD creation",
			"session_id", sessionID, "crd_name", crdName)
	}

	s.logger.Info("CRD materialized",
		"session_id", sessionID,
		"crd_name", crdName,
		"rr_ref", rrRef.Name,
	)
	return nil
}

// FinalizeSessionByRR looks up the active InvestigationSession for a given RR
// and transitions it to the specified terminal phase. Best-effort: returns nil
// if no active session exists. Enables MCP complete/cancel tools to update the
// IS CRD (BR-INTERACTIVE-010 SC-1).
func (s *CRDSessionService) FinalizeSessionByRR(ctx context.Context, rrNamespace, rrName string, phase v1alpha1.SessionPhase) error {
	var list v1alpha1.InvestigationSessionList
	if err := s.client.List(ctx, &list,
		client.InNamespace(s.namespace),
		client.MatchingFields{FieldIndexRRName: rrName},
	); err != nil {
		return fmt.Errorf("list sessions for RR %s/%s: %w", rrNamespace, rrName, err)
	}

	for i := range list.Items {
		is := &list.Items[i]
		if !IsTerminal(is.Status.Phase) {
			msg := fmt.Sprintf("user action: %s", string(phase))
			userID := is.Spec.UserIdentity.Username
			return s.UpdatePhase(ctx, is.Name, phase, msg, userID)
		}
	}
	return nil
}

// InitializeSessionByRR creates an InvestigationSession CRD for a takeover
// action. Unlike MaterializeCRD (which uses deferred A2A state), this builds
// the CRD directly from MCP bridge context. Idempotent: returns nil if an
// active IS CRD for this RR already exists for the same user.
//
// BR-INTERACTIVE-010 SC-1.3: enables dynamic takeover signal via IS CRD.
// BR-INTERACTIVE-004: single-driver guard rejects a different user's takeover.
//
// Concurrency: the List→Create sequence has a TOCTOU window. This is accepted
// because the Lease is authoritative for single-driver enforcement (same
// trade-off as MaterializeCRD). AlreadyExists on Create provides idempotency
// at the K8s layer for concurrent calls with the same CRD name.
func (s *CRDSessionService) InitializeSessionByRR(ctx context.Context, rrNamespace, rrName, kaSessionID, username string, groups []string) error {
	if kaSessionID == "" {
		return fmt.Errorf("kaSessionID is required for IS CRD initialization")
	}

	rrRef := v1alpha1.ObjectRef{Namespace: rrNamespace, Name: rrName}

	var existingList v1alpha1.InvestigationSessionList
	if err := s.client.List(ctx, &existingList,
		client.InNamespace(s.namespace),
		client.MatchingFields{FieldIndexRRName: rrName},
	); err != nil {
		return fmt.Errorf("list sessions for RR %s/%s: %w", rrNamespace, rrName, err)
	}
	for i := range existingList.Items {
		existing := &existingList.Items[i]
		if !IsTerminal(existing.Status.Phase) {
			if existing.Spec.UserIdentity.Username == username {
				return nil
			}
			return fmt.Errorf("session_active: an active investigation session already exists for RR %s/%s",
				rrNamespace, rrName)
		}
	}

	crdName := kaSessionID
	if !validCRDName.MatchString(crdName) {
		crdName = fmt.Sprintf("isess-%d", time.Now().UnixNano())
	}

	crd := &v1alpha1.InvestigationSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: s.namespace,
			Labels: map[string]string{
				LabelManagedBy: "kubernaut-apifrontend",
				LabelUser:      sanitizeLabelValue(username),
				LabelRRName:    sanitizeLabelValue(rrName),
			},
		},
		Spec: v1alpha1.InvestigationSessionSpec{
			RemediationRequestRef: rrRef,
			A2ATaskID:             kaSessionID,
			UserIdentity: v1alpha1.SessionUser{
				Username: username,
				Groups:   groups,
			},
			JoinMode: v1alpha1.SessionJoinModeTakeover,
		},
	}

	if rrNamespace == s.namespace {
		setRROwnerReference(ctx, s.client, s.logger, crd, rrNamespace, rrName)
	}

	if err := s.client.Create(ctx, crd); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return fmt.Errorf("create IS CRD for takeover: %w", err)
	}

	crd.Status.Phase = v1alpha1.SessionPhaseActive
	crd.Status.Message = "session materialized"
	if err := s.client.Status().Update(ctx, crd); err != nil {
		s.logger.Error(err, "failed to set initial phase to Active after IS CRD creation",
			"session_id", kaSessionID, "crd_name", crdName)
	}

	s.mu.Lock()
	s.crdIndex[kaSessionID] = crdName
	s.mu.Unlock()

	s.incSessionGauge(string(v1alpha1.SessionPhaseActive))

	if s.auditor != nil {
		s.auditor.Emit(ctx, &audit.Event{
			Type:   audit.EventSessionCreated,
			UserID: username,
			Detail: map[string]string{
				"session_id":    kaSessionID,
				"join_mode":     string(v1alpha1.SessionJoinModeTakeover),
				"user_identity": username,
				"rr_ref":        rrNamespace + "/" + rrName,
			},
		})
	}

	s.logger.Info("IS CRD initialized for takeover",
		"crd_name", crdName,
		"rr_ref", rrName,
		"user", username,
	)
	return nil
}

// IsMaterialized returns true if the session's CRD has been created in K8s.
func (s *CRDSessionService) IsMaterialized(sessionID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, hasCRD := s.crdIndex[sessionID]
	_, isPending := s.pendingConfigs[sessionID]
	return hasCRD && !isPending
}

var rrGVK = schema.GroupVersionKind{Group: "kubernaut.ai", Version: "v1alpha1", Kind: "RemediationRequest"}

// setRROwnerReference looks up the RemediationRequest and, if found, sets an
// OwnerReference on the IS CRD so Kubernetes garbage-collects the IS when the
// RR is deleted (#1300). Best-effort: a lookup failure is logged and the IS is
// still created without an owner reference.
func setRROwnerReference(ctx context.Context, c client.Client, logger logr.Logger, crd *v1alpha1.InvestigationSession, rrNamespace, rrName string) {
	rr := &unstructured.Unstructured{}
	rr.SetGroupVersionKind(rrGVK)
	if err := c.Get(ctx, types.NamespacedName{Namespace: rrNamespace, Name: rrName}, rr); err != nil {
		logger.V(1).Info("RR lookup for owner reference failed (IS will be created without cascade GC)",
			"rr_namespace", rrNamespace, "rr_name", rrName, "error", err.Error())
		return
	}

	crd.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "kubernaut.ai/v1alpha1",
			Kind:       "RemediationRequest",
			Name:       rr.GetName(),
			UID:        rr.GetUID(),
		},
	}
}

var invalidLabelChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// sanitizeLabelValue truncates and cleans a string for use as a K8s label
// value (max 63 chars, must match [a-zA-Z0-9._-], must start and end with
// an alphanumeric character per the K8s label value specification).
func sanitizeLabelValue(v string) string {
	v = invalidLabelChars.ReplaceAllString(v, "_")
	if len(v) > 63 {
		v = v[:63]
	}
	v = strings.TrimLeft(v, "._-")
	v = strings.TrimRight(v, "._-")
	if v == "" {
		v = "unknown"
	}
	return v
}
