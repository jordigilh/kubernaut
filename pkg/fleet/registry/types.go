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
// federation. It discovers managed clusters by watching Envoy AI Gateway Backend
// CRDs labeled with kubernaut.ai/managed=true. Each Backend represents a managed
// cluster's K8s MCP Server endpoint behind the MCP Gateway.
package registry

import "context"

// ClusterInfo holds metadata about a discovered managed cluster.
type ClusterInfo struct {
	// ID is the unique identifier (Backend CR name in Envoy AI Gateway).
	ID string
	// Name is the human-readable cluster display name.
	Name string
	// MCPEndpoint is the URL to reach the cluster's MCP server through the gateway.
	MCPEndpoint string
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

// ClusterRegistry provides access to discovered managed clusters.
type ClusterRegistry interface {
	// List returns all currently known managed clusters.
	List() []ClusterInfo
	// Get returns info for a specific cluster by ID, or false if not found.
	Get(clusterID string) (ClusterInfo, bool)
	// WatchClusters returns a channel that receives cluster change events.
	// The channel is closed when the registry stops.
	WatchClusters() <-chan ClusterEvent
	// Ready reports whether the registry has completed at least one successful sync.
	Ready() bool
	// Start begins watching for Envoy AI Gateway Backend CRDs.
	Start(ctx context.Context) error
	// Stop halts the watcher and closes subscriber channels.
	Stop()
}
