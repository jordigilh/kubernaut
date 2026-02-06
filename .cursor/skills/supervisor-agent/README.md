# Supervisor Agent - Autonomous Work Orchestration

## 🎯 What is This?

The **Supervisor Agent** is a skill that orchestrates development work with **independent, unbiased oversight**:

1. **Reading** your business requirements
2. **Clarifying** ambiguities (90% confidence gate)
3. **Planning** task breakdowns with TDD phases
4. **Spawning** independent worker via Task tool (no self-review bias)
5. **Monitoring** with evidence-based validation at checkpoints
6. **Recovering** from failures (escalation protocol)
7. **Approving** when all standards met

Think of it as an **independent QA supervisor** that ensures quality without self-review bias.

**Key Innovation**: Supervisor and worker are **separate agents** for objective validation.

---

## 📁 Skill Structure

```
supervisor-agent/
├── SKILL.md (503 lines)              # Core workflow with multi-agent model
├── CONFIDENCE_GATES.md (232 lines)   # Objective question-count method
├── CHECKPOINT_VALIDATION.md (364)    # Evidence-based checkpoint protocols
├── COMMUNICATION_TEMPLATES.md (376)  # All communication templates
├── USAGE_GUIDE.md (468 lines)        # Usage examples and patterns
├── BR_INTEGRATION.md (437 lines)     # BR document parsing
├── IMPROVEMENTS_APPLIED.md (new)     # Summary of all improvements
├── TRIAGE_IMPROVEMENTS.md (555)      # Original triage analysis
└── README.md (this file)             # Overview
```

**Progressive Disclosure**: SKILL.md contains core workflow (503 lines, optimized for agent performance) with links to detailed references. Agent reads concise instructions first, accesses details only when needed.

---

## 🆚 Supervisor vs Review Agent

| Feature | Review Agent | Supervisor Agent |
|---------|--------------|------------------|
| **When** | After work complete | Throughout execution |
| **Role** | Validates quality | Orchestrates work |
| **Reads BRs** | No (expects task known) | Yes (reads BR docs) |
| **Task Planning** | No | Yes (breaks down BR) |
| **Checkpoints** | No (single review) | Yes (RED/GREEN/REFACTOR) |
| **Worker Assignment** | No | Yes (spawns sub-agent) |
| **Feedback Loops** | Final report only | Continuous guidance |
| **Early Detection** | No | Yes (catches issues early) |

**Use Review Agent**: Validate completed work

**Use Supervisor Agent**: Orchestrate new work with oversight

---

## 🚀 Quick Start

### Option 1: From BR Document

```
You: Supervisor, implement BR-WORKFLOW-197 from 
     ../cursor-swarm-dev/docs/business-requirements/BR-WORKFLOW-197.md
     
     Assign to worker agent and monitor with checkpoints.
```

### Option 2: Inline Specification

```
You: Supervisor, oversee implementation of:

"Add Redis caching to workflow search"

Acceptance Criteria:
- Cache layer in pkg/datastorage/cache/
- Integration in cmd/datastorage/main.go  
- 70%+ test coverage
- Use existing CacheProvider interface

Assign to worker with checkpoint monitoring.
```

---

## 📊 How It Works

