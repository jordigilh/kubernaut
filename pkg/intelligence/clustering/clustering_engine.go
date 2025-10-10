<<<<<<< HEAD
=======
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

>>>>>>> crd_implementation
package clustering

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/jordigilh/kubernaut/pkg/intelligence/patterns"
	"github.com/jordigilh/kubernaut/pkg/workflow/engine"
	"github.com/sirupsen/logrus"
)

// ClusteringEngine performs clustering analysis for pattern discovery
type ClusteringEngine struct {
	config *patterns.PatternDiscoveryConfig
	log    *logrus.Logger
}

// ClusteringResult contains results of clustering analysis
type ClusteringResult struct {
	Clusters       []*Cluster             `json:"clusters"`
	Algorithm      string                 `json:"algorithm"`
	Parameters     map[string]interface{} `json:"parameters"`
	Silhouette     float64                `json:"silhouette_score"`
	Inertia        float64                `json:"inertia"`
	OptimalK       int                    `json:"optimal_k"`
	AnalyzedPoints int                    `json:"analyzed_points"`
}

// Cluster represents a cluster of similar data points
type Cluster struct {
	ID              int                    `json:"id"`
	Centroid        []float64              `json:"centroid"`
	Members         []*ClusterMember       `json:"members"`
	Size            int                    `json:"size"`
	Cohesion        float64                `json:"cohesion"`   // Within-cluster similarity
	Separation      float64                `json:"separation"` // Distance to other clusters
	Label           string                 `json:"label"`      // Human-readable cluster description
	Characteristics map[string]interface{} `json:"characteristics"`
}

// ClusterMember represents a member of a cluster
type ClusterMember struct {
	ID       string                 `json:"id"`
	Features []float64              `json:"features"`
	Distance float64                `json:"distance_to_centroid"`
	Metadata map[string]interface{} `json:"metadata"`
}

// AlertCluster contains alert-specific clustering results
type AlertCluster struct {
	*Cluster
	CommonAlertTypes   []string          `json:"common_alert_types"`
	CommonNamespaces   []string          `json:"common_namespaces"`
	CommonResources    []string          `json:"common_resources"`
	TemporalPattern    string            `json:"temporal_pattern"`
	AverageSuccessRate float64           `json:"average_success_rate"`
	CommonLabels       map[string]string `json:"common_labels"`
}

// NewClusteringEngine creates a new clustering engine
func NewClusteringEngine(config *patterns.PatternDiscoveryConfig, log *logrus.Logger) *ClusteringEngine {
	return &ClusteringEngine{
		config: config,
		log:    log,
	}
}

// ClusterAlerts clusters workflow executions based on alert characteristics
func (ce *ClusteringEngine) ClusterAlerts(data []*engine.EngineWorkflowExecutionData) []*AlertClusterGroup {
	ce.log.WithField("data_points", len(data)).Info("Clustering alerts")

	if len(data) < ce.config.MinClusterSize {
		return []*AlertClusterGroup{}
	}

	// Extract features for clustering
	features := make([][]float64, 0)
	metadata := make([]map[string]interface{}, 0)

	for _, execution := range data {
		featureVec := ce.extractAlertFeatures(execution)
		if featureVec != nil {
			features = append(features, featureVec)
			// Extract alert info from metadata if available
			alertName := ""
			namespace := ""
			resource := ""
			if execution.Metadata != nil {
				if name, exists := execution.Metadata["alert_name"]; exists {
					alertName = fmt.Sprintf("%v", name)
				}
				if ns, exists := execution.Metadata["namespace"]; exists {
					namespace = fmt.Sprintf("%v", ns)
				}
				if res, exists := execution.Metadata["resource"]; exists {
					resource = fmt.Sprintf("%v", res)
				}
			}

			metadata = append(metadata, map[string]interface{}{
				"execution_id": execution.ExecutionID,
				"alert_name":   alertName,
				"namespace":    namespace,
				"resource":     resource,
				"timestamp":    execution.Timestamp,
				"success":      execution.Success,
			})
		}
	}

	if len(features) < ce.config.MinClusterSize {
		return []*AlertClusterGroup{}
	}

	// Perform clustering
	result := ce.performKMeansClustering(features, metadata)

	// Convert to alert-specific clusters
	alertClusters := ce.convertToAlertClusters(result, data)

	return alertClusters
}

