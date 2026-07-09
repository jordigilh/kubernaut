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

// Package registry implements the ClusterRegistry interface for multi-cluster
// federation. It discovers managed clusters by watching MCP Gateway CRDs
// labeled with kubernaut.ai/managed=true. Concrete implementations exist for
// Envoy AI Gateway (EAIGWRegistry) and Kuadrant (KuadrantRegistry).
package registry

import "context"

// ClusterInfo holds metadata about a discovered managed cluster.
type ClusterInfo struct {
	// ID is the unique identifier (CR name in the MCP Gateway).
	ID string
	// MCPEndpoint is the URL to reach the cluster's MCP server through the gateway.
	MCPEndpoint string
	// ToolPrefix is the prefix used by the MCP Gateway for this cluster's tools.
	// EAIGW uses "{name}__", Kuadrant reads it from MCPServerRegistration spec.prefix.
	ToolPrefix string
	// Labels from the MCP Gateway CRD resource.
	Labels map[string]string
	// Namespace where the MCP Gateway CRD was found.
	Namespace string
}

// ClusterEvent represents a change in the cluster registry.
type ClusterEvent struct {
	Type    EventType
	Cluster ClusterInfo
}

// EventType classifies cluster registry events.
type EventType string

const (
	EventAdded   EventType = "Added"
	EventUpdated EventType = "Updated"
	EventDeleted EventType = "Deleted"
)

// ToolPrefixResolver resolves the gateway-specific tool prefix for a cluster.
// Implementations typically wrap a ClusterRegistry and return ClusterInfo.ToolPrefix.
// When nil or returning empty, callers fall back to the EAIGW "{clusterID}__" convention.
type ToolPrefixResolver interface {
	ToolPrefixFor(clusterID string) string
}

// ClusterQuerier provides read-only access to discovered managed clusters.
// Split out from ClusterRegistry for ISP (GO-ANTIPATTERN-AUDIT-2026-07-01
// Wave 0b) — most consumers (tools, config resolvers) only ever query the
// registry and never manage its watcher lifecycle.
type ClusterQuerier interface {
	// List returns all currently known managed clusters.
	List() []ClusterInfo
	// Get returns info for a specific cluster by ID, or false if not found.
	Get(clusterID string) (ClusterInfo, bool)
	// WatchClusters returns a channel that receives cluster change events.
	// The channel is closed when the registry stops.
	WatchClusters() <-chan ClusterEvent
	// Ready reports whether the registry has completed at least one successful sync.
	Ready() bool
}

// ClusterRegistryLifecycle manages the registry's background watcher. Split
// out from ClusterRegistry for ISP (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0b)
// — only the owning binary (e.g. cmd/fleetmetadatacache) starts/stops the
// watcher; every other consumer only needs ClusterQuerier.
type ClusterRegistryLifecycle interface {
	// Start begins watching for MCP Gateway CRDs.
	Start(ctx context.Context) error
	// Stop halts the watcher and closes subscriber channels.
	Stop()
}

// ClusterRegistry provides access to discovered managed clusters and
// manages the underlying watcher lifecycle. Kept as a named union — rather
// than inlining the two interfaces at call sites — so existing implementers
// (EAIGWRegistry, KuadrantRegistry) and mocks (which already implement
// every method) need no changes (GO-ANTIPATTERN-AUDIT-2026-07-01 Wave 0b;
// see docs/architecture/audits for rationale).
type ClusterRegistry interface {
	ClusterQuerier
	ClusterRegistryLifecycle
}
