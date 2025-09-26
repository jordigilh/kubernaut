#!/bin/bash
# test-violation-prevention.sh - Validate that enhanced rules prevent original violation

echo "🧪 TESTING VIOLATION PREVENTION - Original ContextOptimizer Scenario"
echo "=================================================================="
echo ""

EXIT_CODE=0

echo "📋 TEST 1: Integration Check (Would have caught standalone ContextOptimizer)"
echo "------------------------------------------------------------------------"
./scripts/integration-check-main-usage.sh ContextOptimizer
if [ $? -eq 1 ]; then
    echo "✅ PASS: Integration script correctly identifies ContextOptimizer violation"
else
    echo "❌ FAIL: Integration script missed ContextOptimizer violation"
    EXIT_CODE=1
fi
echo ""

echo "📋 TEST 2: AI Development Phase Validation (Would have caught REFACTOR violation)"
echo "-------------------------------------------------------------------------------"
./scripts/validate-ai-development.sh refactor
if [ $? -eq 1 ]; then
    echo "✅ PASS: AI development script correctly identifies REFACTOR phase violation"
else
    echo "❌ FAIL: AI development script missed REFACTOR phase violation"
    EXIT_CODE=1
fi
echo ""

echo "📋 TEST 3: Conflict Resolution (Would have provided clear guidance)"
echo "------------------------------------------------------------------"
RESOLUTION_OUTPUT=$(./scripts/resolve-rule-conflict.sh "Rule 03: REFACTOR sophistication" "Rule 07: Integration requirement" "integration")
if echo "$RESOLUTION_OUTPUT" | grep -q "Integration ALWAYS takes precedence"; then
    echo "✅ PASS: Conflict resolution provides clear integration priority guidance"
else
    echo "❌ FAIL: Conflict resolution missing integration priority guidance"
    EXIT_CODE=1
fi
echo ""

echo "📋 TEST 4: Rule 12 AI/ML TDD (Would have enforced proper AI development)"
echo "-----------------------------------------------------------------------"
# Simulate Rule 12 guidance check
if [ -f ".cursor/rules/12-ai-ml-development-methodology.mdc" ]; then
    if grep -q "REFACTOR NEVER MEANS.*Create new parallel" .cursor/rules/12-ai-ml-development-methodology.mdc; then
        echo "✅ PASS: Rule 12 clearly prohibits parallel component creation during REFACTOR"
    else
        echo "❌ FAIL: Rule 12 missing clear REFACTOR prohibition"
        EXIT_CODE=1
    fi
else
    echo "❌ FAIL: Rule 12 AI/ML development methodology not found"
    EXIT_CODE=1
fi
echo ""

echo "📋 TEST 5: Enhanced Rule 03 TDD (Would have clarified REFACTOR scope)"
echo "--------------------------------------------------------------------"
if [ -f ".cursor/rules/03-testing-strategy.mdc" ]; then
    if grep -q "REFACTOR NEVER MEANS.*Create new parallel" .cursor/rules/03-testing-strategy.mdc; then
        echo "✅ PASS: Enhanced Rule 03 clearly defines REFACTOR scope"
    else
        echo "❌ FAIL: Enhanced Rule 03 missing clear REFACTOR definition"
        EXIT_CODE=1
    fi
else
    echo "❌ FAIL: Enhanced Rule 03 not found"
    EXIT_CODE=1
fi
echo ""

echo "📋 TEST 6: Rule 00 Integration Checkpoints (Would have triggered during development)"
echo "-----------------------------------------------------------------------------------"
if [ -f ".cursor/rules/00-project-guidelines.mdc" ]; then
    if grep -q "CHECKPOINT.*Before Creating ANY New Type" .cursor/rules/00-project-guidelines.mdc; then
        echo "✅ PASS: Rule 00 has mandatory checkpoints for new type creation"
    else
        echo "❌ FAIL: Rule 00 missing integration checkpoints"
        EXIT_CODE=1
    fi