// ClusterExecutionPatterns clusters executions based on execution characteristics
func (ce *ClusteringEngine) ClusterExecutionPatterns(data []*engine.EngineWorkflowExecutionData) (*ClusteringResult, error) {
	ce.log.WithField("data_points", len(data)).Info("Clustering execution patterns")

	if len(data) < ce.config.MinClusterSize {
		return nil, fmt.Errorf("insufficient data for clustering: %d points", len(data))
	}

	// Extract execution features
	features := make([][]float64, 0)
	metadata := make([]map[string]interface{}, 0)

	for _, execution := range data {
		featureVec := ce.extractExecutionFeatures(execution)
		if featureVec != nil {
			features = append(features, featureVec)
			metadata = append(metadata, map[string]interface{}{
				"execution_id": execution.ExecutionID,
				"template_id":  execution.WorkflowID,
				"timestamp":    execution.Timestamp,
				"duration":     execution.Duration.Seconds(),
				"success":      execution.Success,
			})
		}
	}

	if len(features) < ce.config.MinClusterSize {
		return nil, fmt.Errorf("insufficient valid features: %d points", len(features))
	}

	// Perform clustering
	result := ce.performKMeansClustering(features, metadata)

	// Enhance with execution-specific analysis
	ce.analyzeExecutionClusters(result)

	return result, nil
}

// ClusterResourceUsage clusters executions based on resource usage patterns
func (ce *ClusteringEngine) ClusterResourceUsage(data []*engine.EngineWorkflowExecutionData) (*ClusteringResult, error) {
	ce.log.WithField("data_points", len(data)).Info("Clustering resource usage patterns")

	// Filter data with resource usage information
	validData := make([]*engine.EngineWorkflowExecutionData, 0)
	for _, execution := range data {
		// Check if execution has resource metrics
		if execution.Metrics != nil {
			hasResource := false
			for key := range execution.Metrics {
				if key == "cpu_usage" || key == "memory_usage" || key == "network_usage" || key == "storage_usage" {
					hasResource = true
					break
				}
			}
			if hasResource {
				validData = append(validData, execution)
			}
		}
	}

	if len(validData) < ce.config.MinClusterSize {
		return nil, fmt.Errorf("insufficient resource usage data: %d points", len(validData))
	}

	// Extract resource usage features
	features := make([][]float64, 0)
	metadata := make([]map[string]interface{}, 0)

	for _, execution := range validData {
		featureVec := ce.extractResourceFeatures(execution)
		features = append(features, featureVec)
		cpuUsage := 0.0
		memoryUsage := 0.0
		networkUsage := 0.0
		storageUsage := 0.0
		if execution.Metrics != nil {
			if cpu, exists := execution.Metrics["cpu_usage"]; exists {
				cpuUsage = cpu
			}
			if memory, exists := execution.Metrics["memory_usage"]; exists {
				memoryUsage = memory
			}
			if network, exists := execution.Metrics["network_usage"]; exists {
				networkUsage = network
			}
			if storage, exists := execution.Metrics["storage_usage"]; exists {
				storageUsage = storage
			}
		}

		metadata = append(metadata, map[string]interface{}{
			"execution_id":  execution.ExecutionID,
			"cpu_usage":     cpuUsage,
			"memory_usage":  memoryUsage,
			"network_usage": networkUsage,
			"storage_usage": storageUsage,
		})
	}

	// Perform clustering
	result := ce.performKMeansClustering(features, metadata)

	// Enhance with resource-specific analysis
	ce.analyzeResourceClusters(result)

	return result, nil
}

