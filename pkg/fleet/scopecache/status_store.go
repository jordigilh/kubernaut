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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/jordigilh/kubernaut/pkg/fleet/inventory"
	"github.com/jordigilh/kubernaut/pkg/shared/scope"
)

// ValkeyStatusStore is the v2 fenced materialized-state store. It never uses
// client-side mutation sequences; every operation is one EVALSHA invocation.
type ValkeyStatusStore struct {
	client  *redis.Client
	scripts *ScriptManager
}

// NewValkeyStatusStore creates a status store backed by Valkey.
func NewValkeyStatusStore(client *redis.Client) (*ValkeyStatusStore, error) {
	scripts, err := NewScriptManager(client)
	if err != nil {
		return nil, err
	}
	return &ValkeyStatusStore{client: client, scripts: scripts}, nil
}

// NewValkeyStatusStoreAt creates a status store with a standalone Valkey
// client. TLS and authentication can be supplied through options.
func NewValkeyStatusStoreAt(addr string, opts ...func(*redis.Options)) (*ValkeyStatusStore, error) {
	redisOpts := &redis.Options{Addr: addr}
	for _, opt := range opts {
		opt(redisOpts)
	}
	return NewValkeyStatusStore(redis.NewClient(redisOpts))
}

// ScriptReady reports whether bootstrap has loaded the embedded script.
func (s *ValkeyStatusStore) ScriptReady() bool {
	if s == nil || s.scripts == nil {
		return false
	}
	return s.scripts.loadedState()
}

// LoadScript is called during FMC startup and recovery before readiness is
// published.
func (s *ValkeyStatusStore) LoadScript(ctx context.Context) error {
	if s == nil || s.scripts == nil {
		return ErrScriptUnavailable
	}
	return s.scripts.Load(ctx)
}

// Close closes the Valkey connection owned by the status store.
func (s *ValkeyStatusStore) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

func (s *ValkeyStatusStore) BeginRepair(ctx context.Context, request BeginRepairRequest) (RepairStartResult, error) {
	if err := validateBeginRepairRequest(request); err != nil {
		return RepairStartResult{}, repairFailureError(RepairFailureInvalidRequest, err, false)
	}
	token := RepairToken{Value: uuid.NewString()}
	items, err := s.evalMutation(ctx, []string{statusKey(request.Partition)},
		"begin", request.RegistrationUID, request.ExpectedRegistryRevision,
		request.WriterFence.HolderPodUID, request.WriterFence.HolderSessionUID,
		request.WriterFence.LeaseEpoch, request.WriterFence.Fence, token.Value)
	if err != nil {
		return RepairStartResult{}, classifyRepairError(err)
	}
	generation, err := scriptInt(items, 0)
	if err != nil || generation <= 0 {
		if err == nil {
			err = fmt.Errorf("generation must be positive")
		}
		return RepairStartResult{}, repairFailureError(RepairFailureAmbiguousWrite, fmt.Errorf("invalid begin repair response: %w", err), false)
	}
	result := RepairStartResult{
		Partition:                request.Partition,
		RegistrationUID:          request.RegistrationUID,
		ExpectedRegistryRevision: request.ExpectedRegistryRevision,
		WriterFence:              request.WriterFence,
		Generation:               uint64(generation),
		Token:                    token,
	}
	return result, nil
}

func (s *ValkeyStatusStore) repairObservation(ctx context.Context, request RepairRequest, observation RepairObservation) (string, repairReconcileProof, error) {
	publicID, err := canonicalObservationID(observation)
	if err != nil {
		return "", repairReconcileProof{}, repairFailureError(RepairFailureRecordInvalid, err, false)
	}
	items, err := s.evalMutation(ctx,
		[]string{statusKey(request.Partition), recordKey(request.Partition, publicID), indexKey(request.Partition)},
		"repair_reconcile", request.RegistrationUID, request.ExpectedRegistryRevision,
		request.WriterFence.HolderPodUID, request.WriterFence.HolderSessionUID,
		request.WriterFence.LeaseEpoch, request.WriterFence.Fence, request.Generation,
		request.Token.Value, publicID, observation.Identity.Group, observation.Identity.Version,
		observation.Identity.Kind, observation.Identity.Namespace, observation.Identity.Name,
		string(observation.LabelState), observation.TTL.Milliseconds())
	if err != nil {
		return publicID, repairReconcileProof{}, err
	}
	proof, err := parseRepairReconcile(items)
	if err != nil {
		return publicID, repairReconcileProof{}, repairFailureError(RepairFailureTTLInvalid, err, false)
	}
	return publicID, proof, nil
}

