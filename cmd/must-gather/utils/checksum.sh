#!/bin/bash
# Copyright 2025 Jordi Gil
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Kubernaut Must-Gather - Checksum Generator
# BR-PLATFORM-001.8: Generate SHA256 checksums for archive integrity

set -euo pipefail

COLLECTION_DIR="${1}"

echo "Generating SHA256 checksums..."

cd "${COLLECTION_DIR}" || exit 1

# Generate checksums for all collected files
find . -type f ! -name "SHA256SUMS" -exec sha256sum {} \; > SHA256SUMS 2>/dev/null || {
    echo "Warning: Failed to generate checksums"
    exit 0
}

# Count checksums generated
CHECKSUM_COUNT=$(wc -l < SHA256SUMS 2>/dev/null || echo "0")

echo "Generated ${CHECKSUM_COUNT} checksums"
echo "Checksum file: ${COLLECTION_DIR}/SHA256SUMS"
echo ""
echo "To verify checksums:"
echo "  cd $(basename "${COLLECTION_DIR}") && sha256sum -c SHA256SUMS"

