package launcher

import (
	"context"
	"net/http"
	"time"

	"github.com/a2aproject/a2a-go/a2a"
	"github.com/go-logr/logr"
	"google.golang.org/adk/model"
	"google.golang.org/adk/server/adka2a"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/jordigilh/kubernaut/pkg/shared/types"
)

// NewReinvokingRunnerForTest exports newReinvokingRunner for unit testing
// (UT-AF-REINV-002/003, issue #1776).
func NewReinvokingRunnerForTest(inner adka2a.Runner, sessionService session.Service, appName string, logger logr.Logger) *reinvokingRunner {
	return newReinvokingRunner(inner, sessionService, appName, logger)
}

// EnrichRRDetailForTest exports enrichRRDetail for unit testing.
func EnrichRRDetailForTest(ctx context.Context, detail map[string]string) {
	enrichRRDetail(ctx, detail)
}

// PartConverterFunc is the exported signature for testing the part converter.
type PartConverterFunc func(ctx context.Context, adkEvent *session.Event, part *genai.Part) (a2a.Part, error)

// BuildPartConverterForTest exports buildPartConverter for unit testing.
func BuildPartConverterForTest() PartConverterFunc {
	fn := buildPartConverter()
	return func(ctx context.Context, adkEvent *session.Event, part *genai.Part) (a2a.Part, error) {
		return fn(ctx, adkEvent, part)
	}
}

// BuildStreamingPartConverterForTest exports buildStreamingPartConverter for unit testing.
func BuildStreamingPartConverterForTest() PartConverterFunc {
	fn := buildStreamingPartConverter()
	return func(ctx context.Context, adkEvent *session.Event, part *genai.Part) (a2a.Part, error) {
		return fn(ctx, adkEvent, part)
	}
}

// BuiltConverterIsNonNil verifies buildPartConverter returns a non-nil function.
func BuiltConverterIsNonNil() bool {
	return buildPartConverter() != nil
}

// ExpectedOutputMode returns the OutputMode constant wired in the ExecutorConfig.
func ExpectedOutputMode() adka2a.OutputMode {
	return adka2a.OutputArtifactPerEvent
}

// BuildTransportChainForTest exports buildTransportChain for unit testing.
func BuildTransportChainForTest(cfg types.LLMConfig) (http.RoundTripper, error) {
	return buildTransportChain(cfg)
}

// EnsureTrailingParagraphBreakForTest exports ensureTrailingParagraphBreak for unit testing.
func EnsureTrailingParagraphBreakForTest(s string) string {
	return ensureTrailingParagraphBreak(s)
}

// SanitizeBridgeTextForTest exports sanitizeBridgeText for unit testing.
func SanitizeBridgeTextForTest(ctx context.Context, text string) string {
	return sanitizeBridgeText(ctx, text)
}

// ResolveA2AMethodForTest exports resolveA2AMethod for unit testing.
func ResolveA2AMethodForTest(ctx context.Context) string {
	return resolveA2AMethod(ctx)
}

// LoggerForTest exports A2AConfig.logger for unit testing.
func LoggerForTest(cfg A2AConfig) logr.Logger {
	return cfg.logger()
}

// StreamingExecutorLoggerForTest returns the logger stored in a StreamingExecutor.
func StreamingExecutorLoggerForTest(se *StreamingExecutor) logr.Logger {
	return se.logger
}

// StripEmojiForTest exports stripEmoji for unit testing.
func StripEmojiForTest(s string) string {
	return stripEmoji(s)
}

// EmitArtifactForTest exports EmitArtifact via bridge from context for testing.
func EmitArtifactForTest(ctx context.Context, data map[string]any, textFallback string, meta map[string]any) error {
	bridge := EventBridgeFromContext(ctx)
	if bridge == nil {
		return nil
	}
	return bridge.EmitArtifact(ctx, data, textFallback, meta)
}

// LastArtifactDataForTest extracts the DataPart.Data map from a
// TaskArtifactUpdateEvent, or nil if the event carries no DataPart
// (#2110/#2111 EmitArtifact gob-safety boundary tests).
func LastArtifactDataForTest(evt a2a.Event) map[string]any {
	artEvt, ok := evt.(*a2a.TaskArtifactUpdateEvent)
	if !ok || artEvt.Artifact == nil {
		return nil
	}
	for _, part := range artEvt.Artifact.Parts {
		if dp, ok := part.(a2a.DataPart); ok {
			return dp.Data
		}
	}
	return nil
}

// ArtifactHasDataPartForTest reports whether the event's artifact carries a
// DataPart at all, distinct from LastArtifactDataForTest returning nil for
// an empty-but-present map (#2110/#2111 degraded-artifact assertions).
func ArtifactHasDataPartForTest(evt a2a.Event) bool {
	artEvt, ok := evt.(*a2a.TaskArtifactUpdateEvent)
	if !ok || artEvt.Artifact == nil {
		return false
	}
	for _, part := range artEvt.Artifact.Parts {
		if _, ok := part.(a2a.DataPart); ok {
			return true
		}
	}
	return false
}

// ArtifactTextFallbackForTest extracts the TextPart.Text from a
// TaskArtifactUpdateEvent (#2110/#2111 degraded-artifact assertions).
func ArtifactTextFallbackForTest(evt a2a.Event) string {
	artEvt, ok := evt.(*a2a.TaskArtifactUpdateEvent)
	if !ok || artEvt.Artifact == nil {
		return ""
	}
	for _, part := range artEvt.Artifact.Parts {
		if tp, ok := part.(a2a.TextPart); ok {
			return tp.Text
		}
	}
	return ""
}

// ValidatePayloadForTest exports ValidatePayload for testing.
func ValidatePayloadForTest(schemaName string, data map[string]any) error {
	return ValidatePayload(schemaName, data)
}

// IsTimeoutWrappedForTest reports whether m is a *timeoutModel (the #1955
// decorator applied by newAnthropicModel/newVertexAnthropicModel) and, if
// so, its configured timeout. Lets black-box (launcher_test) specs prove
// the decorator is wired into NewModelFromConfig's real construction sites
// without exposing timeoutModel itself.
func IsTimeoutWrappedForTest(m model.LLM) (time.Duration, bool) {
	tm, ok := m.(*timeoutModel)
	if !ok {
		return 0, false
	}
	return tm.timeout, true
}

// UnwrapTimeoutForTest peels off the #1955 timeoutModel decorator (if
// present) and returns the underlying provider-specific model.LLM
// unchanged otherwise. Lets pre-existing dispatch-regression specs (e.g.
// IT-AF-1792-002) keep asserting on the concrete provider type via
// fmt.Sprintf("%T", ...) without being coupled to whether that provider
// also happens to be timeout-wrapped.
func UnwrapTimeoutForTest(m model.LLM) model.LLM {
	if tm, ok := m.(*timeoutModel); ok {
		return tm.inner
	}
	return m
}
