#!/bin/bash
# Script to fix package naming convention in test files
# Replaces 'package xxx_test' with 'package xxx' (internal test packages)
#
# Usage: ./scripts/fix-test-package-names.sh [--dry-run]
#
# Background:
# - Go supports two test package patterns:
#   1. Internal: 'package xxx' - tests can access unexported functions
#   2. External: 'package xxx_test' - tests can only access exported functions
# - Kubernaut uses internal test packages as the standard convention
# - This script fixes 104 test files that incorrectly use external pattern

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Parse arguments
DRY_RUN=false
if [[ "${1:-}" == "--dry-run" ]]; then
    DRY_RUN=true
    echo -e "${YELLOW}DRY RUN MODE - No files will be modified${NC}\n"
fi

# Counters
TOTAL_FILES=0
FIXED_FILES=0
SKIPPED_FILES=0
ERROR_FILES=0

echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Test Package Name Convention Fixer${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo ""
echo -e "${GREEN}Correct Convention:${NC}"
echo -e "  File:    component_test.go"
echo -e "  Package: package component  ${GREEN}(NO _test suffix)${NC}"
echo ""
echo -e "${RED}Incorrect Convention (will fix):${NC}"
echo -e "  Package: package component_test  ${RED}(WITH _test suffix)${NC}"
echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo ""

# Function to fix a single test file
fix_test_file() {
    local file="$1"

    TOTAL_FILES=$((TOTAL_FILES + 1))

    # Check if file exists
    if [[ ! -f "$file" ]]; then
        echo -e "${RED}✗${NC} File not found: $file"
        ERROR_FILES=$((ERROR_FILES + 1))
        return 1
    fi

    # Extract the package name with _test suffix
    local current_package
    current_package=$(grep -m 1 "^package " "$file" | awk '{print $2}')

    # Check if it has _test suffix
    if [[ ! "$current_package" =~ _test$ ]]; then
        echo -e "${YELLOW}○${NC} Already correct: $file (package $current_package)"
        SKIPPED_FILES=$((SKIPPED_FILES + 1))
        return 0
    fi

    # Remove _test suffix to get correct package name
    local correct_package="${current_package%_test}"

    # Perform the replacement
    if [[ "$DRY_RUN" == true ]]; then
        echo -e "${BLUE}→${NC} Would fix: $file"
        echo -e "  ${RED}package $current_package${NC} → ${GREEN}package $correct_package${NC}"
        FIXED_FILES=$((FIXED_FILES + 1))
    else
        # Use sed to replace the package declaration (macOS compatible)
        if sed -i '' "s/^package ${current_package}$/package ${correct_package}/" "$file" 2>/dev/null; then
            echo -e "${GREEN}✓${NC} Fixed: $file"
            echo -e "  ${RED}package $current_package${NC} → ${GREEN}package $correct_package${NC}"
            FIXED_FILES=$((FIXED_FILES + 1))
        else
            echo -e "${RED}✗${NC} Failed to fix: $file"
            ERROR_FILES=$((ERROR_FILES + 1))
            return 1
        fi
    fi
}

# Find all test files with package xxx_test declaration
echo -e "${BLUE}Scanning test files...${NC}\n"

# Array to store all violating files
declare -a VIOLATING_FILES

# Find all test files and check for package xxx_test
while IFS= read -r file; do
    # Check if file has package xxx_test declaration
    if grep -q "^package [a-zA-Z_]*_test$" "$file" 2>/dev/null; then
        VIOLATING_FILES+=("$file")
    fi
done < <(find test/ -name "*_test.go" -type f 2>/dev/null)

# Report findings
VIOLATION_COUNT=${#VIOLATING_FILES[@]}
echo -e "${YELLOW}Found ${VIOLATION_COUNT} files with incorrect package naming${NC}\n"

if [[ $VIOLATION_COUNT -eq 0 ]]; then
    echo -e "${GREEN}✓ All test files follow correct package naming convention!${NC}"
    exit 0
fi

# Ask for confirmation in non-dry-run mode
if [[ "$DRY_RUN" == false ]]; then
    echo -e "${YELLOW}This will modify ${VIOLATION_COUNT} files. Continue? [y/N]${NC} "
    read -r response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
        echo -e "${RED}Aborted by user${NC}"
        exit 1
    fi
    echo ""
fi

# Process each violating file
echo -e "${BLUE}Processing files...${NC}\n"

for file in "${VIOLATING_FILES[@]}"; do
    fix_test_file "$file"
done

# Summary
echo ""
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "${BLUE}  Summary${NC}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"
echo -e "Total files scanned: ${TOTAL_FILES}"
echo -e "${GREEN}Fixed:${NC}              ${FIXED_FILES}"
echo -e "${YELLOW}Already correct:${NC}    ${SKIPPED_FILES}"
echo -e "${RED}Errors:${NC}             ${ERROR_FILES}"
echo -e "${BLUE}═══════════════════════════════════════════════════════════${NC}"

if [[ "$DRY_RUN" == true ]]; then
    echo ""
    echo -e "${YELLOW}DRY RUN COMPLETE - No files were modified${NC}"
    echo -e "${YELLOW}Run without --dry-run to apply changes${NC}"
fi

# Exit with error if any files failed
if [[ $ERROR_FILES -gt 0 ]]; then
    exit 1
fi

# Success message
if [[ "$DRY_RUN" == false ]] && [[ $FIXED_FILES -gt 0 ]]; then
    echo ""
    echo -e "${GREEN}✓ Successfully fixed ${FIXED_FILES} test files${NC}"
    echo ""
    echo -e "${YELLOW}Next steps:${NC}"
    echo -e "  1. Run tests to verify changes: ${BLUE}make test${NC}"
    echo -e "  2. Review changes: ${BLUE}git diff test/${NC}"
    echo -e "  3. Commit changes: ${BLUE}git add test/ && git commit -m 'fix: standardize test package naming convention'${NC}"
fi

exit 0