// PerformDBSCANClustering performs density-based clustering
func (ce *ClusteringEngine) PerformDBSCANClustering(features [][]float64, metadata []map[string]interface{}) (*ClusteringResult, error) {
	ce.log.WithFields(logrus.Fields{
		"algorithm":   "DBSCAN",
		"data_points": len(features),
		"epsilon":     ce.config.ClusteringEpsilon,
		"min_samples": ce.config.MinClusterSize,
	}).Info("Performing DBSCAN clustering")

	if len(features) < ce.config.MinClusterSize {
		return nil, fmt.Errorf("insufficient data for clustering: %d points", len(features))
	}

	clusters := make([]*Cluster, 0)
	visited := make([]bool, len(features))
	clusterLabels := make([]int, len(features))

	// Initialize all points as noise (-1)
	for i := range clusterLabels {
		clusterLabels[i] = -1
	}

	clusterID := 0

	for i := 0; i < len(features); i++ {
		if visited[i] {
			continue
		}

		visited[i] = true
		neighbors := ce.getNeighbors(features, i, ce.config.ClusteringEpsilon)

		if len(neighbors) < ce.config.MinClusterSize {
			// Point is noise
			continue
		}

		// Start new cluster
		clusterLabels[i] = clusterID
		cluster := &Cluster{
			ID:              clusterID,
			Members:         make([]*ClusterMember, 0),
			Characteristics: make(map[string]interface{}),
		}

		// Expand cluster
		j := 0
		for j < len(neighbors) {
			neighbor := neighbors[j]

			if !visited[neighbor] {
				visited[neighbor] = true
				newNeighbors := ce.getNeighbors(features, neighbor, ce.config.ClusteringEpsilon)

				if len(newNeighbors) >= ce.config.MinClusterSize {
					neighbors = append(neighbors, newNeighbors...)
				}
			}

			if clusterLabels[neighbor] == -1 {
				clusterLabels[neighbor] = clusterID
			}

			j++
		}

		// Populate cluster members
		for _, pointIdx := range neighbors {
			if clusterLabels[pointIdx] == clusterID {
				member := &ClusterMember{
					ID:       fmt.Sprintf("point_%d", pointIdx),
					Features: features[pointIdx],
					Distance: ce.euclideanDistance(features[i], features[pointIdx]),
					Metadata: metadata[pointIdx],
				}
				cluster.Members = append(cluster.Members, member)
			}
		}

		cluster.Size = len(cluster.Members)
		cluster.Centroid = ce.calculateCentroid(cluster.Members)
		cluster.Cohesion = ce.calculateCohesion(cluster.Members)

		clusters = append(clusters, cluster)
		clusterID++
	}

	// Calculate cluster separation
	for i, cluster := range clusters {
		cluster.Separation = ce.calculateSeparation(cluster, clusters)
		cluster.Label = fmt.Sprintf("Cluster_%d", i)
	}

	result := &ClusteringResult{
		Clusters:  clusters,
		Algorithm: "DBSCAN",
		Parameters: map[string]interface{}{
			"epsilon":     ce.config.ClusteringEpsilon,
			"min_samples": ce.config.MinClusterSize,
		},
		OptimalK:       len(clusters),
		AnalyzedPoints: len(features),
		Silhouette:     ce.calculateSilhouetteScore(clusters),
	}

	return result, nil
}

// Private helper methods

func (ce *ClusteringEngine) extractAlertFeatures(execution *engine.EngineWorkflowExecutionData) []float64 {
	// Extract alert info from metadata
	alertName := ""
	alertSeverity := ""
	alertResource := ""
	labelCount := 0

	if execution.Metadata != nil {
		if name, exists := execution.Metadata["alert_name"]; exists {
			alertName = fmt.Sprintf("%v", name)
		}
		if severity, exists := execution.Metadata["alert_severity"]; exists {
			alertSeverity = fmt.Sprintf("%v", severity)
		}
		if resource, exists := execution.Metadata["resource"]; exists {
			alertResource = fmt.Sprintf("%v", resource)
		}
		if labels, exists := execution.Metadata["labels"]; exists {
			if labelMap, ok := labels.(map[string]interface{}); ok {
				labelCount = len(labelMap)
			}
		}
	}

	if alertName == "" {
		return nil
	}

	features := make([]float64, 0)

	// Alert type encoding (simplified)
	alertTypeScore := ce.encodeAlertType(alertName)
	features = append(features, alertTypeScore)

	// Severity encoding
	severityScore := ce.encodeSeverity(alertSeverity)
	features = append(features, severityScore)

	// Resource type encoding
	resourceScore := ce.encodeResourceType(alertResource)
	features = append(features, resourceScore)

	// Temporal features
	hour := float64(execution.Timestamp.Hour()) / 24.0
	dayOfWeek := float64(execution.Timestamp.Weekday()) / 7.0
	features = append(features, hour, dayOfWeek)

	// Success indicator
	successScore := 0.0
	if execution.Success {
		successScore = 1.0
	}
	features = append(features, successScore)

	// Duration (normalized)
	durationScore := math.Min(execution.Duration.Minutes()/60.0, 1.0)
	features = append(features, durationScore)

	// Label count
	labelCountNorm := float64(labelCount) / 10.0 // Normalize assuming max 10 labels
	features = append(features, labelCountNorm)

	return features
}