func (s *ValkeyStatusStore) cleanupRepairMembers(ctx context.Context, request RepairRequest, seen map[string]struct{}) (int, error) {
	cursor := 0
	cleanupCount := 0
	for {
		items, err := s.evalMutation(ctx, []string{statusKey(request.Partition), indexKey(request.Partition)},
			"repair_list_members", request.RegistrationUID, request.ExpectedRegistryRevision,
			request.WriterFence.HolderPodUID, request.WriterFence.HolderSessionUID,
			request.WriterFence.LeaseEpoch, request.WriterFence.Fence, request.Generation,
			request.Token.Value, cursor)
		if err != nil {
			return cleanupCount, err
		}
		next, members, err := parseRepairMembers(items)
		if err != nil {
			return cleanupCount, repairFailureError(RepairFailureIndexInvalid, err, false)
		}
		for _, member := range members {
			if _, exists := seen[member]; exists {
				continue
			}
			count, err := s.cleanupRepairMember(ctx, request, member)
			if err != nil {
				return cleanupCount, err
			}
			cleanupCount += int(count)
		}
		if next < 0 {
			return cleanupCount, nil
		}
		if next <= cursor {
			return cleanupCount, repairFailureError(RepairFailureIndexInvalid, fmt.Errorf("repair member cursor did not advance"), false)
		}
		cursor = next
	}
}

func (s *ValkeyStatusStore) cleanupRepairMember(ctx context.Context, request RepairRequest, member string) (int64, error) {
	items, err := s.evalMutation(ctx,
		[]string{statusKey(request.Partition), indexKey(request.Partition), recordKey(request.Partition, member)},
		"cleanup_member", request.RegistrationUID, request.ExpectedRegistryRevision,
		request.WriterFence.HolderPodUID, request.WriterFence.HolderSessionUID,
		request.WriterFence.LeaseEpoch, request.WriterFence.Fence, request.Generation,
		request.Token.Value, member)
	if err != nil {
		return 0, err
	}
	count, err := scriptInt(items, 1)
	if err != nil {
		return 0, repairFailureError(RepairFailureIndexInvalid, err, false)
	}
	if count < 0 {
		return 0, repairFailureError(RepairFailureIndexInvalid, fmt.Errorf("cleanup count is negative"), false)
	}
	return count, nil
}

func (s *ValkeyStatusStore) MarkRepairUncertain(ctx context.Context, start RepairStartResult, failure RepairFailure) error {
	if err := start.Validate(); err != nil {
		return repairFailureError(RepairFailureInvalidRequest, err, false)
	}
	if failure.Code == RepairFailureNone {
		return repairFailureError(RepairFailureInvalidRequest, fmt.Errorf("repair failure code is required"), false)
	}
	items, err := s.evalMutation(ctx, []string{statusKey(start.Partition)},
		"mark_uncertain", start.RegistrationUID, start.ExpectedRegistryRevision,
		start.WriterFence.HolderPodUID, start.WriterFence.HolderSessionUID,
		start.WriterFence.LeaseEpoch, start.WriterFence.Fence, start.Generation,
		start.Token.Value, string(failure.Code), failure.Message)
	if err != nil {
		return classifyRepairError(err)
	}
	if len(items) == 0 || scriptString(items, 0) != "ok" {
		return repairFailureError(RepairFailureAmbiguousWrite, fmt.Errorf("mark repair uncertain was not acknowledged"), false)
	}
	return nil
}

