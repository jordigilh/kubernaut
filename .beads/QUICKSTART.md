# Gas Town Quick Start: SOC2 Audit Trail Convoy

## 🚀 **Start Day 3 in 5 Minutes**

### Prerequisites
- Gas Town installed: `https://github.com/steveyegge/gastown`
- Day 2 complete (100% tests passing)
- Kubectl access to Kind cluster

---

## Step 1: Create Convoy (One-Time Setup)

```bash
gastown convoy create \
  --id soc2-audit-trail \
  --name "SOC2 Audit Trail - RR Reconstruction" \
  --owner @jgil \
  --config .beads/convoys/soc2-audit-trail/convoy.toml
```

**Expected Output**:
```
✅ Convoy created: soc2-audit-trail
📊 Progress: 2/8 days complete (25%)
📁 Configuration: .beads/convoys/soc2-audit-trail/convoy.toml
🎯 Next Molecule: day3-workflow-refs
```

---

## Step 2: Launch Day 3 Molecule

```bash
gastown molecule create \
  --formula integration-test-full-validation \
  --name day3-workflow-refs \
  --convoy soc2-audit-trail \
  --owner @jgil \
  --team workflowexecution \
  --inputs docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md
```

**Expected Output**:
```
✅ Molecule created: day3-workflow-refs
📋 Formula: integration-test-full-validation v2.1
⏱️  Estimated Duration: 2h 10m
🔍 First Step: analysis (15m)
🚨 Checkpoints: 4 human approvals required
```

---

## Step 3: Monitor Progress

```bash
# Real-time status
gastown molecule status day3-workflow-refs

# Expected output:
# Molecule: day3-workflow-refs
# Status: analysis (in progress)
# Progress: 10% (Step 1/12)
# ETA: 2h
# Last Update: Analyzing test requirements from SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md
```

---

## Step 4: Respond to Analysis Checkpoint (First Human Interaction)

**You'll receive notification**:
```
📨 Gas Town Notification
🚨 Checkpoint: day3-workflow-refs/analysis-checkpoint
⏰ Timeout: 2 hours
📋 Review: Analysis summary ready for your review

View: gastown checkpoint show day3-workflow-refs analysis-checkpoint
```

**View the checkpoint**:
```bash
gastown checkpoint show day3-workflow-refs analysis-checkpoint
```

**Expected Output**:
```
Analysis Summary:
- Business Requirements: BR-AUDIT-005 (Gap 5-6), BR-WE-013
- Existing Tests: Found 2 similar patterns
- Infrastructure: WE controller + Data Storage required

Questions from Polecat:
1. Should we test BOTH successful workflow execution AND alternative workflow rejection?
2. Do we need to validate the `selected_workflow_id` matches between events?
3. Should this test cover degraded mode (fallback workflows)?

Recommendation: Test BOTH selection + execution events (2 events per workflow)

Confidence: 85%
```

**Approve or reject**:
```bash
# Approve (most common case)
gastown checkpoint approve day3-workflow-refs analysis-checkpoint

# Or provide feedback
gastown checkpoint comment day3-workflow-refs analysis-checkpoint \
  --text "Yes to all 3 questions. Add degraded mode in separate spec for clarity."

# Or reject (if major concerns)
gastown checkpoint reject day3-workflow-refs analysis-checkpoint \
  --reason "Need to discuss degraded mode approach first"
```

---

## Step 5: Approve Plan Checkpoint (Second Human Interaction)

**After plan phase completes (~20 minutes after analysis approval)**:

```bash
# View plan
gastown checkpoint show day3-workflow-refs plan-checkpoint

# Review test plan, TDD strategy, integration points, success criteria

# Approve
gastown checkpoint approve day3-workflow-refs plan-checkpoint
```

---

## Step 6: Automatic DO Phase (RED → GREEN → REFACTOR)

**Gas Town automatically executes (~45 minutes)**:
- Infrastructure setup (10m)
- TDD RED: Write failing tests (15m)
- TDD GREEN: Minimal implementation (20m)
- TDD REFACTOR: Add edge cases (15m)

**Monitor progress**:
```bash
watch -n 30 'gastown molecule status day3-workflow-refs'
```

---

## Step 7: Test Validation Checkpoint (Third Human Interaction) 🚨

**This is the mandatory 100% test pass gate**:

