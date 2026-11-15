package tools

import (
	"bytes"
	"encoding/json"
	"fmt"

	remediationv1 "github.com/jordigilh/kubernaut/api/remediation/v1alpha1"
	sharedtypes "github.com/jordigilh/kubernaut/pkg/shared/types"
)

// UnmarshalJSON keeps the MCP/LLM approval boundary as ordinary JSON while
// constructing the typed CRD union before any Kubernetes write. The custom
// decoder also captures the original numeric tokens for parameter_evidence.
func (a *ApproveArgs) UnmarshalJSON(data []byte) error {
	var wire struct {
		RARName          string `json:"rar_name"`
		Decision         string `json:"decision"`
		Reason           string `json:"reason"`
		WorkflowOverride *struct {
			WorkflowName string          `json:"workflowName"`
			Parameters   json.RawMessage `json:"parameters"`
			Rationale    string          `json:"rationale"`
		} `json:"workflow_override"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*a = ApproveArgs{RARName: wire.RARName, Decision: wire.Decision, Reason: wire.Reason}
	if wire.WorkflowOverride == nil {
		return nil
	}
	override := &remediationv1.WorkflowOverride{
		WorkflowName: wire.WorkflowOverride.WorkflowName,
		Rationale:    wire.WorkflowOverride.Rationale,
	}
	raw := bytes.TrimSpace(wire.WorkflowOverride.Parameters)
	if len(raw) != 0 && !bytes.Equal(raw, []byte("null")) {
		values, evidence, err := sharedtypes.DecodeTypedParameterMap(raw)
		if err != nil {
			return fmt.Errorf("decode workflow override parameters: %w", err)
		}
		override.Parameters = values
		override.ParameterEvidence = evidence
	}
	a.WorkflowOverride = override
	return nil
}
