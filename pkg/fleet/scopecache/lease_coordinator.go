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

package scopecache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	coordinationclient "k8s.io/client-go/kubernetes/typed/coordination/v1"
)

const holderSessionAnnotation = "kubernaut.ai/fmc-holder-session-uid"

// KubernetesLeaseCoordinator uses resource-version CAS on a coordination
// Lease. The Lease's leaseTransitions value is the sole monotonic epoch used
// by the Valkey writer.
type KubernetesLeaseCoordinator struct {
	leases   coordinationclient.LeaseInterface
	name     string
	podUID   types.UID
	duration int32
	logger   logr.Logger

	mu      sync.Mutex
	current WriterFence
	lostCh  chan struct{}
	lost    bool
}

// NewKubernetesLeaseCoordinator creates a Lease coordinator in namespace.
func NewKubernetesLeaseCoordinator(client coordinationclient.CoordinationV1Interface, namespace, name string, podUID types.UID, duration time.Duration, logger logr.Logger) (*KubernetesLeaseCoordinator, error) {
	if client == nil {
		return nil, fmt.Errorf("coordination client is required")
	}
	if namespace == "" || name == "" || podUID == "" {
		return nil, fmt.Errorf("lease namespace, name and pod UID are required")
	}
	if duration <= 0 {
		duration = 60 * time.Second
	}
	seconds := int32(duration / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	return &KubernetesLeaseCoordinator{
		leases:   client.Leases(namespace),
		name:     name,
		podUID:   podUID,
		duration: seconds,
		logger:   logger.WithName("fmc-lease"),
	}, nil
}

func (c *KubernetesLeaseCoordinator) Acquire(ctx context.Context) (LeaseSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	lease, err := c.leases.Get(ctx, c.name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		fence, createErr := c.create(ctx)
		if createErr != nil {
			return LeaseSession{}, createErr
		}
		return c.activateSession(fence), nil
	}
	if err != nil {
		return LeaseSession{}, fmt.Errorf("get FMC lease: %w", err)
	}
	if !leaseAvailable(lease, time.Now().UTC()) {
		return LeaseSession{}, ErrLeaseHeld
	}

	previousEpoch := int64(0)
	if lease.Spec.LeaseTransitions != nil {
		previousEpoch = int64(*lease.Spec.LeaseTransitions)
	}
	fence, err := c.updateOwnership(ctx, lease, previousEpoch+1)
	if err != nil {
		return LeaseSession{}, err
	}
	return c.activateSession(fence), nil
}

func (c *KubernetesLeaseCoordinator) create(ctx context.Context) (WriterFence, error) {
	now := metav1.NewMicroTime(time.Now().UTC())
	transitions := int32(1)
	session := uuid.NewString()
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name: c.name,
			Annotations: map[string]string{
				holderSessionAnnotation: session,
			},
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       stringPtr(string(c.podUID)),
			LeaseDurationSeconds: &c.duration,
			AcquireTime:          &now,
			RenewTime:            &now,
			LeaseTransitions:     &transitions,
		},
	}
	created, err := c.leases.Create(ctx, lease, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return WriterFence{}, ErrLeaseHeld
	}
	if err != nil {
		return WriterFence{}, fmt.Errorf("create FMC lease: %w", err)
	}
	return c.fenceFromLease(created, session)
}

func (c *KubernetesLeaseCoordinator) updateOwnership(ctx context.Context, lease *coordinationv1.Lease, epoch int64) (WriterFence, error) {
	if epoch <= 0 || epoch > int64(^uint32(0)) {
		return WriterFence{}, fmt.Errorf("lease transition epoch is out of range")
	}
	session := uuid.NewString()
	now := metav1.NewMicroTime(time.Now().UTC())
	updated := lease.DeepCopy()
	if updated.Annotations == nil {
		updated.Annotations = make(map[string]string)
	}
	updated.Annotations[holderSessionAnnotation] = session
	updated.Spec.HolderIdentity = stringPtr(string(c.podUID))
	updated.Spec.LeaseDurationSeconds = &c.duration
	updated.Spec.AcquireTime = &now
	updated.Spec.RenewTime = &now
	transitions := int32(epoch)
	updated.Spec.LeaseTransitions = &transitions
	result, err := c.leases.Update(ctx, updated, metav1.UpdateOptions{})
	if apierrors.IsConflict(err) {
		return WriterFence{}, ErrLeaseHeld
	}
	if err != nil {
		return WriterFence{}, fmt.Errorf("claim FMC lease: %w", err)
	}
	return c.fenceFromLease(result, session)
}

func (c *KubernetesLeaseCoordinator) Verify(ctx context.Context, expected WriterFence) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.current != expected {
		c.mu.Unlock()
		return ErrStaleFence
	}
	c.mu.Unlock()
	lease, err := c.leases.Get(ctx, c.name, metav1.GetOptions{})
	if err != nil {
		c.markLost(expected, err)
		return fmt.Errorf("verify FMC lease: %w", err)
	}
	if !leaseMatches(lease, expected, string(c.podUID)) {
		err := fmt.Errorf("%w: lease ownership changed", ErrStaleFence)
		c.markLost(expected, err)
		return err
	}
	if leaseAvailable(lease, time.Now().UTC()) {
		err := fmt.Errorf("%w: lease expired", ErrStaleFence)
		c.markLost(expected, err)
		return err
	}
	return nil
}