func (ce *ClusteringEngine) extractExecutionFeatures(execution *engine.EngineWorkflowExecutionData) []float64 {
	features := make([]float64, 0)

	// Execution success
	successScore := 0.0
	if execution.Success {
		successScore = 1.0
	}
	features = append(features, successScore)

	// Duration features
	durationMinutes := execution.Duration.Minutes()
	stepsCompleted := 0.0
	if execution.Metadata != nil {
		if steps, exists := execution.Metadata["steps_completed"]; exists {
			if stepsInt, ok := steps.(int); ok {
				stepsCompleted = float64(stepsInt)
			} else if stepsFloat, ok := steps.(float64); ok {
				stepsCompleted = stepsFloat
			}
		}
	}
	features = append(features, durationMinutes/60.0, stepsCompleted/20.0) // Normalize

	// Temporal features
	hour := float64(execution.Timestamp.Hour()) / 24.0
	dayOfWeek := float64(execution.Timestamp.Weekday()) / 7.0
	features = append(features, hour, dayOfWeek)

	// Resource usage if available
	cpuUsage := 0.0
	memoryUsage := 0.0
	networkUsage := 0.0
	storageUsage := 0.0
	if execution.Metrics != nil {
		if cpu, exists := execution.Metrics["cpu_usage"]; exists {
			cpuUsage = cpu
		}
		if memory, exists := execution.Metrics["memory_usage"]; exists {
			memoryUsage = memory
		}
		if network, exists := execution.Metrics["network_usage"]; exists {
			networkUsage = network
		}
		if storage, exists := execution.Metrics["storage_usage"]; exists {
			storageUsage = storage
		}
	}
	features = append(features, cpuUsage, memoryUsage, networkUsage, storageUsage)

	// Alert characteristics from metadata
	alertTypeScore := 0.0
	severityScore := 0.0
	if execution.Metadata != nil {
		if alertName, exists := execution.Metadata["alert_name"]; exists {
			alertTypeScore = ce.encodeAlertType(fmt.Sprintf("%v", alertName))
		}
		if alertSeverity, exists := execution.Metadata["alert_severity"]; exists {
			severityScore = ce.encodeSeverity(fmt.Sprintf("%v", alertSeverity))
		}
	}
	features = append(features, alertTypeScore, severityScore)

	return features
}

func (ce *ClusteringEngine) extractResourceFeatures(execution *engine.EngineWorkflowExecutionData) []float64 {
	// Extract resource usage from metrics
	cpuUsage := 0.0
	memoryUsage := 0.0
	networkUsage := 0.0
	storageUsage := 0.0
	hasResourceData := false

	if execution.Metrics != nil {
		if cpu, exists := execution.Metrics["cpu_usage"]; exists {
			cpuUsage = cpu
			hasResourceData = true
		}
		if memory, exists := execution.Metrics["memory_usage"]; exists {
			memoryUsage = memory
			hasResourceData = true
		}
		if network, exists := execution.Metrics["network_usage"]; exists {
			networkUsage = network
			hasResourceData = true
		}
		if storage, exists := execution.Metrics["storage_usage"]; exists {
			storageUsage = storage
			hasResourceData = true
		}
	}

	if !hasResourceData {
		return nil
	}

	features := []float64{
		cpuUsage,
		memoryUsage,
		networkUsage,
		storageUsage,
	}

	// Add execution context
	durationHours := execution.Duration.Hours()
	features = append(features, durationHours)

	// Add temporal context
	hour := float64(execution.Timestamp.Hour()) / 24.0
	dayOfWeek := float64(execution.Timestamp.Weekday()) / 7.0
	features = append(features, hour, dayOfWeek)

	return features
}

