import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Counter } from 'k6/metrics';
import { SLO, BASE_URL, AUTH_TOKEN } from '../config.js';

const errorRate = new Rate('errors');
const sseConnections = new Counter('sse_connections_opened');

export const options = {
  stages: [
    { duration: '30s', target: 10 },
    { duration: '1m', target: 30 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    errors: [`rate<${SLO.ERROR_RATE}`],
  },
};

const headers = {
  'Content-Type': 'application/json',
  'Authorization': `Bearer ${AUTH_TOKEN}`,
};

export default function () {
  // Simulate SSE-like A2A message/send call
  const body = JSON.stringify({
    jsonrpc: '2.0',
    id: `sse-${__VU}-${__ITER}`,
    method: 'message/send',
    params: {
      message: {
        messageId: `msg-${__VU}-${__ITER}`,
        role: 'user',
        parts: [{ kind: 'text', text: 'list failing pods' }],
      },
    },
  });

  const res = http.post(`${BASE_URL}/a2a/invoke`, body, {
    headers,
    timeout: '30s',
  });

  check(res, {
    'a2a invoke returns 200': (r) => r.status === 200,
    'response is JSON-RPC': (r) => {
      try {
        const parsed = JSON.parse(r.body);
        return parsed.jsonrpc === '2.0';
      } catch (e) {
        return false;
      }
    },
  });
  sseConnections.add(1);
  errorRate.add(res.status >= 500);

  sleep(2);
}
