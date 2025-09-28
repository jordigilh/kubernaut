#!/bin/bash

# Validate NO MOCKS Policy in Integration and E2E Tests
# This script enforces the ZERO MOCKS policy for integration and E2E tests

set -e

PROJECT_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_ROOT"

echo "🔍 Validating NO MOCKS policy in integration and E2E tests..."
echo "Policy: Integration and E2E tests MUST use REAL business components - NO MOCKS"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

VIOLATIONS_FOUND=0

# Function to report violation
report_violation() {
    echo -e "${RED}❌ POLICY VIOLATION:${NC} $1"
    VIOLATIONS_FOUND=1
}

# Function to report success
report_success() {
    echo -e "${GREEN}✅${NC} $1"
}

# Function to report warning
report_warning() {
    echo -e "${YELLOW}⚠️${NC} $1"
}

echo "📋 Checking integration tests (test/integration/)..."

# Check for forbidden mock patterns in integration tests
if [ -d "test/integration" ]; then
    INTEGRATION_MOCK_VIOLATIONS=$(grep -r -n "Mock\|Stub\|Fake" test/integration/ --include="*.go" | grep -v "// ✅" | grep -v "// ALLOWED" || true)

    if [ -n "$INTEGRATION_MOCK_VIOLATIONS" ]; then
        report_violation "Mocks found in integration tests:"
        echo "$INTEGRATION_MOCK_VIOLATIONS"
        echo ""
        echo "Integration tests MUST use REAL business components."
        echo "See docs/testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md"
        echo ""
    else
        report_success "No mock violations found in integration tests"
    fi

    # Check for real component usage in integration tests
    REAL_COMPONENTS_INTEGRATION=$(grep -r "NewProcessor\|NewHandler\|NewClient\|NewExecutor\|NewRepository" test/integration/ --include="*.go" | wc -l)

    if [ "$REAL_COMPONENTS_INTEGRATION" -gt 0 ]; then
        report_success "Found $REAL_COMPONENTS_INTEGRATION real business component usages in integration tests"
    else
        report_warning "No real business components found in integration tests"
    fi
else
    report_warning "No test/integration/ directory found"
fi

echo ""
echo "📋 Checking E2E tests (test/e2e/)..."

# Check for forbidden mock patterns in E2E tests
if [ -d "test/e2e" ]; then
    E2E_MOCK_VIOLATIONS=$(grep -r -n "Mock\|Stub\|Fake" test/e2e/ --include="*.go" | grep -v "// ✅" | grep -v "// ALLOWED" || true)

    if [ -n "$E2E_MOCK_VIOLATIONS" ]; then
        report_violation "Mocks found in E2E tests:"
        echo "$E2E_MOCK_VIOLATIONS"
        echo ""
        echo "E2E tests MUST test complete real workflows."
        echo "See docs/testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md"
        echo ""
    else
        report_success "No mock violations found in E2E tests"
    fi

    # Check for real component usage in E2E tests
    REAL_COMPONENTS_E2E=$(grep -r "NewProcessor\|NewHandler\|NewClient\|NewExecutor\|NewRepository" test/e2e/ --include="*.go" | wc -l)

    if [ "$REAL_COMPONENTS_E2E" -gt 0 ]; then
        report_success "Found $REAL_COMPONENTS_E2E real business component usages in E2E tests"
    else
        report_warning "No real business components found in E2E tests"
    fi
else
    report_warning "No test/e2e/ directory found"
fi

echo ""
echo "📋 Checking for acceptable external service configuration..."

# Check for acceptable external service configuration (not mocking)
ACCEPTABLE_CONFIG=$(grep -r "Config\|Endpoint\|Provider" test/integration/ test/e2e/ --include="*.go" | grep -v "Mock" | wc -l || echo "0")

if [ "$ACCEPTABLE_CONFIG" -gt 0 ]; then
    report_success "Found $ACCEPTABLE_CONFIG external service configurations (acceptable)"
else
    report_warning "No external service configurations found"
fi

echo ""
echo "📋 Checking for real business logic usage..."

# Check for real business logic imports
REAL_BUSINESS_IMPORTS=$(grep -r "github.com/jordigilh/kubernaut/pkg/" test/integration/ test/e2e/ --include="*.go" | grep -v "testutil" | wc -l || echo "0")

if [ "$REAL_BUSINESS_IMPORTS" -gt 0 ]; then
    report_success "Found $REAL_BUSINESS_IMPORTS real business logic imports"
else
    report_warning "No real business logic imports found"
fi

echo ""
echo "📋 Checking for forbidden test patterns..."

# Check for forbidden test patterns that bypass business logic
BYPASS_PATTERNS=$(grep -r -n "return nil" test/integration/ test/e2e/ --include="*.go" | grep -E "(ProcessAlert|HandleAlert|Execute)" || true)

if [ -n "$BYPASS_PATTERNS" ]; then
    report_violation "Found patterns that may bypass business logic:"
    echo "$BYPASS_PATTERNS"
    echo ""
    echo "Integration and E2E tests must execute real business logic."
    echo ""
fi

echo ""
echo "📋 Validation Summary..."

if [ "$VIOLATIONS_FOUND" -eq 0 ]; then
    echo ""
    report_success "NO MOCKS policy validation PASSED"
    echo ""
    echo "✅ Integration and E2E tests are using REAL business components"
    echo "✅ No mock violations detected"
    echo "✅ Real business logic is being tested"
    echo ""
    echo "See docs/testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md for policy details"
    exit 0
else
    echo ""
    report_violation "NO MOCKS policy validation FAILED"
    echo ""
    echo "❌ Integration and/or E2E tests are using mocks (FORBIDDEN)"
    echo "❌ Policy requires REAL business components only"
    echo ""
    echo "🔧 To fix:"
    echo "1. Replace all mocks with real business components"
    echo "2. Use real database connections, Kubernetes clients, etc."
    echo "3. Configure external services to integration endpoints (don't mock them)"
    echo "4. See docs/testing/INTEGRATION_E2E_NO_MOCKS_POLICY.md for examples"
    echo ""
    exit 1
fi
