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

// Package creator provides child CRD creation logic for the AIAnalysis
// controller, mirroring the pkg/remediationorchestrator/creator convention.
package creator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
	"github.com/jordigilh/kubernaut/pkg/aianalysis/handlers"
	"github.com/jordigilh/kubernaut/pkg/shared/parentlifecycle"
)

// AgentSessionCreator creates AgentSession CRDs from AIAnalysis, replacing
// the retired agentclient.SubmitInvestigation HTTP call (DD-AA-KA-001,
// BR-AA-KA-065.1). A narrow, single-purpose collaborator -- kept separate
// from InvestigatingHandler so the handler is never handed a raw
// client.Client, matching the existing AgentClientInterface/
// InvestigationSessionChecker/ISPhaseUpdater narrow-interface convention on
// that struct.
type AgentSessionCreator struct {
	client  client.Client
	scheme  *runtime.Scheme
	builder *handlers.RequestBuilder
}

// NewAgentSessionCreator creates a new AgentSessionCreator.
func NewAgentSessionCreator(c client.Client, s *runtime.Scheme) *AgentSessionCreator {
	return &AgentSessionCreator{
		client:  c,
		scheme:  s,
		builder: handlers.NewRequestBuilder(log.Log),
	}
}

// GetOrCreate returns the AgentSession for the given AIAnalysis. The first
// creation is CreateOnly. Recovery is allowed only through an explicitly
// persisted AgentSessionRef whose UID, owner, and immutable spec identity
// validate (BR-AA-KA-065.1, DD-2582).
//
// The ownerRef is set to the AIAnalysis CR itself, not the RemediationRequest
// directly (DD-AA-KA-001 Decision section): AA already holds analysis live,
// so no new RemediationRequest RBAC grant is needed (AC-6). Cascade deletion
// still reaches the RR transitively, since RO already sets the RR as
// AIAnalysis's own owner.
func (c *AgentSessionCreator) GetOrCreate(ctx context.Context, analysis *aianalysisv1.AIAnalysis) (*agentsessionv1.AgentSession, error) {
	logger := log.FromContext(ctx).WithValues(
		"aianalysis", analysis.Name,
		"namespace", analysis.Namespace,
	)

	if analysis.Spec.RemediationRequestRef.Name == "" {
		return nil, fmt.Errorf("cannot create AgentSession: AIAnalysis %s/%s has an empty RemediationRequestRef.Name", analysis.Namespace, analysis.Name)
	}

	name := fmt.Sprintf("as-%s", analysis.Spec.RemediationRequestRef.Name)

	if ref := persistedAgentSessionRef(analysis); ref != nil {
		return c.recoverPersistedAgentSession(ctx, analysis, ref, name, logger)
	}

	// Defensive: prevents an ownerless AgentSession if AIAnalysis isn't
	// fully persisted yet (mirrors creator.AIAnalysisCreator's RR.UID check).
	if analysis.UID == "" {
		return nil, fmt.Errorf("cannot set owner reference: AIAnalysis %s/%s has an empty UID", analysis.Namespace, analysis.Name)
	}

	as := &agentsessionv1.AgentSession{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: analysis.Namespace,
			// TerminalCloseFinalizer set synchronously here, not left to
			// AF's AgentSessionTerminalCloseReconciler to add reactively on
			// its own first reconcile (#2214 CI RCA, PR #2222): closes the
			// narrow bootstrap race where a delete (DeleteForCascadeCancel)
			// could otherwise land before that first reconcile observes
			// this Create, removing the object with no finalizer ever
			// attached. See the constant's doc comment for full detail.
			Finalizers: []string{agentsessionv1.TerminalCloseFinalizer},
		},
		Spec: c.builder.BuildAgentSessionSpec(analysis),
	}

	if err := controllerutil.SetControllerReference(analysis, as, c.scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	if err := c.client.Create(ctx, as); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil, parentlifecycle.NewChildCollisionError("AgentSession", analysis.Namespace, name, err)
		}
		return nil, fmt.Errorf("failed to create AgentSession: %w", err)
	}

	logger.Info("Created AgentSession", "name", name)
	return as, nil
}

