package scenarios

import (
	"strings"

	"github.com/jordigilh/kubernaut/pkg/apifrontend/validate"
	"github.com/jordigilh/kubernaut/test/services/mock-llm/conversation"
)

// This test-service fixture models caller two's LLM tool choice, not AF/KA
// business logic. The RR comes from caller one's live stream, passed explicitly
// in a separate conversation; $from_tool cannot cross conversation boundaries.
type afExistingRRScenario struct{}

const afContentionTool = "kubernaut_investigate"

func (*afExistingRRScenario) Name() string { return "af_fleet_existing_rr_2593" }
func (s *afExistingRRScenario) Metadata() ScenarioMetadata {
	return ScenarioMetadata{Name: s.Name(), Caller: CallerAF}
}
func (*afExistingRRScenario) DAG() *conversation.DAG { return nil }
func (*afExistingRRScenario) Match(ctx *DetectionContext) (bool, float64) {
	if fleetContentionRR(ctx) == "" {
		return false, 0
	}
	return true, 1
}
func (s *afExistingRRScenario) ConfigForContext(ctx *DetectionContext) MockScenarioConfig {
	cfg := MockScenarioConfig{ScenarioName: s.Name()}
	if rrID := fleetContentionRR(ctx); rrID != "" {
		cfg.ToolCallName = afContentionTool
		cfg.ToolCallArgs = map[string]interface{}{"rr_id": rrID}
		cfg.ForceText = BoolPtr(false)
		cfg.ExactAnalysisText = "The tool returned status guidance; no new investigation findings were produced."
	}
	return cfg
}

func fleetContentionRR(ctx *DetectionContext) string {
	if ctx == nil || ctx.Caller != CallerAF {
		return ""
	}
	const prefix = "fleet lease contention investigate rr_id="
	if !strings.HasPrefix(ctx.LastUserContent, prefix) {
		return ""
	}
	// Accept exactly one plain Kubernetes name, not a URL, JSON, interpolation,
	// namespace override, additional instruction or a marker in older history.
	rrID := strings.TrimPrefix(ctx.LastUserContent, prefix)
	if validate.ResourceName(rrID) != nil {
		return ""
	}
	for _, name := range ctx.AvailableTools {
		if name == afContentionTool {
			return rrID
		}
	}
	return ""
}
