package session

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	adksession "google.golang.org/adk/session"

	v1alpha1 "github.com/jordigilh/kubernaut-apifrontend/api/apifrontend/v1alpha1"
)

// Label keys used on InvestigationSession CRDs.
const (
	LabelUser      = "apifrontend.kubernaut.ai/user"
	LabelRRName    = "apifrontend.kubernaut.ai/rr-name"
	LabelPhase     = "apifrontend.kubernaut.ai/phase"
	LabelManagedBy = "app.kubernetes.io/managed-by"
)

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
	delegate  adksession.Service
	client    client.Client
	scheme    *runtime.Scheme
	namespace string
	logger    *slog.Logger

	mu       sync.RWMutex
	crdIndex map[string]string // sessionID -> CRD name
}

// NewCRDSessionService creates a new CRDSessionService. The delegate should
// typically be adksession.InMemoryService().
func NewCRDSessionService(delegate adksession.Service, c client.Client, scheme *runtime.Scheme, ns string) *CRDSessionService {
	return &CRDSessionService{
		delegate:  delegate,
		client:    c,
		scheme:    scheme,
		namespace: ns,
		logger:    slog.Default().With("component", "session-service"),
		crdIndex:  make(map[string]string),
	}
}

// Create creates an InvestigationSession CRD and delegates session creation
// to the in-memory service. The CRD creation config is read from
// req.State[StateKeyCreateConfig].
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

	now := metav1.Now()
	crd := &v1alpha1.InvestigationSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: s.namespace,
			Labels: map[string]string{
				LabelPhase:     string(v1alpha1.SessionPhaseActive),
				LabelManagedBy: "kubernaut-apifrontend",
			},
		},
		Status: v1alpha1.InvestigationSessionStatus{
			Phase:     v1alpha1.SessionPhaseActive,
			StartedAt: &now,
		},
	}

	if cfg != nil {
		crd.OwnerReferences = []metav1.OwnerReference{cfg.OwnerRef}
		crd.Labels[LabelUser] = sanitizeLabelValue(cfg.UserIdentity.Username)
		crd.Labels[LabelRRName] = sanitizeLabelValue(cfg.RemediationRef.Name)
		crd.Spec = v1alpha1.InvestigationSessionSpec{
			RemediationRequestRef: cfg.RemediationRef,
			A2ATaskID:             cfg.A2ATaskID,
			UserIdentity:          cfg.UserIdentity,
			JoinMode:              cfg.JoinMode,
		}
	}

	if err := s.client.Create(ctx, crd); err != nil {
		return nil, fmt.Errorf("create InvestigationSession CRD: %w", err)
	}

	if err := s.client.Status().Update(ctx, crd); err != nil {
		_ = s.client.Delete(ctx, crd)
		return nil, fmt.Errorf("set InvestigationSession initial status: %w", err)
	}

	resp, err := s.delegate.Create(ctx, req)
	if err != nil {
		_ = s.client.Delete(ctx, crd)
		return nil, fmt.Errorf("delegate create: %w", err)
	}

	s.mu.Lock()
	s.crdIndex[resp.Session.ID()] = crdName
	s.mu.Unlock()

	s.logger.InfoContext(ctx, "session created",
		"session_id", resp.Session.ID(),
		"crd_name", crdName,
		"user", req.UserID,
	)
	return resp, nil
}

// Get delegates to the in-memory service.
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

	// Delete CRD (best-effort; may not exist after restart)
	crd := &v1alpha1.InvestigationSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crdName,
			Namespace: s.namespace,
		},
	}
	_ = s.client.Delete(ctx, crd)

	s.mu.Lock()
	delete(s.crdIndex, req.SessionID)
	s.mu.Unlock()

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
		var crd v1alpha1.InvestigationSession
		if err := s.client.Get(ctx, types.NamespacedName{Name: crdName, Namespace: s.namespace}, &crd); err == nil {
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

	var crd v1alpha1.InvestigationSession
	if err := s.client.Get(ctx, types.NamespacedName{Name: crdName, Namespace: s.namespace}, &crd); err != nil {
		return "", fmt.Errorf("get session phase: %w", err)
	}
	return crd.Status.Phase, nil
}

var _ adksession.Service = (*CRDSessionService)(nil)

var invalidLabelChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// sanitizeLabelValue truncates and cleans a string for use as a K8s label
// value (max 63 chars, must match [a-zA-Z0-9._-]).
func sanitizeLabelValue(v string) string {
	v = invalidLabelChars.ReplaceAllString(v, "_")
	if len(v) > 63 {
		v = v[:63]
	}
	return v
}