func (c *AgentSessionCreator) recoverPersistedAgentSession(
	ctx context.Context,
	analysis *aianalysisv1.AIAnalysis,
	ref *corev1.ObjectReference,
	name string,
	logger logr.Logger,
) (*agentsessionv1.AgentSession, error) {
	if ref.Name != name || ref.Namespace != analysis.Namespace || ref.UID == "" {
		return nil, fmt.Errorf("persisted AgentSession recovery identity does not match expected %s/%s", analysis.Namespace, name)
	}
	existing := &agentsessionv1.AgentSession{}
	err := c.client.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: analysis.Namespace}, existing)
	if err != nil && apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("persisted AgentSession recovery object %s/%s is missing: %w", analysis.Namespace, ref.Name, err)
	}
	if err != nil {
		return nil, fmt.Errorf("read persisted AgentSession recovery identity: %w", err)
	}
	if err := validateRecoveryIdentity(analysis, existing, ref, c.builder.BuildAgentSessionSpec(analysis)); err != nil {
		return nil, err
	}
	logger.V(1).Info("Recovered persisted AgentSession", "name", name, "uid", existing.UID)
	return existing, nil
}

func persistedAgentSessionRef(analysis *aianalysisv1.AIAnalysis) *corev1.ObjectReference {
	if analysis.Status.KASession == nil || analysis.Status.KASession.AgentSessionRef == nil ||
		analysis.Status.KASession.AgentSessionRef.Name == "" {
		return nil
	}
	return analysis.Status.KASession.AgentSessionRef
}

func validateRecoveryIdentity(
	analysis *aianalysisv1.AIAnalysis,
	session *agentsessionv1.AgentSession,
	ref *corev1.ObjectReference,
	expectedSpec agentsessionv1.AgentSessionSpec,
) error {
	if ref.UID == "" || session.UID != ref.UID {
		return fmt.Errorf("persisted AgentSession recovery UID mismatch for %s/%s", session.Namespace, session.Name)
	}
	owner := metav1.GetControllerOf(session)
	if owner == nil || owner.Kind != "AIAnalysis" || owner.Name != analysis.Name || owner.UID != analysis.UID {
		return fmt.Errorf("persisted AgentSession recovery owner mismatch for %s/%s", session.Namespace, session.Name)
	}
	if !agentSessionSpecsEqual(session.Spec, expectedSpec) {
		return fmt.Errorf("persisted AgentSession recovery spec identity mismatch for %s/%s", session.Namespace, session.Name)
	}
	return nil
}

// agentSessionSpecsEqual keeps immutable-spec recovery fail-closed while
// treating apiextensionsv1.JSON as JSON rather than as a byte slice. The API
// server may preserve equivalent JSON with different whitespace or object-key
// order when it round-trips a CRD; those serialization details are not a spec
// identity change. Every typed field remains reflect.DeepEqual, and malformed
// JSON is rejected rather than adopted.
func agentSessionSpecsEqual(persisted, expected agentsessionv1.AgentSessionSpec) bool {
	persistedEnrichment := persisted.EnrichmentResults
	expectedEnrichment := expected.EnrichmentResults
	persisted.EnrichmentResults = nil
	expected.EnrichmentResults = nil

	typedEqual := reflect.DeepEqual(persisted, expected)
	jsonEqual := equivalentJSON(persistedEnrichment, expectedEnrichment)
	return typedEqual && jsonEqual
}

func equivalentJSON(persisted, expected *apiextensionsv1.JSON) bool {
	if persisted == nil || expected == nil {
		return persisted == nil && expected == nil
	}

	persistedCanonical, err := canonicalJSON(persisted.Raw)
	if err != nil {
		return false
	}
	expectedCanonical, err := canonicalJSON(expected.Raw)
	if err != nil {
		return false
	}
	return bytes.Equal(persistedCanonical, expectedCanonical)
}

func canonicalJSON(raw []byte) ([]byte, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("json is empty")
	}

	switch raw[0] {
	case '{':
		return canonicalJSONObject(raw)

	case '[':
		return canonicalJSONArray(raw)

	default:
		return canonicalJSONScalar(raw)
	}
}

func canonicalJSONObject(raw []byte) ([]byte, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var canonical bytes.Buffer
	canonical.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			canonical.WriteByte(',')
		}
		encodedKey, err := json.Marshal(key)
		if err != nil {
			return nil, err
		}
		canonical.Write(encodedKey)
		canonical.WriteByte(':')
		encodedValue, err := canonicalJSON(object[key])
		if err != nil {
			return nil, err
		}
		canonical.Write(encodedValue)
	}
	canonical.WriteByte('}')
	return canonical.Bytes(), nil
}

func canonicalJSONArray(raw []byte) ([]byte, error) {
	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err != nil {
		return nil, err
	}

	var canonical bytes.Buffer
	canonical.WriteByte('[')
	for index, item := range array {
		if index > 0 {
			canonical.WriteByte(',')
		}
		encodedItem, err := canonicalJSON(item)
		if err != nil {
			return nil, err
		}
		canonical.Write(encodedItem)
	}
	canonical.WriteByte(']')
	return canonical.Bytes(), nil
}

