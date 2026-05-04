package agent

import (
	_ "embed"
	"strings"
)

//go:embed prompt.txt
var embeddedPrompt string

// defaultInstruction returns the system prompt loaded from the embedded prompt.txt.
// The file can be overridden at runtime by supplying WithInstruction.
func defaultInstruction() string {
	return strings.TrimSpace(embeddedPrompt)
}
