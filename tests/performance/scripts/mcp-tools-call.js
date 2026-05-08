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

export default function () {
  // MCP tools/list
  const listBody = JSON.stringify({
    jsonrpc: '2.0',
    id: `perf-${__VU}-${__ITER}`,
    method: 'tools/list',
    params: {},
  });

  const listRes = http.post(`${BASE_URL}/mcp`, listBody, { headers });
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

  // MCP tools/call (af_list_active_remediations)
  const callBody = JSON.stringify({
    jsonrpc: '2.0',
    id: `perf-call-${__VU}-${__ITER}`,
    method: 'tools/call',
    params: {
      name: 'af_list_active_remediations',
      arguments: {},
    },
  });

  const callRes = http.post(`${BASE_URL}/mcp`, callBody, { headers });
  check(callRes, {
    'tools/call returns 200': (r) => r.status === 200,
  });
  toolDuration.add(callRes.timings.duration);
  errorRate.add(callRes.status >= 500);

  sleep(1);
}