func (s *ValkeyStatusStore) Repair(ctx context.Context, request RepairRequest) (RepairResult, error) {
	result := RepairResult{
		Partition:                request.Partition,
		RegistrationUID:          request.RegistrationUID,
		ExpectedRegistryRevision: request.ExpectedRegistryRevision,
		WriterFence:              request.WriterFence,
		ProvenRegistryRevision:   request.ExpectedRegistryRevision,
		ProvenGeneration:         request.Generation,
		ProvenWriterFence:        request.WriterFence,
		Token:                    request.Token,
	}
	if err := request.Validate(); err != nil {
		return failRepairResult(result, RepairFailureInvalidRequest, err, false)
	}
	result.ExpectedCount = len(request.Observations.Items)
	seen := make(map[string]struct{}, result.ExpectedCount)
	result.SourceComplete = request.Observations.Complete
	result.RecordValid = true
	result.IndexValid = true
	result.StatusValid = true
	result.RecordIndexChecksValid = true
	for _, observation := range request.Observations.Items {
		publicID, proof, err := s.repairObservation(ctx, request, observation)
		if err != nil {
			return failRepairResult(result, repairFailureCode(err), err, repairFailureRetryable(err))
		}
		if _, exists := seen[publicID]; exists {
			return failRepairResult(result, RepairFailureCountMismatch, fmt.Errorf("duplicate repair resource %q", publicID), false)
		}
		seen[publicID] = struct{}{}
		result.ObservedCount++
		result.RecordValid = result.RecordValid && proof.recordValid
		result.IndexValid = result.IndexValid && proof.indexValid
		result.StatusValid = result.StatusValid && proof.statusValid
		result.RecordIndexChecksValid = result.RecordIndexChecksValid && proof.recordIndexValid
		result.RecordTTL = proof.recordTTL
		result.IndexTTL = proof.indexTTL
		result.StatusTTL = proof.statusTTL
	}

	cleanupCount, err := s.cleanupRepairMembers(ctx, request, seen)
	if err != nil {
		return failRepairResult(result, repairFailureCode(err), err, repairFailureRetryable(err))
	}
	result.CleanupCount = cleanupCount
	result.CleanCommitEligible = hasCleanRepairProof(result)
	if !result.CleanCommitEligible {
		return failRepairResult(result, RepairFailureCommitRejected, fmt.Errorf("repair proof is incomplete"), false)
	}
	return result, nil
}

func hasCleanRepairProof(result RepairResult) bool {
	return result.SourceComplete && result.ExpectedCount == result.ObservedCount &&
		result.RecordValid && result.IndexValid && result.StatusValid && result.RecordIndexChecksValid &&
		result.RecordTTL.Valid && result.IndexTTL.Valid && result.StatusTTL.Valid
}

func (s *ValkeyStatusStore) CommitRepair(ctx context.Context, request RepairCommitRequest) (RepairCommitResult, error) {
	result := RepairCommitResult{
		Partition:                request.Partition,
		RegistrationUID:          request.RegistrationUID,
		ExpectedRegistryRevision: request.ExpectedRegistryRevision,
		WriterFence:              request.WriterFence,
		ProvenGeneration:         request.Generation,
		ProvenWriterFence:        request.WriterFence,
		Token:                    request.Token,
	}
	if err := request.Validate(); err != nil {
		result.Failure = RepairFailure{Code: RepairFailureInvalidRequest, Message: err.Error()}
		return result, repairFailureError(RepairFailureInvalidRequest, err, false)
	}
	if err := request.Proof.ValidateForCommit(request); err != nil {
		result.Failure = RepairFailure{Code: RepairFailureCommitRejected, Message: err.Error()}
		return result, repairFailureError(RepairFailureCommitRejected, err, false)
	}
	items, err := s.evalMutation(ctx, []string{statusKey(request.Partition), indexKey(request.Partition)},
		"commit", request.RegistrationUID, request.ExpectedRegistryRevision,
		request.WriterFence.HolderPodUID, request.WriterFence.HolderSessionUID,
		request.WriterFence.LeaseEpoch, request.WriterFence.Fence, request.Generation,
		request.Token.Value, request.ExpectedCount, request.Proof.ObservedCount,
		request.Proof.CleanupCount, boolArg(request.Proof.RecordValid), boolArg(request.Proof.IndexValid),
		boolArg(request.Proof.StatusValid), boolArg(request.Proof.RecordIndexChecksValid),
		boolArg(request.Proof.RecordTTL.Valid), boolArg(request.Proof.IndexTTL.Valid),
		boolArg(request.Proof.StatusTTL.Valid))
	if err != nil {
		result.Failure = repairFailureFromError(err)
		return result, classifyRepairError(err)
	}
	if len(items) == 0 || scriptString(items, 0) != "ok" {
		result.Failure = RepairFailure{Code: RepairFailureCommitRejected, Message: "commit was not acknowledged"}
		return result, repairFailureError(RepairFailureCommitRejected, fmt.Errorf("commit was not acknowledged"), false)
	}
	result.ProvenRegistryRevision = request.Proof.ProvenRegistryRevision
	result.Committed = true
	result.CleanCommit = true
	return result, nil
}