```
┌─────────────────────────────────────────────┐
│  1. Supervisor Reads BR                     │
│     - Business objective                    │
│     - Acceptance criteria                   │
│     - Technical requirements                │
└──────────────────┬──────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────┐
│  2. Supervisor Plans Tasks                  │
│     Phase 1 - RED:    Write tests           │
│     Phase 2 - GREEN:  Implement + integrate │
│     Phase 3 - REFACTOR: Enhance quality     │
└──────────────────┬──────────────────────────┘
                   │
                   ▼
┌─────────────────────────────────────────────┐
│  3. Supervisor Assigns to Worker            │
│     "Worker: Implement [tasks]              │
│      Report at each checkpoint"             │
└──────────────────┬──────────────────────────┘
                   │
                   ▼
         ┌─────────┴─────────┐
         │                   │
         ▼                   ▼
    ┌────────┐         ┌────────┐
    │ Worker │         │ Worker │
    │  RED   │────────▶│ GREEN  │────┐
    └────────┘         └────────┘    │
         │                            │
         │ Report                     │ Report
         │                            │
         ▼                            ▼
┌────────────────┐          ┌────────────────┐
│  Supervisor    │          │  Supervisor    │
│  Validates     │          │  Validates     │
│  Test Quality  │          │  Integration   │
└────────┬───────┘          └────────┬───────┘
         │                           │
         │ ✅ Approve                │ ✅ Approve
         │                           │
         └────────────┬──────────────┘
                      │
                      ▼
                 ┌────────┐
                 │ Worker │
                 │REFACTOR│
                 └────┬───┘
                      │
                      │ Report
                      ▼
              ┌───────────────┐
              │  Supervisor   │
              │ Final Review  │
              │  ✅ APPROVED  │
              └───────────────┘
```

---

## ✅ Key Benefits

### 1. **Early Issue Detection**
```
❌ Without Supervisor: Issue found in final review (late)
✅ With Supervisor: Issue caught in RED checkpoint (early)
Result: Less rework, faster delivery
```

### 2. **Enforced TDD Sequence**
```
❌ Without Supervisor: Worker might skip phases or do out of order
✅ With Supervisor: Must complete RED before GREEN, GREEN before REFACTOR
Result: Proper TDD methodology followed
```

### 3. **Integration Verification**
```
❌ Without Supervisor: Integration might be forgotten until final review
✅ With Supervisor: Integration verified in GREEN checkpoint
Result: No orphaned components
```

### 4. **Continuous Quality**
```
❌ Without Supervisor: Quality checked only at end
✅ With Supervisor: Quality validated at each checkpoint
Result: High confidence throughout
```

### 5. **Automatic BR Decomposition**
```
❌ Without Supervisor: User must break down BR into tasks
✅ With Supervisor: Supervisor reads BR and creates task plan
Result: Less manual planning work
```

---

## 🎯 Use Cases

### Use Case 1: Implementing New Features

**Scenario**: You have a BR document for a new feature

**Command**:
```
Supervisor, implement BR-XXX from [path].
Monitor with RED/GREEN/REFACTOR checkpoints.
```

**Result**: Feature implemented with quality validated throughout

---

### Use Case 2: High-Stakes Changes

**Scenario**: Critical security feature requiring extra validation

**Command**:
```
Supervisor, this is critical security work.
Extra checkpoints:
- Test design review
- Security validation  
- Penetration test scenarios
- Final security audit
```

**Result**: Multiple validation points ensure quality and safety

---

### Use Case 3: Junior Developer / New Patterns

**Scenario**: Team member learning Kubernaut patterns

**Command**:
```
Supervisor, guide worker through BR-XXX implementation.
Provide detailed feedback at each checkpoint.
Explain rationale for changes.
```

**Result**: Learning opportunity with quality assurance

---

## 📋 Checkpoint Focus

| Checkpoint | What Supervisor Validates | Key Question |
|-----------|---------------------------|--------------|
| **RED Phase** | Test quality | Do tests validate behavior or implementation? |
| **GREEN Phase** | Integration + minimal implementation | Is component integrated in cmd/? |
| **REFACTOR Phase** | Complete quality | Does work meet all acceptance criteria? |

---

## 🔗 Works With

### Business Requirements
- Reads BR documents from your project
- Extracts acceptance criteria
- Maps tasks to requirements
- Validates against criteria

### Review Agent Skill
- Uses `review-agent-work` for final comprehensive validation
- Combines checkpoint monitoring + deep review

### Kubernaut Standards
- Enforces TDD methodology
- Validates behavior-focused tests
- Ensures main app integration
- Checks code quality

---

## 📚 Documentation

