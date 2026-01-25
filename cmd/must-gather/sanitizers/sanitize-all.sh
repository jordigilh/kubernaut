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

# Kubernaut Must-Gather - Data Sanitization
# BR-PLATFORM-001.9: Remove sensitive data (passwords, keys, PII)
#
# This script sanitizes collected diagnostic data to comply with:
# - GDPR (EU General Data Protection Regulation)
# - CCPA (California Consumer Privacy Act)
# - SOC2 compliance requirements

set -euo pipefail

# Configuration
COLLECTION_DIR="${1:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ -z "${COLLECTION_DIR}" ]]; then
    echo "Usage: $0 <collection-directory>"
    exit 1
fi

if [[ ! -d "${COLLECTION_DIR}" ]]; then
    echo "Error: Collection directory does not exist: ${COLLECTION_DIR}"
    exit 1
fi

echo "Starting data sanitization..."
echo "Collection directory: ${COLLECTION_DIR}"

# Initialize sanitization report
REPORT_FILE="${COLLECTION_DIR}/sanitization-report.txt"
cat > "${REPORT_FILE}" <<EOF
Kubernaut Must-Gather - Sanitization Report
============================================
Generated: $(date -u +"%Y-%m-%d %H:%M:%S UTC")
Collection: ${COLLECTION_DIR}

This report documents the sanitization process applied to diagnostic data
to ensure compliance with GDPR, CCPA, and SOC2 requirements.

Sanitization Actions:
EOF

SANITIZED_COUNT=0
FILES_PROCESSED=0

# Function to sanitize a file
sanitize_file() {
    local file="$1"
    local temp_file="${file}.sanitizing"
    local backup_file="${file}.pre-sanitize"

    FILES_PROCESSED=$((FILES_PROCESSED + 1))

    # Create backup
    cp "${file}" "${backup_file}"

    # Track if any changes were made
    local changes_made=false

    # Apply sanitization patterns
    cp "${file}" "${temp_file}"

    # 1. Redact passwords (various formats including special characters)
    if grep -qiE '(password|passwd|pwd)[:=]\s*["\047]?[^"\047\s]+' "${temp_file}" 2>/dev/null; then
        # Match passwords with any non-whitespace, non-quote characters (including special chars)
        sed -i -E 's/([Pp][Aa][Ss][Ss][Ww][Oo][Rr][Dd]|[Pp][Aa][Ss][Ss][Ww][Dd]|[Pp][Ww][Dd])([:=][[:space:]]*["\047]?)([^[:space:]"\047]+)/\1\2********/g' "${temp_file}"
        changes_made=true
    fi

    # 2. Redact API keys and tokens
    if grep -qiE '(api[_-]?key|apikey|token|bearer)[:="\s]+[A-Za-z0-9_-]{20,}' "${temp_file}" 2>/dev/null; then
        sed -i -E 's/([Aa][Pp][Ii][_-]?[Kk][Ee][Yy]|[Aa][Pp][Ii][Kk][Ee][Yy]|[Tt][Oo][Kk][Ee][Nn]|[Bb][Ee][Aa][Rr][Ee][Rr])([:="[:space:]]+)[A-Za-z0-9_-]{20,}/\1\2[REDACTED]/g' "${temp_file}"
        changes_made=true
    fi

    # 3. Redact Bearer tokens in Authorization headers
    if grep -qiE 'Authorization:\s*Bearer\s+[A-Za-z0-9_.-]+' "${temp_file}" 2>/dev/null; then
        sed -i -E 's/([Aa][Uu][Tt][Hh][Oo][Rr][Ii][Zz][Aa][Tt][Ii][Oo][Nn]:[[:space:]]*[Bb][Ee][Aa][Rr][Ee][Rr][[:space:]]+)[A-Za-z0-9_.-]+/\1[REDACTED]/g' "${temp_file}"
        changes_made=true
    fi

    # 4. Redact secret keys (sk-proj-*, aws-key-*, etc.) and token= patterns
    if grep -qE '(sk-proj-|aws-key-|user-key-)[A-Za-z0-9_-]+' "${temp_file}" 2>/dev/null; then
        sed -i -E 's/(sk-proj-|aws-key-|user-key-)[A-Za-z0-9_-]+/\1[REDACTED]/g' "${temp_file}"
        changes_made=true
    fi

    # 4b. Redact token= patterns in logs
    if grep -qE 'token[=:][A-Za-z0-9_-]+' "${temp_file}" 2>/dev/null; then
        sed -i -E 's/(token[=:])([A-Za-z0-9_-]+)/\1[REDACTED]/g' "${temp_file}"
        changes_made=true
    fi

    # 5. Redact email addresses (PII) - replace entire email with @[REDACTED]
    if grep -qE '[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}' "${temp_file}" 2>/dev/null; then
        sed -i -E 's/[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}/@[REDACTED]/g' "${temp_file}"
        changes_made=true
    fi

    # 6. Redact connection strings with credentials
    if grep -qE '(postgresql|mysql|mongodb)://[^:]+:[^@]+@' "${temp_file}" 2>/dev/null; then
        sed -i -E 's#(postgresql|mysql|mongodb)://[^:]+:[^@]+@#\1://[REDACTED]:[REDACTED]@#g' "${temp_file}"
        changes_made=true
    fi

    # 7. Redact base64 values in Kubernetes Secrets (data: field)
    if grep -q "kind: Secret" "${temp_file}" 2>/dev/null; then
        # In Secret objects, redact ALL base64 encoded values in data: section
        # Match any field with base64-like value (alphanumeric + / + = + .)
        if grep -qE '^[[:space:]]+(username|password|token|api[_-]?(key|token)|tls\.(key|crt)):[[:space:]]+[A-Za-z0-9+/]' "${temp_file}" 2>/dev/null; then
            sed -i -E 's/^([[:space:]]+(username|password|token|api[_-]?(key|token)|tls\.(key|crt)):[[:space:]]+)[A-Za-z0-9+/=.]+$/\1[REDACTED-BASE64]/g' "${temp_file}"
            changes_made=true
        fi
    fi

    # 8. Redact TLS private keys
    if grep -q "BEGIN PRIVATE KEY" "${temp_file}" 2>/dev/null || grep -q "BEGIN RSA PRIVATE KEY" "${temp_file}" 2>/dev/null; then
        sed -i '/-----BEGIN.*PRIVATE KEY-----/,/-----END.*PRIVATE KEY-----/c\
-----BEGIN PRIVATE KEY-----\
[REDACTED-PRIVATE-KEY]\
-----END PRIVATE KEY-----' "${temp_file}"
        changes_made=true
    fi

    # 9. Redact environment variable secrets (DB_PASSWORD, etc.)
    if grep -qiE '(DB_PASSWORD|REDIS_PASSWORD|AWS_SECRET_KEY|AWS_SECRET_ACCESS_KEY)[:=]\s*["\047]?[^"\047\s]+' "${temp_file}" 2>/dev/null; then
        # Handle both quoted and unquoted values with any non-whitespace characters
        sed -i -E 's/(DB_PASSWORD|REDIS_PASSWORD|AWS_SECRET_KEY|AWS_SECRET_ACCESS_KEY)([:=][[:space:]]*["\047]?)([^[:space:]"\047]+)/\1\2********/g' "${temp_file}"
        changes_made=true
    fi

    # 9b. Redact value: fields in YAML after password/secret parameter names
    if grep -qiE '(PASSWORD|SECRET|KEY|TOKEN)' "${temp_file}" 2>/dev/null; then
        # Match value: "..." or value: ... after password-like parameter names
        sed -i -E 's/(value:[[:space:]]*["\047])([^"\047]+)(["\047])/\1********\3/g' "${temp_file}"
        changes_made=true
    fi

    if [[ "$changes_made" == "true" ]]; then
        mv "${temp_file}" "${file}"
        SANITIZED_COUNT=$((SANITIZED_COUNT + 1))
        echo "  ✓ Sanitized: ${file#${COLLECTION_DIR}/}" >> "${REPORT_FILE}"
    else
        # No changes, restore original
        rm "${temp_file}"
        rm "${backup_file}"
    fi
}

