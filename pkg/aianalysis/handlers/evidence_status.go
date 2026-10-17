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

package handlers

import (
	agentsessionv1 "github.com/jordigilh/kubernaut/api/agentsession/v1alpha1"
	aianalysisv1 "github.com/jordigilh/kubernaut/api/aianalysis/v1alpha1"
)

func mapEvidenceStatusToAIAnalysis(status *agentsessionv1.EvidenceStatus) *aianalysisv1.EvidenceStatus {
	if status == nil {
		return nil
	}
	mapped := &aianalysisv1.EvidenceStatus{
		Version:          status.Version,
		Class:            aianalysisv1.EvidenceFailureClass(status.Class),
		Source:           aianalysisv1.EvidenceSource(status.Source),
		Outcome:          aianalysisv1.EvidenceOutcome(status.Outcome),
		TargetRef:        status.TargetRef,
		ClusterID:        status.ClusterID,
		LogicalTool:      status.LogicalTool,
		Attempts:         status.Attempts,
		LastFailureCode:  status.LastFailureCode,
		CorrelationID:    status.CorrelationID,
		SourcesTruncated: status.SourcesTruncated,
	}
	if len(status.Sources) == 0 {
		return mapped
	}
	mapped.Sources = make([]aianalysisv1.EvidenceSourceStatus, 0, len(status.Sources))
	for _, source := range status.Sources {
		mapped.Sources = append(mapped.Sources, aianalysisv1.EvidenceSourceStatus{
			Version:         source.Version,
			Class:           aianalysisv1.EvidenceFailureClass(source.Class),
			Source:          aianalysisv1.EvidenceSource(source.Source),
			Outcome:         aianalysisv1.EvidenceOutcome(source.Outcome),
			TargetRef:       source.TargetRef,
			ClusterID:       source.ClusterID,
			LogicalTool:     source.LogicalTool,
			Attempts:        source.Attempts,
			LastFailureCode: source.LastFailureCode,
			CorrelationID:   source.CorrelationID,
		})
	}
	return mapped
}

func evidenceFailureClassification(status *agentsessionv1.EvidenceStatus) (reason aianalysisv1.AIAnalysisReason, subReason string, failed bool) {
	if status == nil {
		return "", "", false
	}
	switch {
	case status.Class == agentsessionv1.EvidenceFailureTransportUnavailable,
		status.Outcome == agentsessionv1.EvidenceOutcomeTransportUnavailable:
		return aianalysisv1.ReasonAPIError, aianalysisv1.SubReasonTransientError, true
	case status.Class == agentsessionv1.EvidenceFailureSchemaClientRequestFailure,
		status.Outcome == agentsessionv1.EvidenceOutcomeSchemaClientFailure:
		return aianalysisv1.ReasonAPIError, "PermanentError", true
	default:
		return "", "", false
	}
}