### [SKILL.md](./SKILL.md)
Complete supervisor instructions with:
- Phase-by-phase workflow
- Checkpoint validation checklists
- Communication templates
- Decision framework

### [USAGE_GUIDE.md](./USAGE_GUIDE.md)
How to use the supervisor with:
- Basic usage examples
- Checkpoint flow demonstrations
- Advanced patterns
- Troubleshooting

### [BR_INTEGRATION.md](./BR_INTEGRATION.md)
How supervisor reads your BRs with:
- BR parsing process
- Task decomposition examples
- Checkpoint mapping to acceptance criteria
- Multi-BR coordination

---

## ⚡ Quick Examples

### Example 1: Smooth Execution

```
You: Supervisor, implement BR-WORKFLOW-197

Supervisor: [Reads BR, creates plan, assigns worker]

Worker: RED phase complete

Supervisor: ✅ Tests validate behavior. Proceed to GREEN.

Worker: GREEN phase complete

Supervisor: ✅ Integration verified. Proceed to REFACTOR.

Worker: REFACTOR complete

Supervisor: ✅ APPROVED. Ready for commit.
```

**Result**: Clean execution, all checkpoints passed

---

### Example 2: Issue Caught Early

```
You: Supervisor, implement caching feature

Worker: RED phase complete

Supervisor: ❌ Tests validate implementation logic:
           - "should call Redis 3 times" ← Implementation detail
           
           Fix: Test behavior instead (e.g., "should cache results")

Worker: Fixed tests to validate behavior

Supervisor: ✅ Tests approved. Proceed to GREEN.

[Continues to completion]
```

**Result**: Implementation-logic tests caught in RED (early), not final review (late)

---

### Example 3: Missing Integration Caught

```
You: Supervisor, implement validator

Worker: GREEN phase complete

Supervisor: ❌ Integration missing in cmd/
           Add integration before proceeding to REFACTOR

Worker: Integration added in cmd/datastorage/main.go

Supervisor: ✅ Integration verified. Proceed to REFACTOR.

[Continues to completion]
```

**Result**: Integration enforced in GREEN (correct timing)

---

## 🎓 Best Practices

### DO:
- ✅ Let supervisor read BR documents
- ✅ Trust checkpoint process
- ✅ Provide clarification when supervisor asks
- ✅ Review supervisor's validation evidence

### DON'T:
- ❌ Skip checkpoints ("just do it all")
- ❌ Rush past validation ("looks fine")
- ❌ Ignore supervisor feedback
- ❌ Let worker bypass approval

---

## 🔧 Troubleshooting

**Issue**: Supervisor approves without checking

**Fix**: Request evidence
```
"Supervisor, show me grep results proving integration exists"
```

**Issue**: Worker skips ahead without approval

**Fix**: Make checkpoints explicit
```
"Worker MUST STOP after RED and wait for supervisor approval"
```

**Issue**: Unclear if requirement met

**Fix**: Supervisor should ask user
```
Supervisor: "⏸️ PAUSED - Is 62% test coverage acceptable for this component?"
```

---

## ✨ Try It Now

Ready to use the supervisor? Just say:

```
"Supervisor, implement [BR-NAME] from [path]"
```

Or with inline spec:

```
"Supervisor, oversee implementation of [requirement]
 with checkpoints at RED, GREEN, REFACTOR phases"
```

The supervisor will take it from there!

---

## 📊 Success Metrics

You'll know it's working when:
- ✅ Issues caught early (RED/GREEN) not late (final review)
- ✅ Worker reports at each checkpoint
- ✅ Integration verified in GREEN (not deferred)
- ✅ Final work requires minimal rework
- ✅ All acceptance criteria met
- ✅ High confidence throughout

---

## 🎯 Next Steps

1. **Try with a simple BR**: Start with small feature
2. **Observe checkpoint flow**: See how validation works
3. **Adapt to your needs**: Customize checkpoint frequency
4. **Scale up**: Use for larger, more complex BRs

Happy supervising! 🚀