func (c *KubernetesLeaseCoordinator) Renew(ctx context.Context, expected WriterFence) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.current != expected {
		c.mu.Unlock()
		return ErrStaleFence
	}
	lease, err := c.leases.Get(ctx, c.name, metav1.GetOptions{})
	if err != nil {
		c.mu.Unlock()
		c.markLost(expected, err)
		return fmt.Errorf("get FMC lease for renewal: %w", err)
	}
	if !leaseMatches(lease, expected, string(c.podUID)) {
		err := fmt.Errorf("%w: lease ownership changed", ErrStaleFence)
		c.mu.Unlock()
		c.markLost(expected, err)
		return err
	}
	if leaseAvailable(lease, time.Now().UTC()) {
		err := fmt.Errorf("%w: lease expired", ErrStaleFence)
		c.mu.Unlock()
		c.markLost(expected, err)
		return err
	}
	updated := lease.DeepCopy()
	now := metav1.NewMicroTime(time.Now().UTC())
	updated.Spec.RenewTime = &now
	result, err := c.leases.Update(ctx, updated, metav1.UpdateOptions{})
	if apierrors.IsConflict(err) {
		c.mu.Unlock()
		c.markLost(expected, ErrStaleFence)
		return ErrStaleFence
	}
	if err != nil {
		c.mu.Unlock()
		c.markLost(expected, err)
		return fmt.Errorf("renew FMC lease: %w", err)
	}
	c.mu.Unlock()
	_ = result
	return nil
}

func (c *KubernetesLeaseCoordinator) Release(ctx context.Context, expected WriterFence) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.current != expected {
		c.mu.Unlock()
		return ErrStaleFence
	}
	lease, err := c.leases.Get(ctx, c.name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		c.current = WriterFence{}
		c.lostCh = nil
		c.lost = false
		c.mu.Unlock()
		return nil
	}
	if err != nil {
		c.mu.Unlock()
		return fmt.Errorf("get FMC lease for release: %w", err)
	}
	if !leaseMatches(lease, expected, string(c.podUID)) {
		c.mu.Unlock()
		return ErrStaleFence
	}
	updated := lease.DeepCopy()
	updated.Spec.HolderIdentity = nil
	updated.Spec.AcquireTime = nil
	updated.Spec.RenewTime = nil
	if updated.Annotations != nil {
		delete(updated.Annotations, holderSessionAnnotation)
	}
	if _, err := c.leases.Update(ctx, updated, metav1.UpdateOptions{}); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("release FMC lease: %w", err)
	}
	c.current = WriterFence{}
	c.lostCh = nil
	c.lost = false
	c.mu.Unlock()
	return nil
}

func (c *KubernetesLeaseCoordinator) fenceFromLease(lease *coordinationv1.Lease, session string) (WriterFence, error) {
	if lease == nil || lease.Spec.LeaseTransitions == nil || *lease.Spec.LeaseTransitions <= 0 {
		return WriterFence{}, fmt.Errorf("FMC lease has no positive leaseTransitions")
	}
	fence := WriterFence{
		HolderPodUID:     string(c.podUID),
		HolderSessionUID: session,
		LeaseEpoch:       int64(*lease.Spec.LeaseTransitions),
		Fence:            uint64(*lease.Spec.LeaseTransitions),
	}
	if err := fence.Validate(); err != nil {
		return WriterFence{}, err
	}
	return fence, nil
}

func (c *KubernetesLeaseCoordinator) activateSession(fence WriterFence) LeaseSession {
	c.current = fence
	c.lostCh = make(chan struct{})
	c.lost = false
	return LeaseSession{Fence: fence, Loss: c.lostCh}
}

func (c *KubernetesLeaseCoordinator) markLost(expected WriterFence, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != expected || c.lostCh == nil || c.lost {
		return
	}
	c.logger.Error(err, "FMC writer lease lost")
	c.lost = true
	close(c.lostCh)
}

func leaseAvailable(lease *coordinationv1.Lease, now time.Time) bool {
	if lease == nil {
		return true
	}
	if lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity == "" || lease.Spec.RenewTime == nil || lease.Spec.LeaseDurationSeconds == nil {
		return true
	}
	expires := lease.Spec.RenewTime.Add(time.Duration(*lease.Spec.LeaseDurationSeconds) * time.Second)
	return !now.Before(expires)
}

func leaseMatches(lease *coordinationv1.Lease, expected WriterFence, podUID string) bool {
	if lease == nil || lease.Spec.HolderIdentity == nil || *lease.Spec.HolderIdentity != podUID || lease.Spec.LeaseTransitions == nil {
		return false
	}
	if int64(*lease.Spec.LeaseTransitions) != expected.LeaseEpoch {
		return false
	}
	return lease.Annotations != nil && lease.Annotations[holderSessionAnnotation] == expected.HolderSessionUID
}

func stringPtr(value string) *string { return &value }

var _ LeaseCoordinator = (*KubernetesLeaseCoordinator)(nil)
