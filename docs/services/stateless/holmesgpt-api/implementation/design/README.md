# HolmesGPT API Design Decisions

Design decisions for HolmesGPT API are centralized in:
`docs/architecture/decisions/DD-HOLMESGPT-*`

---

## Available Decisions

### Format and Optimization
- **DD-HOLMESGPT-009**: Self-Documenting JSON Format (token optimization)
  - Path: `docs/architecture/decisions/DD-HOLMESGPT-009-Ultra-Compact-JSON-Format.md`
  - Summary: 63.75% token reduction (800 → 290 tokens), $2.24M/year savings

- **DD-HOLMESGPT-009-ADDENDUM**: YAML vs JSON Evaluation
  - Path: `docs/architecture/decisions/DD-HOLMESGPT-009-ADDENDUM-YAML-Evaluation.md`
  - Summary: YAML provides 17.5% additional reduction but insufficient ROI ($75-100/year vs $4-6K implementation)

### Architecture and Integration
- **DD-EFFECTIVENESS-001**: Hybrid Automated+AI Analysis
  - Path: `docs/architecture/decisions/DD-EFFECTIVENESS-001-Hybrid-Automated-AI-Analysis.md`
  - Summary: Effectiveness Monitor uses selective AI (0.7% of actions, $989/year)

- **DD-EFFECTIVENESS-003**: RemediationRequest Watch Strategy
  - Path: `docs/architecture/decisions/DD-EFFECTIVENESS-003-RemediationRequest-Watch-Strategy.md`
  - Summary: Watch RemediationRequest CRD (not WorkflowExecution) for decoupling

### Safety and Investigation
- **DD-HOLMESGPT-008**: Safety-Aware Investigation
  - Path: `docs/architecture/decisions/DD-HOLMESGPT-008-Safety-Aware-Investigation.md`
  - Summary: Embed safety context in prompts (not separate endpoint)

---

## Complete Index

For the complete decision document index and cross-service decisions, see:
`docs/architecture/decisions/README.md`

---

## Design Decision Template

When creating new HolmesGPT API design decisions, follow the template structure:

```markdown
# DD-HOLMESGPT-XXX: [Decision Title]

**Date**: YYYY-MM-DD
**Status**: [PROPOSED | APPROVED | REJECTED | SUPERSEDED]
**Confidence**: XX%
**Decision Makers**: [Names/Roles]

## Context
[Problem statement and background]

## Decision
[The decision made]

## Alternatives Considered
[Other options evaluated]

## Rationale
[Why this decision was made]

## Consequences
[Impact and implications]

## References
[Related documents and decisions]
```

---

## Local vs Centralized Decisions

**Centralized** (in `docs/architecture/decisions/`):
- Cross-service decisions (affects multiple services)
- Format standards (JSON, YAML, token optimization)
- Architectural patterns (CRD watch strategies, hybrid approaches)

**Local** (in this directory):
- Service-specific implementation choices
- Technology stack decisions (FastAPI, pytest)
- Internal API design patterns

**Note**: Currently, HolmesGPT API decisions are primarily centralized due to their architectural impact across Kubernaut.


