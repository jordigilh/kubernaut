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

package routing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	ogenclient "github.com/jordigilh/kubernaut/pkg/datastorage/ogen-client"
	"github.com/jordigilh/kubernaut/pkg/shared/auth"
)

// HistoryContextClient is a narrow interface for querying remediation history
// from DataStorage. Satisfied by *ogenclient.Client.
type HistoryContextClient interface {
	GetRemediationHistoryContext(
		ctx context.Context,
		params ogenclient.GetRemediationHistoryContextParams,
	) (ogenclient.GetRemediationHistoryContextRes, error)
}

// DSHistoryAdapter adapts the ogen-generated DataStorage client to the
// RemediationHistoryQuerier interface used by the routing engine.
// Issue #214: Enables CheckIneffectiveRemediationChain to query real DS data.
type DSHistoryAdapter struct {
	client HistoryContextClient
}

var _ RemediationHistoryQuerier = (*DSHistoryAdapter)(nil)

// NewDSHistoryAdapter creates a new adapter wrapping the given DS client.
// Panics if client is nil (programming error -- must be caught at startup).
func NewDSHistoryAdapter(client HistoryContextClient) *DSHistoryAdapter {
	if client == nil {
		panic("DSHistoryAdapter: client must not be nil")
	}
	return &DSHistoryAdapter{client: client}
}

// NewDSHistoryAdapterFromConfig creates a DSHistoryAdapter with a standalone
// ogen client configured from the DataStorage URL and timeout.
// Uses ServiceAccount auth transport (same pattern as audit.OpenAPIClientAdapter).
func NewDSHistoryAdapterFromConfig(baseURL string, timeout time.Duration) (*DSHistoryAdapter, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("DataStorage base URL cannot be empty")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	transport := auth.NewServiceAccountTransportWithBase(&http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	})

	ogenClient, err := ogenclient.NewClient(baseURL, ogenclient.WithClient(&http.Client{
		Timeout:   timeout,
		Transport: transport,
	}))
	if err != nil {
		return nil, fmt.Errorf("failed to create ogen client for DS history: %w", err)
	}

	return &DSHistoryAdapter{client: ogenClient}, nil
}

// GetRemediationHistory queries DataStorage for remediation history entries
// within the specified time window. Maps routing types to ogen params and
// extracts Tier1.Chain from the response.
func (a *DSHistoryAdapter) GetRemediationHistory(
	ctx context.Context,
	target TargetResource,
	currentSpecHash string,
	window time.Duration,
) ([]ogenclient.RemediationHistoryEntry, error) {
	params := ogenclient.GetRemediationHistoryContextParams{
		TargetKind:      target.Kind,
		TargetName:      target.Name,
		TargetNamespace: target.Namespace,
		CurrentSpecHash: currentSpecHash,
		Tier1Window: ogenclient.OptString{
			Value: window.String(),
			Set:   true,
		},
	}

	res, err := a.client.GetRemediationHistoryContext(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("DS history query failed for %s: %w", target, err)
	}

	historyCtx, ok := res.(*ogenclient.RemediationHistoryContext)
	if !ok {
		return nil, fmt.Errorf("unexpected response type %T from DS history query", res)
	}

	return historyCtx.Tier1.Chain, nil
}
