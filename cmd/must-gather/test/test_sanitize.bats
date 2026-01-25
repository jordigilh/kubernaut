#!/usr/bin/env bats
# Kubernaut Must-Gather - Sanitization Tests
# BR-PLATFORM-001.9: Sensitive data is protected in diagnostic collections

load helpers

setup() {
    setup_test_environment
    export SCRIPT_DIR="${MUST_GATHER_ROOT}"
}

teardown() {
    teardown_test_environment
}

# ========================================
# Business Outcome: GDPR/CCPA Compliance
# ========================================

@test "BR-PLATFORM-001.9: Support engineer cannot extract database passwords from diagnostics" {
    # Business Outcome: Compliance with data protection regulations
    # Edge Case: Password in multiple formats (plain, env var, connection string)
    mkdir -p "${MOCK_COLLECTION_DIR}/cluster-scoped"
    cat > "${MOCK_COLLECTION_DIR}/cluster-scoped/configmap.yaml" <<EOF
database:
  password: CompanySecret2026!
  connection_string: "postgresql://admin:MyP@ssw0rd@postgres:5432/db"
  DB_PASSWORD: "AnotherSecret123"
EOF

    run bash "${SANITIZERS_DIR}/sanitize-all.sh" "${MOCK_COLLECTION_DIR}"

    # Verify NO actual passwords remain (GDPR requirement)
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/cluster-scoped/configmap.yaml" "CompanySecret2026!"
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/cluster-scoped/configmap.yaml" "MyP@ssw0rd"
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/cluster-scoped/configmap.yaml" "AnotherSecret123"

    # But troubleshooting context preserved
    assert_file_contains "${MOCK_COLLECTION_DIR}/cluster-scoped/configmap.yaml" "password: ********"
}

@test "BR-PLATFORM-001.9: Support engineer cannot extract API keys from service logs" {
    # Business Outcome: Prevent credential leakage from logs
    # Edge Case: API keys in various log contexts (headers, body, env)
    mkdir -p "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway"
    cat > "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway/current.log" <<EOF
2026-01-04T12:00:00Z INFO Authenticating with HolmesGPT
2026-01-04T12:00:01Z DEBUG Authorization: Bearer sk-proj-abc123def456ghi789xyz
2026-01-04T12:00:02Z DEBUG Request: {"apiKey":"user-key-789xyz", "endpoint":"https://api.holmes.ai"}
2026-01-04T12:00:03Z INFO Authentication successful
EOF

    run bash "${SANITIZERS_DIR}/sanitize-all.sh" "${MOCK_COLLECTION_DIR}"

    # Verify API keys are redacted
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway/current.log" "sk-proj-abc123def456ghi789xyz"
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway/current.log" "user-key-789xyz"

    # But diagnostic context preserved
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway/current.log" "Authentication successful"
}

@test "BR-PLATFORM-001.9: Support engineer cannot extract PII (emails, names) from audit events" {
    # Business Outcome: GDPR Article 17 - Right to erasure
    # Edge Case: PII in structured data (JSON, YAML) and logs
    mkdir -p "${MOCK_COLLECTION_DIR}/datastorage"
    cat > "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" <<EOF
{
  "data": [
    {"event_type": "user.login", "email": "john.doe@acme.com", "timestamp": "2026-01-04T12:00:00Z"},
    {"event_type": "alert.created", "created_by": "jane.smith@acme.com", "message": "High CPU on prod"}
  ]
}
EOF

    run bash "${SANITIZERS_DIR}/sanitize-all.sh" "${MOCK_COLLECTION_DIR}"

    # Verify emails are redacted (GDPR compliance)
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" "john.doe@acme.com"
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" "jane.smith@acme.com"

    # Redacted format allows domain analysis without PII
    assert_file_contains "${MOCK_COLLECTION_DIR}/datastorage/audit-events.json" "@[REDACTED]"
}

# ========================================
# Edge Case: Kubernetes Secrets (base64)
# ========================================