```bash
# View test results
gastown checkpoint show day3-workflow-refs test-validation-checkpoint

# Expected output:
# Test Validation Results:
# ✅ Total Tests: 35 (32 existing + 3 new)
# ✅ Passed: 35
# ✅ Failed: 0
# ✅ Skipped: 0
# ✅ Pass Rate: 100%
#
# Anti-Pattern Detection:
# ✅ No time.Sleep() detected
# ✅ No Skip() calls
# ✅ Eventually() used correctly
```

**Approve** (if 100% pass):
```bash
gastown checkpoint approve day3-workflow-refs test-validation-checkpoint
```

**Reject** (if < 100% pass):
```bash
gastown checkpoint reject day3-workflow-refs test-validation-checkpoint \
  --reason "3 tests failing, need fixes"
# Gas Town will automatically return to do-refactor phase
```

---

## Step 8: Compliance Checkpoint (Fourth Human Interaction)

**After compliance triage completes (~30-45 minutes after test validation approval)**:

```bash
# View compliance report
gastown checkpoint show day3-workflow-refs compliance-checkpoint

# Review report sections:
# - Test validation status
# - BR mapping
# - Testing standards compliance
# - API contract alignment
# - APDC methodology adherence
# - SOC2 requirements
# - Anti-patterns detected
# - Critical issues
# - Recommendations
```

**Read full report**:
```bash
cat .beads/convoys/soc2-audit-trail/molecules/day3-workflow-refs/reports/compliance-triage-$(date +%Y-%m-%d).md
```

**Approve**:
```bash
gastown checkpoint approve day3-workflow-refs compliance-checkpoint
```

---

## Step 9: Automatic Documentation & Day Completion

**Gas Town automatically (~10 minutes)**:
- Updates test plan status
- Commits compliance report
- Generates Day 3 summary
- Updates convoy progress to 37.5% (3/8 days)
- Prepares handoff to Day 4

---

## Step 10: View Convoy Progress

```bash
gastown convoy show soc2-audit-trail
```

**Expected Output**:
```
Convoy: soc2-audit-trail
Status: In Progress (Day 3 Complete)
Progress: 3/8 days (37.5%)

Molecules:
✅ day1-gateway-fields (complete, 100% pass)
✅ day2-hybrid-provider-data (complete, 100% pass)
✅ day3-workflow-refs (complete, 100% pass)
⏳ day4-error-details (ready to start)
⏳ day5-error-all-services (blocked by day4)
⏳ day6-timeout-config (blocked by day4-5)
⏳ day7-rr-reconstruction-integration (blocked by day6)
⏳ day8-e2e-validation (blocked by day7)

Next: Launch day4-error-details molecule
```

---

## Common Commands Reference

### Convoy Management
```bash
# Show convoy status
gastown convoy show soc2-audit-trail

# List all convoys
gastown convoy list

# View convoy health
gastown convoy health soc2-audit-trail
```

### Molecule Management
```bash
# List all molecules in convoy
gastown molecule list --convoy soc2-audit-trail

# View molecule status
gastown molecule status day3-workflow-refs

# View molecule history
gastown molecule history day3-workflow-refs

# Pause molecule (emergency)
gastown molecule pause day3-workflow-refs

# Resume molecule
gastown molecule resume day3-workflow-refs
```

### Checkpoint Management
```bash
# List all checkpoints
gastown checkpoint list day3-workflow-refs

# Show checkpoint details
gastown checkpoint show day3-workflow-refs analysis-checkpoint

# Approve checkpoint
gastown checkpoint approve day3-workflow-refs analysis-checkpoint

# Reject checkpoint
gastown checkpoint reject day3-workflow-refs analysis-checkpoint \
  --reason "Need more analysis"

# Comment on checkpoint
gastown checkpoint comment day3-workflow-refs analysis-checkpoint \
  --text "Looks good, minor suggestion: add X"
```

### Mail System
```bash
# View messages
gastown mail inbox

# Read specific message
gastown mail read <message-id>

# Send message to team
gastown mail send --to @workflow-execution-team \
  --subject "Day 3 Question" \
  --body "..."
```

---

## Notification Setup (Optional)

**Email notifications**:
```bash
gastown config set notifications.email.enabled true
gastown config set notifications.email.address "you@example.com"
```