func (s *ValkeyStatusStore) ReadVerified(ctx context.Context, request FencedReadRequest) (FencedReadResult, error) {
	result := FencedReadResult{State: ReadUncertain}
	if err := request.Validate(); err != nil {
		return result, err
	}
	publicID, err := canonicalResourceID(request.Identity)
	if err != nil {
		return result, err
	}
	items, err := s.evalMutation(ctx, []string{statusKey(request.Partition), indexKey(request.Partition), recordKey(request.Partition, publicID)},
		"read_verified", request.RegistrationUID, request.ExpectedRegistryRevision,
		request.WriterFence.HolderPodUID, request.WriterFence.HolderSessionUID,
		request.WriterFence.LeaseEpoch, request.WriterFence.Fence, request.Generation, publicID)
	if err != nil {
		return result, classifyRepairError(err)
	}
	if len(items) == 0 || scriptString(items, 0) == "missing" {
		result.State = ReadMissing
		return result, nil
	}
	return parseReadVerified(items, request)
}

type readVerifiedProof struct {
	labelState    scope.LabelState
	now           int64
	expires       int64
	recordPTTL    int64
	statusExpires int64
	statusPTTL    int64
	indexExpires  int64
	indexPTTL     int64
	generation    int64
	fence         int64
}

func decodeReadVerified(items []interface{}) (readVerifiedProof, error) {
	if scriptString(items, 0) != "ok" {
		return readVerifiedProof{}, fmt.Errorf("invalid read_verified response")
	}
	if len(items) < 12 {
		return readVerifiedProof{}, fmt.Errorf("short read_verified response")
	}
	values := make([]int64, 9)
	labels := []string{"server time", "record expiry", "record pttl", "status expiry", "status pttl", "index expiry", "index pttl", "generation", "fence"}
	for index := range values {
		value, err := scriptInt(items, index+2)
		if err != nil {
			return readVerifiedProof{}, fmt.Errorf("invalid read %s: %w", labels[index], err)
		}
		values[index] = value
	}
	labelState := scope.LabelState(scriptString(items, 1))
	if labelState != scope.LabelStateTrue && labelState != scope.LabelStateFalse && labelState != scope.LabelStateAbsent {
		return readVerifiedProof{}, fmt.Errorf("invalid read label state %q", labelState)
	}
	return readVerifiedProof{
		labelState:    labelState,
		now:           values[0],
		expires:       values[1],
		recordPTTL:    values[2],
		statusExpires: values[3],
		statusPTTL:    values[4],
		indexExpires:  values[5],
		indexPTTL:     values[6],
		generation:    values[7],
		fence:         values[8],
	}, nil
}

func parseReadVerified(items []interface{}, request FencedReadRequest) (FencedReadResult, error) {
	result := FencedReadResult{State: ReadUncertain}
	proof, err := decodeReadVerified(items)
	if err != nil {
		return result, err
	}
	if proof.generation <= 0 || proof.fence <= 0 {
		return result, fmt.Errorf("read proof generation and fence must be positive")
	}
	result.Record = ResourceRecord{
		Identity:   request.Identity,
		LabelState: proof.labelState,
		Generation: uint64(proof.generation),
		ExpiresAt:  time.UnixMilli(proof.expires),
	}
	result.State = ReadVerified
	result.ProvenRegistryRevision = request.ExpectedRegistryRevision
	result.ProvenGeneration = uint64(proof.generation)
	result.ProvenWriterFence = request.WriterFence
	result.RecordValid = true
	result.IndexValid = true
	result.StatusValid = true
	result.RecordTTL = ttlValidation(proof.now, proof.expires, proof.recordPTTL)
	result.StatusTTL = ttlValidation(proof.now, proof.statusExpires, proof.statusPTTL)
	result.IndexTTL = ttlValidation(proof.now, proof.indexExpires, proof.indexPTTL)
	if !result.RecordTTL.Valid || !result.IndexTTL.Valid || !result.StatusTTL.Valid {
		result.State = ReadUncertain
		return result, repairFailureError(RepairFailureTTLInvalid, fmt.Errorf("read ttl proof is invalid"), false)
	}
	return result, nil
}