@test "BR-PLATFORM-001.9: Support engineer cannot decode Kubernetes Secret values" {
    # Edge Case: Secrets in base64 encoding
    # Business Outcome: Base64 encoded secrets are also redacted
    mkdir -p "${MOCK_COLLECTION_DIR}/cluster-scoped"
    cat > "${MOCK_COLLECTION_DIR}/cluster-scoped/secrets.yaml" <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: datastorage-db-creds
type: Opaque
data:
  username: YWRtaW4=
  password: c3VwZXJzZWNyZXRwYXNzd29yZA==
  api_token: dG9rZW4tYWJjMTIzZGVmNDU2
EOF

    run bash "${SANITIZERS_DIR}/sanitize-all.sh" "${MOCK_COLLECTION_DIR}"

    # Verify base64 values are redacted
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/cluster-scoped/secrets.yaml" "YWRtaW4="
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/cluster-scoped/secrets.yaml" "c3VwZXJzZWNyZXRwYXNzd29yZA=="
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/cluster-scoped/secrets.yaml" "dG9rZW4tYWJjMTIzZGVmNDU2"

    # But Secret metadata preserved for troubleshooting
    assert_file_contains "${MOCK_COLLECTION_DIR}/cluster-scoped/secrets.yaml" "kind: Secret"
    assert_file_contains "${MOCK_COLLECTION_DIR}/cluster-scoped/secrets.yaml" "datastorage-db-creds"
}

# ========================================
# Edge Case: TLS Certificates & Private Keys
# ========================================

@test "BR-PLATFORM-001.9: Support engineer cannot extract TLS private keys" {
    # Edge Case: Multi-line private keys in YAML
    # Business Outcome: Prevent certificate compromise
    mkdir -p "${MOCK_COLLECTION_DIR}/cluster-scoped"
    cat > "${MOCK_COLLECTION_DIR}/cluster-scoped/tls-secret.yaml" <<EOF
apiVersion: v1
kind: Secret
metadata:
  name: gateway-tls
type: kubernetes.io/tls
data:
  tls.crt: LS0tLS1CRUdJTi...
  tls.key: LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0tCk1JSUV2UUlCQURBTkJna3Foa2lHOXcwQkFRRUZBQVNDQktjd2dnU2pBZ0VBQW9JQkFRQ...
EOF

    run bash "${SANITIZERS_DIR}/sanitize-all.sh" "${MOCK_COLLECTION_DIR}"

    # Verify private key is redacted
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/cluster-scoped/tls-secret.yaml" "LS0tLS1CRUdJTiBQUklWQVRFIEtFWS0tLS0t"

    # Certificate metadata preserved (support can see TLS is configured)
    assert_file_contains "${MOCK_COLLECTION_DIR}/cluster-scoped/tls-secret.yaml" "kubernetes.io/tls"
}

# ========================================
# Edge Case: Nested Sensitive Data
# ========================================

@test "BR-PLATFORM-001.9: Support engineer cannot extract credentials from nested JSON/YAML structures" {
    # Edge Case: Credentials deeply nested in complex structures
    # Business Outcome: Comprehensive sanitization across nesting levels
    mkdir -p "${MOCK_COLLECTION_DIR}/crds/workflowexecutions"
    cat > "${MOCK_COLLECTION_DIR}/crds/workflowexecutions/instance.yaml" <<EOF
apiVersion: kubernaut.ai/v1alpha1
kind: WorkflowExecution
spec:
  pipelineSpec:
    params:
      - name: DATABASE_URL
        value: "postgresql://user:SecretPass123@db:5432/prod"
      - name: REDIS_PASSWORD
        value: "redis-secret-456"
    tasks:
      - name: backup
        env:
          - name: AWS_SECRET_KEY
            value: "aws-key-789xyz"
EOF

    run bash "${SANITIZERS_DIR}/sanitize-all.sh" "${MOCK_COLLECTION_DIR}"

    # Verify nested secrets are redacted
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/crds/workflowexecutions/instance.yaml" "SecretPass123"
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/crds/workflowexecutions/instance.yaml" "redis-secret-456"
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/crds/workflowexecutions/instance.yaml" "aws-key-789xyz"
}