func (ce *ClusteringEngine) performKMeansClustering(features [][]float64, metadata []map[string]interface{}) *ClusteringResult {
	// Determine optimal number of clusters using elbow method
	optimalK := ce.findOptimalK(features)

	ce.log.WithField("optimal_k", optimalK).Info("Found optimal number of clusters")

	// Perform K-means with optimal K
	clusters := ce.kMeans(features, metadata, optimalK)

	// Calculate metrics
	silhouette := ce.calculateSilhouetteScore(clusters)
	inertia := ce.calculateInertia(clusters)

	result := &ClusteringResult{
		Clusters:  clusters,
		Algorithm: "K-Means",
		Parameters: map[string]interface{}{
			"k": optimalK,
		},
		Silhouette:     silhouette,
		Inertia:        inertia,
		OptimalK:       optimalK,
		AnalyzedPoints: len(features),
	}

	return result
}

func (ce *ClusteringEngine) findOptimalK(features [][]float64) int {
	maxK := int(math.Min(float64(len(features)/ce.config.MinClusterSize), 10))
	if maxK < 2 {
		return 2
	}

	inertias := make([]float64, 0)

	for k := 2; k <= maxK; k++ {
		clusters := ce.kMeans(features, nil, k)
		inertia := ce.calculateInertia(clusters)
		inertias = append(inertias, inertia)
	}

	// Find elbow point (simplified)
	optimalK := 2
	if len(inertias) > 1 {
		maxImprovement := 0.0
		for i := 1; i < len(inertias); i++ {
			improvement := inertias[i-1] - inertias[i]
			if improvement > maxImprovement {
				maxImprovement = improvement
				optimalK = i + 2
			}
		}
	}

	return optimalK
}

func (ce *ClusteringEngine) kMeans(features [][]float64, metadata []map[string]interface{}, k int) []*Cluster {
	if len(features) == 0 {
		return []*Cluster{}
	}

	numFeatures := len(features[0])

	// Initialize centroids randomly
	centroids := make([][]float64, k)
	for i := 0; i < k; i++ {
		centroids[i] = make([]float64, numFeatures)
		// Use random points as initial centroids
		randomPoint := features[i%len(features)]
		copy(centroids[i], randomPoint)
	}

	// K-means iterations
	maxIterations := 100
	assignments := make([]int, len(features))

	for iter := 0; iter < maxIterations; iter++ {
		changed := false

		// Assign points to nearest centroid
		for i, feature := range features {
			minDist := math.Inf(1)
			bestCluster := 0

			for j, centroid := range centroids {
				dist := ce.euclideanDistance(feature, centroid)
				if dist < minDist {
					minDist = dist
					bestCluster = j
				}
			}

			if assignments[i] != bestCluster {
				changed = true
				assignments[i] = bestCluster
			}
		}

		if !changed {
			break
		}

		// Update centroids
		newCentroids := make([][]float64, k)
		clusterCounts := make([]int, k)

		for i := 0; i < k; i++ {
			newCentroids[i] = make([]float64, numFeatures)
		}

		for i, feature := range features {
			cluster := assignments[i]
			clusterCounts[cluster]++
			for j, val := range feature {
				newCentroids[cluster][j] += val
			}
		}

		for i := 0; i < k; i++ {
			if clusterCounts[i] > 0 {
				for j := 0; j < numFeatures; j++ {
					newCentroids[i][j] /= float64(clusterCounts[i])
				}
				centroids[i] = newCentroids[i]
			}
		}
	}

	// Create cluster objects
	clusters := make([]*Cluster, k)
	for i := 0; i < k; i++ {
		clusters[i] = &Cluster{
			ID:              i,
			Centroid:        centroids[i],
			Members:         make([]*ClusterMember, 0),
			Characteristics: make(map[string]interface{}),
		}
	}

	// Populate cluster members
	for i, feature := range features {
		cluster := assignments[i]
		member := &ClusterMember{
			ID:       fmt.Sprintf("point_%d", i),
			Features: feature,
			Distance: ce.euclideanDistance(feature, centroids[cluster]),
		}

		if metadata != nil && i < len(metadata) {
			member.Metadata = metadata[i]
		}

		clusters[cluster].Members = append(clusters[cluster].Members, member)
	}

	// Calculate cluster statistics
	for i, cluster := range clusters {
		cluster.Size = len(cluster.Members)
		cluster.Cohesion = ce.calculateCohesion(cluster.Members)
		cluster.Separation = ce.calculateSeparation(cluster, clusters)
		cluster.Label = fmt.Sprintf("Cluster_%d", i)
	}

	return clusters
}

