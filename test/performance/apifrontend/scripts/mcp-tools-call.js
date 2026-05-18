import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { SLO, BASE_URL, AUTH_TOKEN } from '../config.js';

const errorRate = new Rate('errors');
const toolListDuration = new Trend('tool_list_duration_ms');
const toolCallDuration = new Trend('tool_call_duration_ms');

export const options = {
  stages: [
    { duration: '30s', target: 5 },
    { duration: '1m', target: 20 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_duration: [`p(95)<${SLO.HTTP_P95_MS}`, `p(99)<${SLO.HTTP_P99_MS}`],
    errors: [`rate<${SLO.ERROR_RATE}`],
    tool_call_duration_ms: [`p(99)<${SLO.PROXY_TOOL_P99_MS}`],
  },
};

const baseHeaders = {
  'Content-Type': 'application/json',
  'Authorization': `Bearer ${AUTH_TOKEN}`,
  'Accept': 'application/json, text/event-stream',
};

function initSession() {
  const initBody = JSON.stringify({
    jsonrpc: '2.0',
    id: 'init-1',
    method: 'initialize',
    params: {
      protocolVersion: '2025-03-26',
      capabilities: {},
      clientInfo: { name: 'k6-perf', version: '1.0' },
    },
  });

  const initRes = http.post(`${BASE_URL}/mcp`, initBody, { headers: baseHeaders });
  // k6 normalizes headers to lowercase
  const sessionId = initRes.headers['mcp-session-id'] || initRes.headers['Mcp-Session-Id'];

  if (!sessionId) {
    console.error('Failed to obtain MCP session ID');
    return null;
  }

  const notifBody = JSON.stringify({
    jsonrpc: '2.0',
    method: 'notifications/initialized',
  });
  http.post(`${BASE_URL}/mcp`, notifBody, {
    headers: { ...baseHeaders, 'Mcp-Session-Id': sessionId },
  });

  return sessionId;
}

export function setup() {
  const sessionId = initSession();
  return { sessionId };
}

export default function (data) {
  // Each VU creates its own session on first iteration to avoid session contention
  let sessionId = data.sessionId;
  if (__ITER === 0 && __VU > 1) {
    sessionId = initSession() || data.sessionId;
  }

  const sessionHeaders = {
    ...baseHeaders,
    'Mcp-Session-Id': sessionId,
  };

  // MCP tools/list
  const listBody = JSON.stringify({
    jsonrpc: '2.0',
    id: `perf-list-${__VU}-${__ITER}`,
    method: 'tools/list',
    params: {},
  });

  const listRes = http.post(`${BASE_URL}/mcp`, listBody, { headers: sessionHeaders });
  const listOk = check(listRes, {
    'tools/list returns 200': (r) => r.status === 200,
    'tools/list has result (no JSON-RPC error)': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.result !== undefined && body.error === undefined;
      } catch (e) {
        return false;
      }
    },
  });
  toolListDuration.add(listRes.timings.duration);
  errorRate.add(!listOk);

  // MCP tools/call (kubernaut_list_remediations)
  const callBody = JSON.stringify({
    jsonrpc: '2.0',
    id: `perf-call-${__VU}-${__ITER}`,
    method: 'tools/call',
    params: {
      name: 'kubernaut_list_remediations',
      arguments: { namespace: 'default' },
    },
  });

  const callRes = http.post(`${BASE_URL}/mcp`, callBody, { headers: sessionHeaders });
  const callOk = check(callRes, {
    'tools/call returns 200': (r) => r.status === 200,
    'tools/call result is not error': (r) => {
      try {
        const body = JSON.parse(r.body);
        if (body.error) return false;
        if (body.result && body.result.isError) return false;
        return true;
      } catch (e) {
        return false;
      }
    },
  });
  toolCallDuration.add(callRes.timings.duration);
  errorRate.add(!callOk);

  sleep(1);
}
