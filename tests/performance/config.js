// SLO threshold assertions for k6 performance tests.
// Values aligned with docs/slo/SLO_DEFINITIONS.md

export const SLO = {
  HTTP_P95_MS: 500,   // SLO-1: HTTP request latency P95 < 500ms
  HTTP_P99_MS: 1000,  // SLO-2: HTTP request latency P99 < 1s
  CRD_TOOL_P99_MS: 500, // SLO-3: Native CRD tool P99 < 500ms
  PROXY_TOOL_P99_MS: 2000, // SLO-4: Proxied tool P99 < 2s
  AUTH_P99_MS: 200,   // SLO-5: Auth latency P99 < 200ms
  ERROR_RATE: 0.001,  // SLO-6: < 0.1% 5xx rate
  AGENT_CARD_P99_MS: 50, // SLO-7: Agent Card P99 < 50ms
};

export const BASE_URL = __ENV.AF_BASE_URL || 'http://localhost:8443';
export const AUTH_TOKEN = __ENV.AF_AUTH_TOKEN || 'test-jwt-token';