func (ce *ClusteringEngine) getNeighbors(features [][]float64, pointIdx int, epsilon float64) []int {
	neighbors := make([]int, 0)
	point := features[pointIdx]

	for i, other := range features {
		if i != pointIdx {
			distance := ce.euclideanDistance(point, other)
			if distance <= epsilon {
				neighbors = append(neighbors, i)
			}
		}
	}

	return neighbors
}

func (ce *ClusteringEngine) euclideanDistance(a, b []float64) float64 {
	if len(a) != len(b) {
		return math.Inf(1)
	}

	sum := 0.0
	for i := 0; i < len(a); i++ {
		diff := a[i] - b[i]
		sum += diff * diff
	}
	return math.Sqrt(sum)
}

func (ce *ClusteringEngine) calculateCentroid(members []*ClusterMember) []float64 {
	if len(members) == 0 {
		return []float64{}
	}

	numFeatures := len(members[0].Features)
	centroid := make([]float64, numFeatures)

	for _, member := range members {
		for i, feature := range member.Features {
			centroid[i] += feature
		}
	}

	for i := range centroid {
		centroid[i] /= float64(len(members))
	}

	return centroid
}

func (ce *ClusteringEngine) calculateCohesion(members []*ClusterMember) float64 {
	if len(members) <= 1 {
		return 1.0
	}

	// Calculate average distance between all pairs of points in cluster
	totalDistance := 0.0
	count := 0

	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			distance := ce.euclideanDistance(members[i].Features, members[j].Features)
			totalDistance += distance
			count++
		}
	}

	if count == 0 {
		return 1.0
	}

	avgDistance := totalDistance / float64(count)
	// Convert to cohesion score (lower distance = higher cohesion)
	cohesion := 1.0 / (1.0 + avgDistance)
	return cohesion
}

func (ce *ClusteringEngine) calculateSeparation(cluster *Cluster, allClusters []*Cluster) float64 {
	if len(allClusters) <= 1 {
		return 1.0
	}

	minDistance := math.Inf(1)

	for _, other := range allClusters {
		if other.ID != cluster.ID {
			distance := ce.euclideanDistance(cluster.Centroid, other.Centroid)
			if distance < minDistance {
				minDistance = distance
			}
		}
	}

	return minDistance
}

func (ce *ClusteringEngine) calculateSilhouetteScore(clusters []*Cluster) float64 {
	if len(clusters) <= 1 {
		return 0.0
	}

	totalScore := 0.0
	totalPoints := 0

	for _, cluster := range clusters {
		for _, member := range cluster.Members {
			// Calculate average distance to points in same cluster (a)
			a := ce.calculateIntraClusterDistance(member, cluster)

			// Calculate minimum average distance to points in other clusters (b)
			b := ce.calculateMinInterClusterDistance(member, clusters, cluster.ID)

			// Calculate silhouette score for this point
			var silhouette float64
			if math.Max(a, b) == 0 {
				silhouette = 0
			} else {
				silhouette = (b - a) / math.Max(a, b)
			}

			totalScore += silhouette
			totalPoints++
		}
	}

	if totalPoints == 0 {
		return 0.0
	}

	return totalScore / float64(totalPoints)
}

func (ce *ClusteringEngine) calculateInertia(clusters []*Cluster) float64 {
	totalInertia := 0.0

	for _, cluster := range clusters {
		for _, member := range cluster.Members {
			distance := ce.euclideanDistance(member.Features, cluster.Centroid)
			totalInertia += distance * distance
		}
	}

	return totalInertia
}

func (ce *ClusteringEngine) calculateIntraClusterDistance(member *ClusterMember, cluster *Cluster) float64 {
	if len(cluster.Members) <= 1 {
		return 0.0
	}

	totalDistance := 0.0
	count := 0

	for _, other := range cluster.Members {
		if other.ID != member.ID {
			distance := ce.euclideanDistance(member.Features, other.Features)
			totalDistance += distance
			count++
		}
	}

	if count == 0 {
		return 0.0
	}

	return totalDistance / float64(count)
}