**Slack notifications**:
```bash
gastown config set notifications.slack.enabled true
gastown config set notifications.slack.webhook "https://hooks.slack.com/..."
gastown config set notifications.slack.channel "#kubernaut-soc2"
```

**Mobile app** (if available):
```bash
gastown config set notifications.mobile.enabled true
gastown config set notifications.mobile.device_token "..."
```

---

## Troubleshooting

### Checkpoint Timeout
**Problem**: Checkpoint approval timed out (2h)

**Solution**:
```bash
# Checkpoint auto-rejected, molecule paused
gastown molecule resume day3-workflow-refs

# Checkpoint will re-trigger
gastown checkpoint approve day3-workflow-refs analysis-checkpoint
```

### Test Validation Failure
**Problem**: Tests not passing at 100%

**Solution**:
```bash
# Gas Town automatically returns to do-refactor phase
# View failure details
gastown checkpoint show day3-workflow-refs test-validation-checkpoint

# Polecat will fix issues and re-run test-validation
# No manual intervention needed unless you want to guide fixes
```

### Compliance Issues Detected
**Problem**: Compliance triage found critical violations

**Solution**:
```bash
# View compliance report
cat .beads/convoys/soc2-audit-trail/molecules/day3-workflow-refs/reports/compliance-triage-*.md

# Review critical issues section
# Reject compliance checkpoint with guidance
gastown checkpoint reject day3-workflow-refs compliance-checkpoint \
  --reason "Fix critical issue #1: missing BR mapping for test X"

# Gas Town returns to appropriate phase (plan or do-refactor)
```

---

## Time Estimates

| Phase | Duration | Human Interaction |
|-------|----------|-------------------|
| Analysis | 15m | ✅ Checkpoint approval |
| Plan | 20m | ✅ Checkpoint approval |
| Infrastructure | 10m | ❌ Automated |
| DO-RED | 15m | ❌ Automated |
| DO-GREEN | 20m | ❌ Automated |
| DO-REFACTOR | 15m | ❌ Automated |
| Test Validation | 10m | ✅ Checkpoint approval |
| Compliance Triage | 30-45m | ❌ Automated |
| Compliance Review | 10m | ✅ Checkpoint approval |
| Documentation | 10m | ❌ Automated |
| **Total** | **2-2.5h** | **~30-40 minutes human time** |

**Key Insight**: Out of 2-2.5 hours total, only ~30-40 minutes requires human attention (4 checkpoints).

---

## Next Steps After Day 3

### Launch Day 4-5 (Error Details)

**Option A: Sequential (single molecule)**
```bash
gastown molecule create \
  --formula integration-test-full-validation \
  --name day4-5-error-details \
  --convoy soc2-audit-trail \
  --duration "2 days"
```

**Option B: Parallel (per service)**
```bash
# Gateway errors
gastown molecule create --formula integration-test-full-validation \
  --name day4-gateway-errors --convoy soc2-audit-trail

# AI Analysis errors
gastown molecule create --formula integration-test-full-validation \
  --name day4-ai-errors --convoy soc2-audit-trail

# Workflow Execution errors
gastown molecule create --formula integration-test-full-validation \
  --name day4-we-errors --convoy soc2-audit-trail

# ... etc (5 molecules total)
```

**Benefit of Option B**: Reduces calendar time from 2 days to ~1 day

---

## Dashboard View (Future Enhancement)

**If Gas Town has web dashboard**:
```
http://localhost:8080/convoys/soc2-audit-trail

Visual display:
- Progress bar (37.5% complete)
- Molecule status cards
- Checkpoint timeline
- Test pass rate graphs
- Compliance status indicators
```

---

## Support

- **Gas Town Docs**: `https://github.com/steveyegge/gastown/docs`
- **Formula Issues**: Create issue in `.beads/FORMULA_ISSUES.md`
- **Kubernaut Rules**: `.cursor/rules/*.mdc`
- **SOC2 Test Plan**: `docs/development/SOC2/SOC2_AUDIT_RR_RECONSTRUCTION_TEST_PLAN.md`

---

**Quick Start Complete! 🎉**

You're now ready to orchestrate SOC2 audit trail work with Gas Town's structured workflow, mandatory quality gates, and human-in-the-loop checkpoints.

**Time to Value**: ~10 minutes to launch Day 3, ~2-2.5 hours to complete Day 3 with 100% test pass guarantee.