# ========================================
# Business Outcome: Audit Trail for Compliance
# ========================================

@test "BR-PLATFORM-001.9: Support engineer can prove sanitization occurred for compliance audit" {
    # Business Outcome: Generate audit trail for SOC2/ISO compliance
    mkdir -p "${MOCK_COLLECTION_DIR}/test"
    cat > "${MOCK_COLLECTION_DIR}/test/config.yaml" <<EOF
password: secret123
apiKey: key-456
EOF

    run bash "${SANITIZERS_DIR}/sanitize-all.sh" "${MOCK_COLLECTION_DIR}"

    # Sanitization report proves compliance
    assert_file_exists "${MOCK_COLLECTION_DIR}/sanitization-report.txt"
    assert_file_contains "${MOCK_COLLECTION_DIR}/sanitization-report.txt" "Sanitization Report"

    # Original data preserved for forensics (if needed by security team)
    assert_file_exists "${MOCK_COLLECTION_DIR}/test/config.yaml.pre-sanitize"
}

# ========================================
# Edge Case: Preserve Troubleshooting Context
# ========================================

@test "BR-PLATFORM-001.9: Support engineer retains troubleshooting context after sanitization" {
    # Business Outcome: Sanitization doesn't destroy diagnostic value
    # Edge Case: Service names, ports, error messages must remain
    mkdir -p "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway"
    cat > "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway/current.log" <<EOF
2026-01-04T12:00:00Z ERROR Failed to connect to datastorage:8080
2026-01-04T12:00:01Z ERROR Authentication failed with token=abc123xyz
2026-01-04T12:00:02Z ERROR Retry attempt 3/3 failed
2026-01-04T12:00:03Z FATAL Service shutting down due to persistent errors
EOF

    run bash "${SANITIZERS_DIR}/sanitize-all.sh" "${MOCK_COLLECTION_DIR}"

    # Sensitive data redacted
    assert_file_not_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway/current.log" "abc123xyz"

    # But troubleshooting context preserved
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway/current.log" "Failed to connect to datastorage:8080"
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway/current.log" "Retry attempt 3/3 failed"
    assert_file_contains "${MOCK_COLLECTION_DIR}/logs/kubernaut-system/gateway/current.log" "Service shutting down"
}

# ========================================
# Edge Case: No False Positives
# ========================================

@test "BR-PLATFORM-001.9: Sanitization does not corrupt legitimate data that looks like secrets" {
    # Edge Case: Don't redact data that matches patterns but isn't sensitive
    # Business Outcome: Avoid over-sanitization that destroys diagnostics
    mkdir -p "${MOCK_COLLECTION_DIR}/test"
    cat > "${MOCK_COLLECTION_DIR}/test/metrics.yaml" <<EOF
metrics:
  password_validation_failures: 42
  api_key_rotations: 5
  token_expiration_time: 3600
  email_notifications_sent: 150
EOF

    run bash "${SANITIZERS_DIR}/sanitize-all.sh" "${MOCK_COLLECTION_DIR}"

    # Verify metric names containing "password", "api_key", "token", "email" are NOT redacted
    assert_file_contains "${MOCK_COLLECTION_DIR}/test/metrics.yaml" "password_validation_failures: 42"
    assert_file_contains "${MOCK_COLLECTION_DIR}/test/metrics.yaml" "api_key_rotations: 5"
    assert_file_contains "${MOCK_COLLECTION_DIR}/test/metrics.yaml" "token_expiration_time: 3600"
    assert_file_contains "${MOCK_COLLECTION_DIR}/test/metrics.yaml" "email_notifications_sent: 150"
}