func (s *ValkeyStatusStore) evalMutation(ctx context.Context, keys []string, args ...interface{}) ([]interface{}, error) {
	if s == nil || s.scripts == nil {
		return nil, ErrScriptUnavailable
	}
	return s.scripts.eval(ctx, keys, args...)
}

func classifyScriptError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrScriptUnavailable) || errors.Is(err, ErrAmbiguousWrite) || errors.Is(err, ErrStaleFence) {
		return err
	}
	if strings.Contains(err.Error(), "STALE_FENCE") {
		return fmt.Errorf("%w: %w", ErrStaleFence, err)
	}
	if strings.Contains(err.Error(), "TTL_MISMATCH") || strings.Contains(err.Error(), "INVALID_PTTL") {
		return fmt.Errorf("%w: %w", ErrAmbiguousWrite, err)
	}
	return err
}

func classifyRepairError(err error) error {
	if err == nil {
		return nil
	}
	var repairErr RepairFailureError
	if errors.As(err, &repairErr) {
		return err
	}
	classified := classifyScriptError(err)
	return repairFailureError(repairFailureCode(classified), classified, repairFailureRetryable(classified))
}

func statusKey(partition PartitionStateKey) string {
	return partitionPrefix(partition) + ":status"
}

func indexKey(partition PartitionStateKey) string {
	return partitionPrefix(partition) + ":index"
}

func recordKey(partition PartitionStateKey, publicID string) string {
	return partitionPrefix(partition) + ":record:" + publicID
}

func partitionPrefix(partition PartitionStateKey) string {
	digest := sha256.Sum256([]byte(partition.ClusterID + "\x00" + partition.RegistrationUID))
	return "kubernaut:fmc:v2:" + hex.EncodeToString(digest[:])
}

func scriptString(items []interface{}, index int) string {
	if index >= len(items) || items[index] == nil {
		return ""
	}
	return fmt.Sprint(items[index])
}

func scriptInt(items []interface{}, index int) (int64, error) {
	value := scriptString(items, index)
	if value == "" {
		return 0, fmt.Errorf("script integer is empty")
	}
	return strconv.ParseInt(value, 10, 64)
}

func validateBeginRepairRequest(request BeginRepairRequest) error {
	if err := request.Partition.Validate(); err != nil {
		return err
	}
	if request.RegistrationUID == "" || request.RegistrationUID != request.Partition.RegistrationUID {
		return fmt.Errorf("repair begin registration identity is inconsistent")
	}
	if request.ExpectedRegistryRevision == 0 {
		return fmt.Errorf("repair begin registry revision is required")
	}
	return request.WriterFence.Validate()
}

func canonicalResourceID(identity ResourceIdentity) (string, error) {
	return inventory.CanonicalResourceIDChecked(scope.ResourceIdentity{
		ClusterID: identity.ClusterID,
		Group:     identity.Group,
		Version:   identity.Version,
		Kind:      identity.Kind,
		Namespace: identity.Namespace,
		Name:      identity.Name,
	})
}

func canonicalObservationID(observation RepairObservation) (string, error) {
	if err := observation.Identity.Validate(); err != nil {
		return "", err
	}
	return canonicalResourceID(observation.Identity)
}

type repairReconcileProof struct {
	recordValid      bool
	indexValid       bool
	statusValid      bool
	recordIndexValid bool
	recordTTL        TTLPTTLValidation
	indexTTL         TTLPTTLValidation
	statusTTL        TTLPTTLValidation
}

