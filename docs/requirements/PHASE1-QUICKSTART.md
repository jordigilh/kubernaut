# Phase 1 Quick Start: HolmesGPT SDK Investigation

**Phase**: Phase 1 - SDK Capability Assessment
**Estimated Time**: 1-2 hours
**Goal**: Determine HolmesGPT SDK's error handling capabilities

---

## Prerequisites

**Required**:
- Python 3.8+ installed
- Access to install Python packages (`pip`)
- Terminal/command line access

**Optional** (for comprehensive testing):
- HolmesGPT API key (LLM provider)
- Access to Kubernetes cluster
- `kubectl` configured

---

## Quick Start Steps

### Step 1: Install HolmesGPT SDK

```bash
# Navigate to kubernaut project root
cd /Users/jgil/go/src/github.com/jordigilh/kubernaut

# Install HolmesGPT SDK (verify package name)
pip install holmes-ai  # or: pip3 install holmes-ai

# Verify installation
python3 -c "from holmes import Client; print('✅ SDK installed')"
```

**If installation fails**:
- Check package name: https://pypi.org/search/?q=holmes
- Check GitHub: https://github.com/robusta-dev/holmesgpt
- Try: `pip install holmesgpt` or `pip install holmes-gpt`

---

### Step 2: Run Investigation Script

```bash
# Make script executable
chmod +x scripts/investigate_holmesgpt_sdk.py

# Run investigation
python3 scripts/investigate_holmesgpt_sdk.py | tee docs/requirements/sdk_investigation_report_$(date +%Y%m%d).txt

# View results
cat docs/requirements/sdk_investigation_report_$(date +%Y%m%d).txt
```

**Expected Output**:
```
================================================================================
 HolmesGPT SDK Investigation Report
 BR-HAPI-189 Phase 1: SDK Capability Assessment
 Date: 2025-01-15 10:30:00
================================================================================

================================================================================
Task 1: SDK Import and Basic Inspection
================================================================================

✅ SUCCESS: HolmesGPT SDK imported successfully
   Client class: holmes.client.Client
   Client module file: /path/to/holmes/client.py

[... detailed investigation results ...]
```

---

### Step 3: Analyze Findings

**Review the investigation report for**:

1. **Exception Hierarchy** (Critical):
   - ✅ Does SDK provide `KubernetesToolsetError`, `PrometheusToolsetError`?
   - ✅ Do exceptions have `toolset_name` metadata attribute?
   - ⚠️ SDK uses generic `Exception` with no metadata?

2. **Investigation Results**:
   - ✅ Does `investigate()` return object with `toolsets_used` field?
   - ✅ Does result include toolset execution details?
   - ⚠️ Returns generic dict with no toolset metadata?

3. **Extension Points**:
   - ✅ Can we subclass `Client` to add error tracking?
   - ✅ Are there callback/hook mechanisms for toolset execution?
   - ⚠️ Must use wrapper pattern?

---

### Step 4: Document Findings

**Update investigation document**:

```bash
# Open investigation doc
code docs/requirements/BR-HAPI-189-PHASE1-SDK-INVESTIGATION.md

# Fill in findings from report:
# - Finding 1: Exception Hierarchy
# - Finding 2: Investigation Results Metadata
# - Finding 3: Toolset Callbacks/Hooks
# - Finding 4: Extension Points
```

**For each finding, document**:
- What we found (with code examples from SDK)
- Implications for implementation
- Recommended approach

---

### Step 5: Select Implementation Approach

**Based on findings, choose**:

**Scenario A** (BEST CASE - 95% confidence):
- SDK provides structured exceptions with toolset metadata
- **Action**: Use exception types directly
- **Document in**: BR-HAPI-189 specification

**Scenario B** (LIKELY - 70-85% confidence):
- SDK uses generic exceptions without toolset metadata
- **Action**: Implement multi-tier error classification (traceback + patterns)
- **Document in**: BR-HAPI-189 specification

**Scenario C** (RECOMMENDED - 85% confidence):
- SDK can be wrapped/extended
- **Action**: Create `HolmesGPTClientWrapper` with structured error tracking
- **Document in**: BR-HAPI-189 specification + implementation guide

---

## Expected Deliverables

**After Phase 1 completion**:

1. ✅ **Investigation Report**
   - File: `docs/requirements/sdk_investigation_report_YYYYMMDD.txt`
   - Contains: Detailed findings from investigation script

2. ✅ **Findings Documentation**
   - File: `docs/requirements/BR-HAPI-189-PHASE1-SDK-INVESTIGATION.md`
   - Contains: Findings summary, implications, code examples