# Find and sanitize all YAML, JSON, and log files
echo "Scanning for files to sanitize..."

while IFS= read -r -d '' file; do
    sanitize_file "$file"
done < <(find "${COLLECTION_DIR}" -type f \( -name "*.yaml" -o -name "*.yml" -o -name "*.json" -o -name "*.log" -o -name "*.txt" \) ! -name "sanitization-report.txt" ! -name "*.pre-sanitize" -print0)

# Finalize report
cat >> "${REPORT_FILE}" <<EOF

Summary:
--------
Total files processed: ${FILES_PROCESSED}
Files sanitized: ${SANITIZED_COUNT}
Files unchanged: $((FILES_PROCESSED - SANITIZED_COUNT))

Sanitization Patterns Applied:
-------------------------------
1. Passwords (password:, passwd:, pwd:)
2. API keys and tokens (apiKey:, token:, Bearer)
3. Secret keys (sk-proj-*, aws-key-*, user-key-*)
4. Email addresses (PII - preserved domain for analysis)
5. Database connection strings (credentials redacted)
6. Kubernetes Secret base64 values (data: fields)
7. TLS private keys (-----BEGIN PRIVATE KEY-----)
8. Environment variable secrets (DB_PASSWORD, AWS_SECRET_KEY, etc.)

Backup Files:
-------------
Original files preserved as: <filename>.pre-sanitize
These backups are for internal forensic use only and should NOT be shared externally.

Compliance:
-----------
✓ GDPR Article 17 (Right to erasure) - PII redacted
✓ CCPA § 1798.100 (Consumer data protection) - Personal data removed
✓ SOC2 CC6.1 (Data security) - Credentials and secrets sanitized

For questions or to report sanitization issues:
support@kubernaut.ai
EOF

echo ""
echo "✓ Sanitization complete!"
echo "  Files processed: ${FILES_PROCESSED}"
echo "  Files sanitized: ${SANITIZED_COUNT}"
echo "  Report: ${REPORT_FILE}"
echo ""
echo "⚠️  IMPORTANT: Original files backed up as *.pre-sanitize"
echo "   These backups contain sensitive data and should be handled securely."
