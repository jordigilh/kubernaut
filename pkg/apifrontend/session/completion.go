package session

// CompletionObligation describes the independent presentation and execution
// consequences of a successful workflow discovery result.
type CompletionObligation struct {
	PresentationRequired bool
	ExecutionAllowed     bool
}

// DecisionObligation derives the business outcome required after workflow
// discovery. Presentation is required for user-facing modes, while execution
// remains controlled exclusively by the phase-3 consent state.
func DecisionObligation(mode string, discoverySucceeded, phase3Blocked bool) CompletionObligation {
	if !discoverySucceeded {
		return CompletionObligation{ExecutionAllowed: !phase3Blocked}
	}

	if !ValidInteractionMode(mode) {
		mode = InteractionModeInteractive
	}
	if mode == InteractionModeFullRemediationAutonomous {
		return CompletionObligation{ExecutionAllowed: !phase3Blocked}
	}

	return CompletionObligation{
		PresentationRequired: true,
		ExecutionAllowed:     !phase3Blocked,
	}
}