else
    echo "❌ FAIL: Enhanced Rule 00 not found"
    EXIT_CODE=1
fi
echo ""

echo "📋 TEST 7: TDD Business Requirements Completeness (Would have ensured complete BR coverage)"
echo "----------------------------------------------------------------------------------------"
if [ -f ".cursor/rules/03-testing-strategy.mdc" ]; then
    if grep -q "ALL of those targeted.*MUST be covered by TDD" .cursor/rules/03-testing-strategy.mdc; then
        echo "✅ PASS: Rule 03 requires complete coverage of targeted business requirements"
    else
        echo "❌ FAIL: Rule 03 missing TDD completeness requirement"
        EXIT_CODE=1
    fi
else
    echo "❌ FAIL: Enhanced Rule 03 not found"
    EXIT_CODE=1
fi

# Test the validation script
if [ -f "scripts/validate-tdd-completeness.sh" ]; then
    # Test with existing BRs (should pass)
    ./scripts/validate-tdd-completeness.sh "BR-CONTEXT-OPT-001,BR-CONTEXT-OPT-002" > /dev/null 2>&1
    if [ $? -eq 0 ]; then
        echo "✅ PASS: TDD completeness script correctly validates existing BR coverage"
    else
        echo "❌ FAIL: TDD completeness script failed on existing BRs"
        EXIT_CODE=1
    fi

    # Test with missing BR (should fail)
    ./scripts/validate-tdd-completeness.sh "BR-CONTEXT-OPT-001,BR-MISSING-TEST" > /dev/null 2>&1
    if [ $? -eq 1 ]; then
        echo "✅ PASS: TDD completeness script correctly detects missing BR coverage"
    else
        echo "❌ FAIL: TDD completeness script missed missing BR"
        EXIT_CODE=1
    fi
else
    echo "❌ FAIL: TDD completeness validation script not found"
    EXIT_CODE=1
fi
echo ""

echo "=================================================================="
if [ $EXIT_CODE -eq 0 ]; then
    echo "🎉 ALL TESTS PASSED: Enhanced rules would have prevented the original violation"
    echo ""
    echo "🛡️  VIOLATION PREVENTION SUMMARY:"
    echo "1. ✅ Integration script detects orphaned components"
    echo "2. ✅ AI development script enforces REFACTOR phase rules"
    echo "3. ✅ Conflict resolution prioritizes integration"
    echo "4. ✅ Rule 12 prohibits parallel component creation"
    echo "5. ✅ Enhanced Rule 03 clarifies REFACTOR scope"
    echo "6. ✅ Rule 00 provides mandatory checkpoints"
    echo "7. ✅ TDD completeness ensures ALL targeted BRs are covered"
    echo ""
    echo "🔒 RESULT: The ContextOptimizer violation scenario is now IMPOSSIBLE"
else
    echo "❌ SOME TESTS FAILED: Rule enhancements need refinement"
    echo ""
    echo "⚠️  FAILED CHECKS:"
    echo "- Review failed test output above"
    echo "- Verify rule content and script functionality"
    echo "- Ensure all automation scripts are properly configured"
fi

echo ""
echo "🎯 ORIGINAL VIOLATION ANALYSIS:"
echo "WHAT HAPPENED: Created ContextOptimizer as standalone component during 'REFACTOR' phase"
echo "WHY IT HAPPENED: Ambiguous rules allowed multiple interpretations"
echo "HOW PREVENTED NOW:"
echo "  • Clear REFACTOR definition: 'Enhance EXACT SAME code that tests are calling'"
echo "  • Mandatory integration checkpoints: Before creating ANY new type"
echo "  • Automated detection: Scripts catch violations immediately"
echo "  • Crystal clear guidance: No ambiguity in rule interpretation"
echo "  • Conflict resolution: Automated priority resolution"

exit $EXIT_CODE
