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

package registry

import "context"

type ClusterStatus string

const (
	ClusterStatusReady    ClusterStatus = "Ready"
	ClusterStatusDegraded ClusterStatus = "Degraded"
	ClusterStatusOffline  ClusterStatus = "Offline"
)

type ClusterInfo struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	MCPEndpoint string            `json:"mcpEndpoint"`
	JWTAudience string            `json:"jwtAudience,omitempty"`
	ToolPrefix  string            `json:"toolPrefix"`
	Labels      map[string]string `json:"labels,omitempty"`
	Status      ClusterStatus     `json:"status"`
}

type ClusterEventType string

const (
	ClusterAdded   ClusterEventType = "Added"
	ClusterUpdated ClusterEventType = "Updated"
	ClusterRemoved ClusterEventType = "Removed"
)

type ClusterEvent struct {
	Type    ClusterEventType
	Cluster ClusterInfo
}

type ClusterRegistry interface {
	ListClusters(ctx context.Context) ([]ClusterInfo, error)
	GetCluster(ctx context.Context, id string) (ClusterInfo, error)
	WatchClusters(ctx context.Context) (<-chan ClusterEvent, error)
}
