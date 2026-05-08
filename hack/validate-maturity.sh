#!/usr/bin/env bash
# validate-maturity.sh — Static checks to verify kubernaut service maturity criteria.
# Exit code 0: all checks pass. Non-zero: at least one check failed.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

FAILURES=0

fail() {
  echo "FAIL: $1"
  FAILURES=$((FAILURES + 1))
}

pass() {
  echo "PASS: $1"
}

# 1. Prometheus metrics registered (af_ namespace)
if grep -rq 'Namespace:.*"af"' "${ROOT_DIR}/internal/metrics/"; then
  pass "Prometheus metrics use af_ namespace"
else
  fail "No Prometheus metrics with af_ namespace found"
fi

# 2. Health endpoints (/healthz, /readyz)
if grep -rq '"/healthz"' "${ROOT_DIR}/internal/handler/"; then
  pass "Health endpoint /healthz registered"
else
  fail "Missing /healthz endpoint"
fi

if grep -rq '"/readyz"' "${ROOT_DIR}/internal/handler/"; then
  pass "Readiness endpoint /readyz registered"
else
  fail "Missing /readyz endpoint"
fi

# 3. Graceful shutdown (signal handling)
if grep -rq 'signal.NotifyContext\|signal.Notify' "${ROOT_DIR}/cmd/"; then
  pass "Graceful shutdown signal handling present"
else
  fail "Missing signal handling for graceful shutdown"
fi

# 4. RFC 7807 error responses
if grep -rq 'WriteProblem\|application/problem' "${ROOT_DIR}/internal/"; then
  pass "RFC 7807 problem responses implemented"
else
  fail "Missing RFC 7807 error response handling"
fi

# 5. Audit trail emission
if grep -rq 'audit.Emitter\|audit.Event' "${ROOT_DIR}/internal/"; then
  pass "Audit trail event emission present"
else
  fail "Missing audit trail implementation"
fi

# 6. Structured logging (slog or zap)
if grep -rq 'slog\.\|zap\.' "${ROOT_DIR}/internal/logging/"; then
  pass "Structured logging (slog/zap) configured"
else
  fail "Missing structured logging implementation"
fi

# 7. Rate limiting
if [ -d "${ROOT_DIR}/internal/ratelimit" ]; then
  pass "Rate limiting package present"
else
  fail "Missing rate limiting implementation"
fi

# 8. Circuit breaker (resilience)
if grep -rq 'CircuitBreaker\|gobreaker' "${ROOT_DIR}/internal/resilience/"; then
  pass "Circuit breaker pattern implemented"
else
  fail "Missing circuit breaker implementation"
fi

echo ""
echo "---"
if [ $FAILURES -eq 0 ]; then
  echo "All maturity checks passed."
  exit 0
else
  echo "${FAILURES} maturity check(s) FAILED."
  exit 1
fi
