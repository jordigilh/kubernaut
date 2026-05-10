import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { SLO, BASE_URL, AUTH_TOKEN } from '../config.js';

const errorRate = new Rate('errors');
const toolDuration = new Trend('tool_call_duration_ms');

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

const headers = {
  'Content-Type': 'application/json',
  'Authorization': `Bearer ${AUTH_TOKEN}`,
};

export function setup() {
  // Initialize MCP session (required by Streamable HTTP protocol)
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

  const initRes = http.post(`${BASE_URL}/mcp`, initBody, {
    headers: { ...headers, Accept: 'application/json, text/event-stream' },
  });
  const sessionId = initRes.headers['Mcp-Session-Id'];

  // Send initialized notification
  const notifBody = JSON.stringify({
    jsonrpc: '2.0',
    method: 'notifications/initialized',
  });
  http.post(`${BASE_URL}/mcp`, notifBody, {
    headers: { ...headers, 'Mcp-Session-Id': sessionId, Accept: 'application/json, text/event-stream' },
  });

  return { sessionId };
}

export default function (data) {
  const sessionHeaders = {
    ...headers,
    'Mcp-Session-Id': data.sessionId,
    Accept: 'application/json, text/event-stream',
  };

  // MCP tools/list
  const listBody = JSON.stringify({
    jsonrpc: '2.0',
    id: `perf-${__VU}-${__ITER}`,
    method: 'tools/list',
    params: {},
  });

  const listRes = http.post(`${BASE_URL}/mcp`, listBody, { headers: sessionHeaders });
  check(listRes, {
    'tools/list returns 200': (r) => r.status === 200,
    'tools/list has result': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.result !== undefined;
      } catch (e) {
        return false;
      }
    },
  });
  toolDuration.add(listRes.timings.duration);
  errorRate.add(listRes.status >= 500);

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
  check(callRes, {
    'tools/call returns 200': (r) => r.status === 200,
  });
  toolDuration.add(callRes.timings.duration);
  errorRate.add(callRes.status >= 500);

  sleep(1);
}