func (ce *ClusteringEngine) calculateMinInterClusterDistance(member *ClusterMember, allClusters []*Cluster, excludeClusterID int) float64 {
	minAvgDistance := math.Inf(1)

	for _, cluster := range allClusters {
		if cluster.ID != excludeClusterID {
			totalDistance := 0.0
			count := 0

			for _, other := range cluster.Members {
				distance := ce.euclideanDistance(member.Features, other.Features)
				totalDistance += distance
				count++
			}

			if count > 0 {
				avgDistance := totalDistance / float64(count)
				if avgDistance < minAvgDistance {
					minAvgDistance = avgDistance
				}
			}
		}
	}

	return minAvgDistance
}

func (ce *ClusteringEngine) convertToAlertClusters(result *ClusteringResult, data []*engine.EngineWorkflowExecutionData) []*AlertClusterGroup {
	alertClusters := make([]*AlertClusterGroup, 0)

	for _, cluster := range result.Clusters {
		if cluster.Size < ce.config.MinClusterSize {
			continue
		}

		alertCluster := &AlertClusterGroup{
			Members:               make([]*engine.EngineWorkflowExecutionData, 0),
			CommonCharacteristics: "",
			AlertTypes:            make([]string, 0),
			Namespaces:            make([]string, 0),
			Resources:             make([]string, 0),
			CommonLabels:          make(map[string]string),
			Confidence:            cluster.Cohesion,
		}

		// Extract alert characteristics from cluster members
		alertTypeCount := make(map[string]int)
		namespaceCount := make(map[string]int)
		resourceCount := make(map[string]int)
		successCount := 0

		for _, member := range cluster.Members {
			if execID, ok := member.Metadata["execution_id"].(string); ok {
				// Find corresponding execution data
				for _, execution := range data {
					if execution.ExecutionID == execID {
						alertCluster.Members = append(alertCluster.Members, execution)

						// Extract alert info from metadata
						if execution.Metadata != nil {
							if alertName, exists := execution.Metadata["alert_name"]; exists {
								alertTypeCount[fmt.Sprintf("%v", alertName)]++
							}
							if namespace, exists := execution.Metadata["namespace"]; exists {
								namespaceCount[fmt.Sprintf("%v", namespace)]++
							}
							if resource, exists := execution.Metadata["resource"]; exists {
								resourceCount[fmt.Sprintf("%v", resource)]++
							}
						}

						if execution.Success {
							successCount++
						}
						break
					}
				}
			}
		}

		// Calculate success rate
		if len(alertCluster.Members) > 0 {
			alertCluster.SuccessRate = float64(successCount) / float64(len(alertCluster.Members))
		}

		// Extract common characteristics
		alertCluster.AlertTypes = ce.getTopKeys(alertTypeCount, 3)
		alertCluster.Namespaces = ce.getTopKeys(namespaceCount, 3)
		alertCluster.Resources = ce.getTopKeys(resourceCount, 3)

		// Generate description
		if len(alertCluster.AlertTypes) > 0 {
			alertCluster.CommonCharacteristics = fmt.Sprintf("Alert types: %v", alertCluster.AlertTypes)
		}

		alertClusters = append(alertClusters, alertCluster)
	}

	return alertClusters
}

func (ce *ClusteringEngine) analyzeExecutionClusters(result *ClusteringResult) {
	for _, cluster := range result.Clusters {
		characteristics := make(map[string]interface{})

		// Analyze success rates
		successCount := 0
		totalDuration := 0.0
		durationCount := 0

		for _, member := range cluster.Members {
			if success, ok := member.Metadata["success"].(bool); ok && success {
				successCount++
			}
			if duration, ok := member.Metadata["duration"].(float64); ok {
				totalDuration += duration
				durationCount++
			}
		}

		if cluster.Size > 0 {
			characteristics["success_rate"] = float64(successCount) / float64(cluster.Size)
		}
		if durationCount > 0 {
			characteristics["average_duration"] = totalDuration / float64(durationCount)
		}

		cluster.Characteristics = characteristics

		// Generate cluster label based on characteristics
		if successRate, ok := characteristics["success_rate"].(float64); ok {
			if successRate > 0.8 {
				cluster.Label = "High Success Rate Cluster"
			} else if successRate < 0.5 {
				cluster.Label = "Low Success Rate Cluster"
			} else {
				cluster.Label = "Mixed Success Rate Cluster"
			}
		}
	}
}

