#!/bin/bash
# Generate E2E Coverage Reports for Go Services
# Usage: ./scripts/generate-e2e-coverage.sh <service-name> <coverdata-dir> <output-dir>
# Example: ./scripts/generate-e2e-coverage.sh notification ./test/e2e/notification/coverdata ./test/e2e/notification
#
# This script follows DD-TEST-007: E2E Coverage Capture Standard
# Reference: docs/architecture/decisions/DD-TEST-007-e2e-coverage-capture-standard.md

set -euo pipefail

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Input validation
if [ $# -ne 3 ]; then
    echo -e "${RED}Error: Wrong number of arguments${NC}"
    echo "Usage: $0 <service-name> <coverdata-dir> <output-dir>"
    echo "Example: $0 notification ./test/e2e/notification/coverdata ./test/e2e/notification"
    exit 1
fi

SERVICE_NAME="$1"
COVERDATA_DIR="$2"
OUTPUT_DIR="$3"

# Output files
TEXT_REPORT="${OUTPUT_DIR}/e2e-coverage.txt"
HTML_REPORT="${OUTPUT_DIR}/e2e-coverage.html"
FUNC_REPORT="${OUTPUT_DIR}/e2e-coverage-func.txt"

echo "════════════════════════════════════════════════════════════════════════"
echo -e "${BLUE}📊 ${SERVICE_NAME} Service - E2E Coverage Report Generation${NC}"
echo "════════════════════════════════════════════════════════════════════════"
echo "Coverage data: ${COVERDATA_DIR}"
echo "Output directory: ${OUTPUT_DIR}"
echo ""

# Check if coverage data exists
if [ ! -d "${COVERDATA_DIR}" ]; then
    echo -e "${RED}⚠️  Coverage data directory not found: ${COVERDATA_DIR}${NC}"
    echo "Did the controller shut down gracefully to flush coverage data?"
    exit 1
fi

# Check if coverage data is non-empty
if [ -z "$(ls -A "${COVERDATA_DIR}" 2>/dev/null)" ]; then
    echo -e "${RED}⚠️  Coverage data directory is empty: ${COVERDATA_DIR}${NC}"
    echo "Possible causes:"
    echo "  • Controller built without GOFLAGS=-cover"
    echo "  • GOCOVERDIR not set in deployment"
    echo "  • Controller crashed before coverage flush"
    echo "  • Permission issues (coverdata directory not writable)"
    exit 1
fi

# Create output directory if it doesn't exist
mkdir -p "${OUTPUT_DIR}"

echo -e "${BLUE}Step 1: Generating text coverage report...${NC}"
if go tool covdata textfmt -i="${COVERDATA_DIR}" -o="${TEXT_REPORT}"; then
    echo -e "${GREEN}   ✅ Text report: ${TEXT_REPORT}${NC}"
else
    echo -e "${RED}   ❌ Failed to generate text report${NC}"
    exit 1
fi

echo ""
echo -e "${BLUE}Step 2: Generating HTML coverage report...${NC}"
if go tool cover -html="${TEXT_REPORT}" -o="${HTML_REPORT}"; then
    echo -e "${GREEN}   ✅ HTML report: ${HTML_REPORT}${NC}"
else
    echo -e "${RED}   ❌ Failed to generate HTML report${NC}"
    exit 1
fi

echo ""
echo -e "${BLUE}Step 3: Generating function-level coverage report...${NC}"
if go tool covdata func -i="${COVERDATA_DIR}" -o="${FUNC_REPORT}"; then
    echo -e "${GREEN}   ✅ Function report: ${FUNC_REPORT}${NC}"
else
    echo -e "${YELLOW}   ⚠️  Failed to generate function report (non-critical)${NC}"
fi

echo ""
echo "════════════════════════════════════════════════════════════════════════"
echo -e "${GREEN}📈 Coverage Summary for ${SERVICE_NAME} Service:${NC}"
echo "════════════════════════════════════════════════════════════════════════"
go tool covdata percent -i="${COVERDATA_DIR}"

echo ""
echo "════════════════════════════════════════════════════════════════════════"
echo -e "${GREEN}✅ Coverage Reports Generated Successfully${NC}"
echo "════════════════════════════════════════════════════════════════════════"
echo "  📄 Text:     ${TEXT_REPORT}"
echo "  🌐 HTML:     ${HTML_REPORT}"
echo "  📊 Function: ${FUNC_REPORT}"
echo "  📁 Data:     ${COVERDATA_DIR}"
echo ""
echo -e "${BLUE}💡 View HTML report:${NC}"
echo "   open ${HTML_REPORT}"
echo "════════════════════════════════════════════════════════════════════════"



