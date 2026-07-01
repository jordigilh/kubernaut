/*
Copyright 2025 Jordi Gil.

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

package controller

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	roconfig "github.com/jordigilh/kubernaut/internal/config/remediationorchestrator"
	"github.com/jordigilh/kubernaut/pkg/fleet"
	"github.com/jordigilh/kubernaut/pkg/remediationorchestrator/locking"
)

// IsTerminalPhase checks if a phase is terminal (no further processing).
// BR-ORCH-042.2: Blocked is NON-terminal (active)
// #280: Verifying is NON-terminal (EA assessment in progress)
//
// AC-042-2-1: IsTerminal(Blocked) returns false
func IsTerminalPhase(phase remediationv1.RemediationPhase) bool {
	terminalPhases := []remediationv1.RemediationPhase{
		remediationv1.PhaseCompleted,
		remediationv1.PhaseFailed,
		remediationv1.PhaseTimedOut,
		remediationv1.PhaseCancelled,
		remediationv1.PhaseSkipped,
	}

	for _, terminal := range terminalPhases {
		if phase == terminal {
			return true
		}
	}
	return false
}

// SetRESTMapper sets the REST mapper used by CapturePreRemediationHash to
// resolve Kind strings to GroupVersionKind for the unstructured client.
// DD-EM-002: Called from cmd/remediationorchestrator/main.go after manager setup.
func (r *Reconciler) SetRESTMapper(mapper meta.RESTMapper) {
	r.restMapper = mapper
}

// SetReaderFactory configures fleet-aware target reads (BR-FLEET-054).
// When set, CapturePreRemediationHash routes reads to the remote cluster
// via ReaderFactory for RRs with a non-empty ClusterID.
func (r *Reconciler) SetReaderFactory(rf fleet.ReaderFactory) {
	r.readerFactory = rf
}

// readerForHash returns the appropriate client.Reader for pre-remediation
// hash capture. Uses apiReader (uncached, direct API) for local clusters,
// and the fleet reader for remote clusters (BR-FLEET-054).
func (r *Reconciler) readerForHash(ctx context.Context, clusterID string) (client.Reader, error) {
	if clusterID == "" || r.readerFactory == nil {
		return r.apiReader, nil
	}
	return r.readerFactory.ReaderFor(ctx, clusterID)
}

// SetAsyncPropagation configures propagation delays for async-managed targets.
// DD-EM-004 v2.0, Issue #253: Called from cmd/remediationorchestrator/main.go.
// Thread-safe: acquires configMu write lock (#835, DD-INFRA-001).
func (r *Reconciler) SetAsyncPropagation(cfg roconfig.AsyncPropagationConfig) {
	r.configMu.Lock()
	r.asyncPropagation = cfg
	r.configMu.Unlock()
}

// getAsyncPropagation returns the current async propagation config.
// Thread-safe: acquires configMu read lock (#835).
func (r *Reconciler) getAsyncPropagation() roconfig.AsyncPropagationConfig {
	r.configMu.RLock()
	defer r.configMu.RUnlock()
	return r.asyncPropagation
}

// IsDryRunExported exposes isDryRun for unit testing hot-reload thread safety (#835).
func (r *Reconciler) IsDryRunExported() bool {
	return r.isDryRun()
}

// GetDryRunHoldPeriodExported exposes getDryRunHoldPeriod for unit testing (#835).
func (r *Reconciler) GetDryRunHoldPeriodExported() time.Duration {
	return r.getDryRunHoldPeriod()
}

// GetRetentionPeriodExported exposes getRetentionPeriod for unit testing (#835).
func (r *Reconciler) GetRetentionPeriodExported() time.Duration {
	return r.getRetentionPeriod()
}

// GetAsyncPropagationExported exposes getAsyncPropagation for unit testing (#835).
func (r *Reconciler) GetAsyncPropagationExported() roconfig.AsyncPropagationConfig {
	return r.getAsyncPropagation()
}

// SetNotifySelfResolved enables or disables the self-resolved status-update notification.
// BR-ORCH-037 AC-037-08, Issue #590: Called from cmd/remediationorchestrator/main.go.
func (r *Reconciler) SetNotifySelfResolved(enabled bool) {
	r.aiAnalysisHandler.SetNotifySelfResolved(enabled)
}

// SetClusterIdentity configures the cluster name and UUID for inclusion in notification bodies.
// Issue #615: Called from cmd/remediationorchestrator/main.go after DiscoverIdentity.
func (r *Reconciler) SetClusterIdentity(name, uuid string) {
	r.notificationCreator.SetClusterIdentity(name, uuid)
}

// SetLockManager configures the distributed lock manager for WFE creation safety.
// BR-ORCH-025: Called from cmd/remediationorchestrator/main.go.
// nil = locking disabled (single-replica deployments).
func (r *Reconciler) SetLockManager(lm *locking.DistributedLockManager) {
	r.lockManager = lm
}

// SetFleetConfig configures the fleet settings for federated scope checking.
// ADR-068: Used in the default routing engine fallback path (routingEngine == nil)
// to wrap the local scope.Manager with fleet.NewScopeChecker.
// Called from cmd/remediationorchestrator/main.go.
func (r *Reconciler) SetFleetConfig(cfg fleet.FleetConfig) {
	r.configMu.Lock()
	r.fleetConfig = cfg
	r.configMu.Unlock()
}
