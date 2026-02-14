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

// Adapter that bridges the repository.RemediationHistoryRepository to the
// server.RemediationHistoryQuerier interface, converting EffectivenessEventRow
// (repository package) to EffectivenessEvent (server package).
//
// BR-HAPI-016: Remediation history context for LLM prompt enrichment.
// DD-HAPI-016 v1.1: Two-step query pattern with EM scoring infrastructure.
package server

import (
	"context"
	"time"

	"github.com/jordigilh/kubernaut/pkg/datastorage/repository"
)

// remediationHistoryRepoAdapter adapts repository.RemediationHistoryRepository
// to the RemediationHistoryQuerier interface. It converts
// repository.EffectivenessEventRow to server.EffectivenessEvent, which is
// required by the correlation functions (CorrelateTier1Chain, BuildTier2Summaries)
// and the shared EM scoring infrastructure (BuildEffectivenessResponse).
type remediationHistoryRepoAdapter struct {
	repo *repository.RemediationHistoryRepository
}

// NewRemediationHistoryRepoAdapter creates a new adapter wrapping the given repository.
func NewRemediationHistoryRepoAdapter(repo *repository.RemediationHistoryRepository) RemediationHistoryQuerier {
	return &remediationHistoryRepoAdapter{repo: repo}
}

func (a *remediationHistoryRepoAdapter) QueryROEventsByTarget(ctx context.Context, targetResource string, since time.Time) ([]repository.RawAuditRow, error) {
	return a.repo.QueryROEventsByTarget(ctx, targetResource, since)
}

func (a *remediationHistoryRepoAdapter) QueryEffectivenessEventsBatch(ctx context.Context, correlationIDs []string) (map[string][]*EffectivenessEvent, error) {
	rows, err := a.repo.QueryEffectivenessEventsBatch(ctx, correlationIDs)
	if err != nil {
		return nil, err
	}

	// Convert EffectivenessEventRow -> EffectivenessEvent
	result := make(map[string][]*EffectivenessEvent, len(rows))
	for cid, eventRows := range rows {
		events := make([]*EffectivenessEvent, len(eventRows))
		for i, row := range eventRows {
			events[i] = &EffectivenessEvent{
				EventData: row.EventData,
			}
		}
		result[cid] = events
	}
	return result, nil
}

func (a *remediationHistoryRepoAdapter) QueryROEventsBySpecHash(ctx context.Context, specHash string, since, until time.Time) ([]repository.RawAuditRow, error) {
	return a.repo.QueryROEventsBySpecHash(ctx, specHash, since, until)
}