func canonicalJSONScalar(raw []byte) ([]byte, error) {
	var canonical bytes.Buffer
	if err := json.Compact(&canonical, raw); err != nil {
		return nil, err
	}
	return canonical.Bytes(), nil
}

// DeleteForRetry deletes as so that the next reconcile's GetOrCreate call
// naturally falls through to Create, giving the retry attempt a fresh
// AgentSession rather than mutating a terminal Failed object (BR-AI-009,
// DD-AA-KA-001 amendment: AgentSessionReasonCapacityExceeded retry path).
// Idempotent: a NotFound (e.g. a concurrent retry, or a delete already
// observed by another reconcile) is not an error.
//
// Explicitly removes agentsessionv1.TerminalCloseFinalizer before deleting
// (#2214 finalizer redesign): a capacity-exceeded retry is not a terminal
// outcome for the underlying investigation -- it immediately continues
// under the same rrName with a fresh AgentSession -- so it must NOT trigger
// AF's AgentSessionTerminalCloseReconciler's Cancelled-IS-closure the way
// DeleteForCascadeCancel intentionally does. This also keeps the delete
// synchronous (the deterministic as-<rrName> name must be free again
// immediately for GetOrCreate's same-named Create, which would otherwise
// collide with a terminating-but-not-yet-removed object of that name).
func (c *AgentSessionCreator) DeleteForRetry(ctx context.Context, as *agentsessionv1.AgentSession) error {
	removedTerminalClose := controllerutil.RemoveFinalizer(as, agentsessionv1.TerminalCloseFinalizer)
	// #2231 / DD-AA-KA-001 Amendment: KA's Dispatcher now also carries
	// DispatchCleanupFinalizer (replacing its prior raw watch.Deleted
	// handling). Left unstripped, this Delete would defer to KA's own
	// Dispatcher.Reconcile to observe the DeletionTimestamp before the
	// object actually disappears -- reintroducing exactly the "retry Create
	// collides with a still-present, terminating object" race this
	// function's synchronous-delete contract exists to avoid. Safe to strip
	// unconditionally: see DispatchCleanupFinalizer's doc comment for why
	// this specific call site never has an in-memory investigation to lose.
	removedDispatchCleanup := controllerutil.RemoveFinalizer(as, agentsessionv1.DispatchCleanupFinalizer)
	if removedTerminalClose || removedDispatchCleanup {
		if err := c.client.Update(ctx, as); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to remove finalizers from AgentSession %s/%s before retry delete: %w", as.Namespace, as.Name, err)
		}
	}
	if err := c.client.Delete(ctx, as); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete AgentSession %s/%s for retry: %w", as.Namespace, as.Name, err)
	}
	return nil
}

// DeleteForCascadeCancel deletes the deterministically-named AgentSession
// (as-<rrName>) for the given RemediationRequest on external cascade-cancel
// (#1421 ParentCancelled), replacing AA's retired direct IS write
// (K8sISPhaseUpdater.SetTerminalPhase) with an AgentSession delete (#2214 /
// DD-AA-KA-001 Amendment). This lets two independent, already-proven
// consumers react to the same signal without new coupling:
//   - AF's AgentSessionTerminalCloseReconciler (watching AgentSession)
//     closes the correlated InvestigationSession to Cancelled.
//   - KA's Dispatcher.cancelOnDelete stops the in-flight investigation
//     goroutine.
//
// Idempotent: a NotFound (e.g. the AgentSession was never created, or a
// concurrent cascade-cancel already deleted it) is not an error, mirroring
// DeleteForRetry's contract.
func (c *AgentSessionCreator) DeleteForCascadeCancel(ctx context.Context, rrName, namespace string) error {
	name := fmt.Sprintf("as-%s", rrName)
	as := &agentsessionv1.AgentSession{}
	if err := c.client.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, as); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get agentsession %s/%s for cascade-cancel: %w", namespace, name, err)
	}
	if as.Status.Phase == agentsessionv1.AgentSessionPhaseCompleted ||
		as.Status.Phase == agentsessionv1.AgentSessionPhaseFailed ||
		as.Status.Phase == agentsessionv1.AgentSessionPhaseCancelled {
		return nil
	}
	if err := c.client.Delete(ctx, as); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete agentsession %s/%s for cascade-cancel: %w", namespace, name, err)
	}
	return nil
}