func (ce *ClusteringEngine) analyzeResourceClusters(result *ClusteringResult) {
	for _, cluster := range result.Clusters {
		characteristics := make(map[string]interface{})

		// Analyze resource usage patterns
		totalCPU := 0.0
		totalMemory := 0.0
		totalNetwork := 0.0
		totalStorage := 0.0
		count := 0

		for _, member := range cluster.Members {
			if cpu, ok := member.Metadata["cpu_usage"].(float64); ok {
				totalCPU += cpu
				count++
			}
			if memory, ok := member.Metadata["memory_usage"].(float64); ok {
				totalMemory += memory
			}
			if network, ok := member.Metadata["network_usage"].(float64); ok {
				totalNetwork += network
			}
			if storage, ok := member.Metadata["storage_usage"].(float64); ok {
				totalStorage += storage
			}
		}

		if count > 0 {
			characteristics["avg_cpu_usage"] = totalCPU / float64(count)
			characteristics["avg_memory_usage"] = totalMemory / float64(count)
			characteristics["avg_network_usage"] = totalNetwork / float64(count)
			characteristics["avg_storage_usage"] = totalStorage / float64(count)
		}

		cluster.Characteristics = characteristics

		// Generate cluster label based on resource usage
		if avgCPU, ok := characteristics["avg_cpu_usage"].(float64); ok {
			if avgCPU > 0.8 {
				cluster.Label = "High CPU Usage Cluster"
			} else if avgCPU < 0.3 {
				cluster.Label = "Low CPU Usage Cluster"
			} else {
				cluster.Label = "Medium CPU Usage Cluster"
			}
		}
	}
}

// Encoding methods

func (ce *ClusteringEngine) encodeAlertType(alertName string) float64 {
	encodings := map[string]float64{
		"HighMemoryUsage":   1.0,
		"PodCrashLoop":      2.0,
		"NodeNotReady":      3.0,
		"DiskSpaceCritical": 4.0,
		"NetworkIssue":      5.0,
		"ServiceDown":       6.0,
		"DeploymentFailed":  7.0,
	}

	if score, exists := encodings[alertName]; exists {
		return score / 7.0 // Normalize to 0-1
	}
	return 0.0
}

func (ce *ClusteringEngine) encodeSeverity(severity string) float64 {
	encodings := map[string]float64{
		"critical": 1.0,
		"warning":  0.75,
		"info":     0.5,
		"debug":    0.25,
	}

	if score, exists := encodings[severity]; exists {
		return score
	}
	return 0.5 // Default
}

func (ce *ClusteringEngine) encodeResourceType(resourceType string) float64 {
	encodings := map[string]float64{
		"deployment": 1.0,
		"pod":        2.0,
		"service":    3.0,
		"node":       4.0,
		"pvc":        5.0,
	}

	if score, exists := encodings[resourceType]; exists {
		return score / 5.0 // Normalize to 0-1
	}
	return 0.0
}

func (ce *ClusteringEngine) getTopKeys(counts map[string]int, limit int) []string {
	type kv struct {
		Key   string
		Value int
	}

	var sorted []kv
	for k, v := range counts {
		sorted = append(sorted, kv{k, v})
	}

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Value > sorted[j].Value
	})

	result := make([]string, 0)
	for i, kv := range sorted {
		if i >= limit {
			break
		}
		result = append(result, kv.Key)
	}

	return result
}

// Supporting types

type MLClusterer struct {
	config *patterns.PatternDiscoveryConfig
	log    *logrus.Logger
}

func NewMLClusterer(config *patterns.PatternDiscoveryConfig, log *logrus.Logger) *MLClusterer {
	return &MLClusterer{
		config: config,
		log:    log,
	}
}

type AlertClusterGroup struct {
	Members               []*engine.EngineWorkflowExecutionData `json:"members"`
	CommonCharacteristics string                                `json:"common_characteristics"`
	AlertTypes            []string                              `json:"alert_types"`
	Namespaces            []string                              `json:"namespaces"`
	Resources             []string                              `json:"resources"`
	TimeWindow            time.Duration                         `json:"time_window"`
	CommonLabels          map[string]string                     `json:"common_labels"`
	Confidence            float64                               `json:"confidence"`
	SuccessRate           float64                               `json:"success_rate"`
}