func parseRepairReconcile(items []interface{}) (repairReconcileProof, error) {
	if len(items) < 8 || scriptString(items, 0) != "ok" {
		return repairReconcileProof{}, fmt.Errorf("invalid repair reconcile response")
	}
	now, err := scriptInt(items, 1)
	if err != nil {
		return repairReconcileProof{}, fmt.Errorf("invalid repair server time: %w", err)
	}
	expires, err := scriptInt(items, 2)
	if err != nil {
		return repairReconcileProof{}, fmt.Errorf("invalid repair record expiry: %w", err)
	}
	recordPTTL, err := scriptInt(items, 3)
	if err != nil {
		return repairReconcileProof{}, fmt.Errorf("invalid repair record pttl: %w", err)
	}
	indexPTTL, err := scriptInt(items, 4)
	if err != nil {
		return repairReconcileProof{}, fmt.Errorf("invalid repair index pttl: %w", err)
	}
	statusPTTL, err := scriptInt(items, 5)
	if err != nil {
		return repairReconcileProof{}, fmt.Errorf("invalid repair status pttl: %w", err)
	}
	indexExpires, err := scriptInt(items, 6)
	if err != nil {
		return repairReconcileProof{}, fmt.Errorf("invalid repair index expiry: %w", err)
	}
	statusExpires, err := scriptInt(items, 7)
	if err != nil {
		return repairReconcileProof{}, fmt.Errorf("invalid repair status expiry: %w", err)
	}
	return repairReconcileProof{
		recordValid:      true,
		indexValid:       true,
		statusValid:      true,
		recordIndexValid: true,
		recordTTL:        ttlValidation(now, expires, recordPTTL),
		indexTTL:         ttlValidation(now, indexExpires, indexPTTL),
		statusTTL:        ttlValidation(now, statusExpires, statusPTTL),
	}, nil
}

func parseRepairMembers(items []interface{}) (int, []string, error) {
	if len(items) == 0 {
		return -1, nil, fmt.Errorf("empty repair member response")
	}
	next, err := scriptInt(items, 0)
	if err != nil {
		return -1, nil, fmt.Errorf("invalid repair member cursor: %w", err)
	}
	members := make([]string, 0, len(items)-1)
	for index := 1; index < len(items); index++ {
		member := scriptString(items, index)
		if member == "" {
			return -1, nil, fmt.Errorf("empty repair index member")
		}
		members = append(members, member)
	}
	return int(next), members, nil
}

func ttlValidation(serverNow, expiresAt, pttl int64) TTLPTTLValidation {
	delta := expiresAt - serverNow - pttl
	if delta < 0 {
		delta = -delta
	}
	return TTLPTTLValidation{
		ServerNowMillis: serverNow,
		ExpiresAtMillis: expiresAt,
		PTTLMillis:      pttl,
		DeltaMillis:     delta,
		Valid:           serverNow > 0 && expiresAt > serverNow && pttl > 0 && delta <= 2,
	}
}

func boolArg(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func repairFailureError(code RepairFailureCode, err error, retryable bool) error {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return RepairFailureError{Failure: RepairFailure{Code: code, Message: message, Retryable: retryable}}
}

func failRepairResult(result RepairResult, code RepairFailureCode, err error, retryable bool) (RepairResult, error) {
	result.Failure = RepairFailure{Code: code, Message: err.Error(), Retryable: retryable}
	result.CleanCommitEligible = false
	return result, repairFailureError(code, err, retryable)
}

func repairFailureFromError(err error) RepairFailure {
	return RepairFailure{Code: repairFailureCode(err), Message: err.Error(), Retryable: repairFailureRetryable(err)}
}

func repairFailureCode(err error) RepairFailureCode {
	if errors.Is(err, ErrScriptUnavailable) {
		return RepairFailureScriptUnavailable
	}
	if errors.Is(err, ErrAmbiguousWrite) {
		return RepairFailureAmbiguousWrite
	}
	if errors.Is(err, ErrStaleFence) {
		return RepairFailureStaleFence
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "ttl") || strings.Contains(message, "pttl"):
		return RepairFailureTTLInvalid
	case strings.Contains(message, "count"):
		return RepairFailureCountMismatch
	default:
		return RepairFailureCommitRejected
	}
}

func repairFailureRetryable(err error) bool {
	return errors.Is(err, ErrScriptUnavailable)
}

var _ StatusStore = (*ValkeyStatusStore)(nil)
