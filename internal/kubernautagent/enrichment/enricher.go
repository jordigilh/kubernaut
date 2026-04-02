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

package enrichment

import (
	"context"
	"log/slog"

	"github.com/jordigilh/kubernaut/internal/kubernautagent/audit"
)

// K8sClient abstracts Kubernetes API access for enrichment.
type K8sClient interface {
	GetOwnerChain(ctx context.Context, kind, name, namespace string) ([]string, error)
}

// DataStorageClient abstracts DataStorage API access for enrichment.
type DataStorageClient interface {
	GetRemediationHistory(ctx context.Context, name, namespace string) ([]RemediationHistoryEntry, error)
}

// RemediationHistoryEntry is a single remediation history record.
type RemediationHistoryEntry struct {
	WorkflowID string `json:"workflow_id"`
	Outcome    string `json:"outcome"`
	Timestamp  string `json:"timestamp"`
}

// EnrichmentResult is the combined enrichment data.
type EnrichmentResult struct {
	OwnerChain         []string                  `json:"owner_chain"`
	RemediationHistory []RemediationHistoryEntry `json:"remediation_history"`
}

// Enricher resolves owner chain and remediation history.
type Enricher struct {
	k8s        K8sClient
	ds         DataStorageClient
	auditStore audit.AuditStore
	logger     *slog.Logger
}

// NewEnricher creates an enricher with the given clients.
func NewEnricher(k8s K8sClient, ds DataStorageClient, auditStore audit.AuditStore, logger *slog.Logger) *Enricher {
	return &Enricher{
		k8s:        k8s,
		ds:         ds,
		auditStore: auditStore,
		logger:     logger,
	}
}

// Enrich resolves enrichment data for the given resource.
// Implements partial failure: each sub-call is best-effort.
func (e *Enricher) Enrich(ctx context.Context, kind, name, namespace string) (*EnrichmentResult, error) {
	result := &EnrichmentResult{}

	var ownerErr, histErr error

	chain, err := e.k8s.GetOwnerChain(ctx, kind, name, namespace)
	if err != nil {
		ownerErr = err
		e.logger.Warn("enrichment: owner chain resolution failed",
			slog.String("resource", namespace+"/"+kind+"/"+name),
			slog.String("error", err.Error()),
		)
	} else {
		result.OwnerChain = chain
	}

	history, err := e.ds.GetRemediationHistory(ctx, name, namespace)
	if err != nil {
		histErr = err
		e.logger.Warn("enrichment: remediation history fetch failed",
			slog.String("resource", namespace+"/"+name),
			slog.String("error", err.Error()),
		)
	} else {
		result.RemediationHistory = history
	}

	if ownerErr != nil && histErr != nil {
		event := audit.NewEvent(audit.EventTypeEnrichmentFailed, "")
		event.Data["owner_error"] = ownerErr.Error()
		event.Data["history_error"] = histErr.Error()
		audit.StoreBestEffort(ctx, e.auditStore, event, e.logger)
	} else {
		event := audit.NewEvent(audit.EventTypeEnrichmentCompleted, "")
		if ownerErr != nil {
			event.Data["owner_error"] = ownerErr.Error()
		}
		if histErr != nil {
			event.Data["history_error"] = histErr.Error()
		}
		audit.StoreBestEffort(ctx, e.auditStore, event, e.logger)
	}

	return result, nil
}