3. ✅ **Implementation Approach Selection**
   - File: `docs/requirements/BR-HAPI-189-PHASE1-SDK-INVESTIGATION.md` (Decision Matrix section)
   - Contains: Selected approach with confidence assessment

4. ✅ **Updated BR-HAPI-189 Specification** (optional, can wait for Phase 2)
   - File: `docs/requirements/BR-HAPI-VALIDATION-RESILIENCE.md`
   - Contains: Updated error classification approach based on findings

---

## Troubleshooting

### Issue: Cannot import HolmesGPT SDK

**Symptoms**:
```
❌ FAILED: Cannot import HolmesGPT SDK
   Error: No module named 'holmes'
```

**Solutions**:
1. Verify package name on PyPI: https://pypi.org/search/?q=holmes
2. Check GitHub for installation instructions: https://github.com/robusta-dev/holmesgpt
3. Try alternative package names:
   ```bash
   pip install holmesgpt
   pip install holmes-gpt
   pip install robusta-holmes
   ```
4. Install from source:
   ```bash
   git clone https://github.com/robusta-dev/holmesgpt.git
   cd holmesgpt
   pip install -e .
   ```

---

### Issue: SDK installed but investigation fails

**Symptoms**:
```
✅ SUCCESS: HolmesGPT SDK imported successfully
❌ UNEXPECTED ERROR: 'Client' object has no attribute 'investigate'
```

**Solutions**:
1. Check SDK version:
   ```bash
   pip show holmes-ai  # or appropriate package name
   ```
2. Consult SDK documentation for correct API usage
3. Update investigation script to match actual SDK API

---

### Issue: Want to test with real investigation

**Note**: Investigation script does NOT require API key or K8s cluster by default (only inspects SDK structure).

**To test runtime behavior** (optional):
1. Set LLM API key:
   ```bash
   export LLM_API_KEY="sk-your-api-key"
   ```
2. Configure Kubernetes access:
   ```bash
   export KUBECONFIG=/path/to/kubeconfig
   ```
3. Modify investigation script to add runtime test (Task 6)

---

## Next Steps After Phase 1

**Once investigation complete**:

1. **Review findings** with team
   - Present investigation report
   - Discuss implementation approach
   - Get approval for selected scenario

2. **Update BR-HAPI-189** with findings
   - Document error classification approach
   - Update acceptance criteria based on SDK capabilities
   - Revise confidence assessment

3. **Proceed to Phase 2**: Implementation
   - Create implementation tasks based on selected approach
   - Develop unit tests for error classification
   - Implement error tracking in `HolmesGPTService`

---

## Timeline

**Estimated Phase 1 Duration**: 1-2 hours

| Task | Time | Status |
|------|------|--------|
| Install SDK | 10 min | ⏳ Pending |
| Run investigation script | 5 min | ⏳ Pending |
| Analyze findings | 30 min | ⏳ Pending |
| Document findings | 30 min | ⏳ Pending |
| Select approach | 15 min | ⏳ Pending |
| **Total** | **1.5 hours** | |

---

## Success Criteria

**Phase 1 is complete when**:

- ✅ Investigation script executed successfully
- ✅ All SDK capabilities documented
- ✅ Implementation approach selected with confidence assessment
- ✅ Findings documented in BR-HAPI-189-PHASE1-SDK-INVESTIGATION.md
- ✅ Team approval obtained for implementation approach

---

## Quick Reference

**Key Files**:
- Investigation script: `scripts/investigate_holmesgpt_sdk.py`
- Investigation doc: `docs/requirements/BR-HAPI-189-PHASE1-SDK-INVESTIGATION.md`
- BR specification: `docs/requirements/BR-HAPI-VALIDATION-RESILIENCE.md`

**Key Commands**:
```bash
# Install SDK
pip install holmes-ai

# Run investigation
python3 scripts/investigate_holmesgpt_sdk.py | tee docs/requirements/sdk_investigation_report_$(date +%Y%m%d).txt

# View findings
cat docs/requirements/sdk_investigation_report_$(date +%Y%m%d).txt
```

**Decision Points**:
1. Does SDK provide structured exceptions? → Use Scenario A
2. SDK uses generic exceptions? → Use Scenario B (multi-tier)
3. Can we wrap SDK? → Use Scenario C (recommended)

---

## Contact

**Questions?**
- HolmesGPT Docs: https://docs.robusta.dev/holmesgpt
- HolmesGPT GitHub: https://github.com/robusta-dev/holmesgpt
- Phase 1 Lead: [Your Name]

