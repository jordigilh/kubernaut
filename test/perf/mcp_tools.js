import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '1m', target: 50 },
    { duration: '5m', target: 50 },
    { duration: '1m', target: 0 },
  ],
  thresholds: {
    'http_req_duration{name:initialize}': ['p(95)<500'],
    'http_req_duration{name:tools_list}': ['p(95)<500'],
    'http_req_duration{name:tools_call}': ['p(95)<2000'],
    http_req_failed: ['rate<0.01'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'https://localhost:8443';
const TOKEN = __ENV.AUTH_TOKEN || 'test-token';

const headers = {
  'Content-Type': 'application/json',
  Accept: 'application/json, text/event-stream',
  Authorization: `Bearer ${TOKEN}`,
};

function jsonrpc(method, id, params) {
  const body = { jsonrpc: '2.0', id, method };
  if (params) body.params = params;
  return JSON.stringify(body);
}

export default function () {
  // Initialize session
  const initRes = http.post(
    `${BASE_URL}/mcp`,
    jsonrpc('initialize', 1, {
      protocolVersion: '2025-03-26',
      capabilities: {},
      clientInfo: { name: 'k6-perf', version: '1.0' },
    }),
    { headers, tags: { name: 'initialize' } }
  );
  check(initRes, {
    'initialize 200': (r) => r.status === 200,
  });

  const sessionId = initRes.headers['Mcp-Session-Id'] || '';
  const sessionHeaders = { ...headers };
  if (sessionId) sessionHeaders['Mcp-Session-Id'] = sessionId;

  // Send initialized notification
  http.post(
    `${BASE_URL}/mcp`,
    JSON.stringify({ jsonrpc: '2.0', method: 'notifications/initialized' }),
    { headers: sessionHeaders, tags: { name: 'initialized_notif' } }
  );

  // tools/list
  const listRes = http.post(
    `${BASE_URL}/mcp`,
    jsonrpc('tools/list', 2, null),
    { headers: sessionHeaders, tags: { name: 'tools_list' } }
  );
  check(listRes, {
    'tools/list 200': (r) => r.status === 200,
    'has tools': (r) => r.body.includes('kubernaut_'),
  });

  // tools/call (stub tool)
  const callRes = http.post(
    `${BASE_URL}/mcp`,
    jsonrpc('tools/call', 3, {
      name: 'kubernaut_list_remediations',
      arguments: {},
    }),
    { headers: sessionHeaders, tags: { name: 'tools_call' } }
  );
  check(callRes, {
    'tools/call 200': (r) => r.status === 200,
  });

  sleep(1);
}
