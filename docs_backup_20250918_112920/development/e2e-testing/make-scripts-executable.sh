#!/bin/bash
# Make all E2E testing scripts executable
# This script ensures all bash scripts in the e2e-testing directory have execute permissions

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }

echo -e "${BLUE}Making all E2E testing scripts executable...${NC}"

# List of scripts to make executable
SCRIPTS=(
    "setup-complete-e2e-environment.sh"
    "cleanup-e2e-environment.sh"
    "validate-complete-e2e-environment.sh"
    "deploy-kcli-cluster.sh"
    "setup-storage.sh"
    "validate-baremetal-setup.sh"
    "make-scripts-executable.sh"
)

# Make scripts executable
for script in "${SCRIPTS[@]}"; do
    if [[ -f "${SCRIPT_DIR}/${script}" ]]; then
        chmod +x "${SCRIPT_DIR}/${script}"
        log_success "Made executable: ${script}"
    else
        log_info "Script not found: ${script}"
    fi
done

# Make any additional shell scripts executable
find "${SCRIPT_DIR}" -name "*.sh" -type f -exec chmod +x {} \;

# Make run-* scripts executable if they exist
if ls "${SCRIPT_DIR}"/run-*.sh >/dev/null 2>&1; then
    chmod +x "${SCRIPT_DIR}"/run-*.sh
    log_success "Made run-* scripts executable"
fi

log_success "All E2E testing scripts are now executable!"

echo -e "\n${BLUE}Quick Start Commands:${NC}"
echo -e "  Deploy complete environment: ./setup-complete-e2e-environment.sh"
echo -e "  Validate environment:        ./validate-complete-e2e-environment.sh --detailed"
echo -e "  Cleanup environment:         ./cleanup-e2e-environment.sh"
echo ""
