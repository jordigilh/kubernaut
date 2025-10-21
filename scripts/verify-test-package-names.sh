#!/bin/bash
# Script to verify test package naming convention compliance
# Checks that all test files use 'package xxx' (not 'package xxx_test')
#
# Usage: ./scripts/verify-test-package-names.sh
#
# Exit codes:
#   0 - All test files follow correct convention
#   1 - Violations found

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Test Package Naming Convention Verifier${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo ""

# Counters
TOTAL_FILES=0
COMPLIANT_FILES=0
VIOLATING_FILES=0

# Array to store violations
declare -a VIOLATIONS

# Find all test files
echo -e "${BLUE}Scanning test files...${NC}\n"

while IFS= read -r file; do
    TOTAL_FILES=$((TOTAL_FILES + 1))

    # Extract package name
    package_name=$(grep -m 1 "^package " "$file" 2>/dev/null | awk '{print $2}')

    # Check if package name ends with _test
    if [[ "$package_name" =~ _test$ ]]; then
        VIOLATIONS+=("$file:$package_name")
        VIOLATING_FILES=$((VIOLATING_FILES + 1))
    else
        COMPLIANT_FILES=$((COMPLIANT_FILES + 1))
    fi
done < <(find test/ -name "*_test.go" -type f 2>/dev/null)

# Report results
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Results${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "Total test files: ${TOTAL_FILES}"
echo -e "${GREEN}Compliant:${NC}       ${COMPLIANT_FILES}"
echo -e "${RED}Violations:${NC}      ${VIOLATING_FILES}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo ""

# Show violations if any
if [[ $VIOLATING_FILES -gt 0 ]]; then
    echo -e "${RED}✗ Found ${VIOLATING_FILES} files with incorrect package naming:${NC}\n"

    for violation in "${VIOLATIONS[@]}"; do
        file="${violation%%:*}"
        package="${violation##*:}"
        correct_package="${package%_test}"
        echo -e "  ${RED}✗${NC} $file"
        echo -e "    Current:  ${RED}package $package${NC}"
        echo -e "    Expected: ${GREEN}package $correct_package${NC}"
        echo ""
    done

    echo -e "${YELLOW}To fix these violations, run:${NC}"
    echo -e "  ${BLUE}./scripts/fix-test-package-names.sh --dry-run${NC}  # Preview changes"
    echo -e "  ${BLUE}./scripts/fix-test-package-names.sh${NC}            # Apply fixes"
    echo ""

    exit 1
else
    echo -e "${GREEN}✓ All test files follow correct package naming convention!${NC}"
    echo ""
    echo -e "Correct convention:"
    echo -e "  File:    ${BLUE}component_test.go${NC}"
    echo -e "  Package: ${GREEN}package component${NC} (internal test package)"
    echo ""

    exit 0
fi

