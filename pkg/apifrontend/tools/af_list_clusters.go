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

package tools

import (
	"context"
	"errors"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"

	"github.com/jordigilh/kubernaut/pkg/fleet/registry"
)

// ListClustersArgs defines the input for list_clusters.
// No arguments are required; the tool returns all known fleet clusters.
type ListClustersArgs struct{}

// ClusterSummary is a subset of registry.ClusterInfo exposed to the LLM.
// ID-only (issue #1651): cluster display names are non-unique and unsafe
// for disambiguation, so only the unique ClusterID is surfaced.
type ClusterSummary struct {
	ID string `json:"id"`
}

// ListClustersResult is the output of list_clusters.
type ListClustersResult struct {
	Clusters []ClusterSummary `json:"clusters"`
	Count    int              `json:"count"`
}

// HandleListClusters returns all managed clusters known to the fleet registry.
// BR-FLEET-054: Enables the LLM to discover available clusters and pass
// cluster_id to kubectl_get/kubectl_list for cross-cluster triage.
func HandleListClusters(_ context.Context, reg registry.ClusterRegistry) (ListClustersResult, error) {
	if reg == nil {
		return ListClustersResult{}, errors.New("fleet management is not configured")
	}
	clusters := reg.List()
	summaries := make([]ClusterSummary, 0, len(clusters))
	for _, c := range clusters {
		summaries = append(summaries, ClusterSummary{
			ID: c.ID,
		})
	}
	return ListClustersResult{
		Clusters: summaries,
		Count:    len(summaries),
	}, nil
}

// NewListClustersTool creates the list_clusters ADK tool.
// When reg is nil, the tool is not registered (fleet not configured).
func NewListClustersTool(reg registry.ClusterRegistry) (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "list_clusters",
		Description: "List all managed fleet clusters. Returns cluster IDs that can be passed as cluster_id to kubectl_get or kubectl_list for cross-cluster investigation.",
	}, func(_ tool.Context, _ ListClustersArgs) (ListClustersResult, error) {
		return HandleListClusters(context.Background(), reg)
	})
}
