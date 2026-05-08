import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { SLO, BASE_URL, AUTH_TOKEN } from '../config.js';

const errorRate = new Rate('errors');
const latencyTrend = new Trend('request_duration_ms');

export const options = {
  scenarios: {
    health_checks: {
      executor: 'constant-arrival-rate',
      rate: 10,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 5,
      maxVUs: 20,
      exec: 'healthCheck',
    },
    tool_calls: {
      executor: 'ramping-vus',
      startVUs: 1,
      stages: [
        { duration: '30s', target: 10 },
        { duration: '1m', target: 10 },
        { duration: '30s', target: 0 },
      ],
      exec: 'toolCall',
    },
    agent_card: {
      executor: 'constant-arrival-rate',
      rate: 5,
      timeUnit: '1s',
      duration: '2m',
      preAllocatedVUs: 3,
      maxVUs: 10,
      exec: 'agentCard',
    },
  },
  thresholds: {
    http_req_duration: [`p(95)<${SLO.HTTP_P95_MS}`, `p(99)<${SLO.HTTP_P99_MS}`],
    errors: [`rate<${SLO.ERROR_RATE}`],
  },
};

const headers = {
  'Content-Type': 'application/json',
  'Authorization': `Bearer ${AUTH_TOKEN}`,
};

export function healthCheck() {
  group('health', () => {
    const res = http.get(`${BASE_URL}/healthz`);
    check(res, { 'healthz 200': (r) => r.status === 200 });
    latencyTrend.add(res.timings.duration);
    errorRate.add(res.status >= 500);
  });
}

export function toolCall() {
  group('mcp-tools', () => {
    const body = JSON.stringify({
      jsonrpc: '2.0',
      id: `mixed-${__VU}-${__ITER}`,
      method: 'tools/list',
      params: {},
    });

    const res = http.post(`${BASE_URL}/mcp`, body, { headers });
    check(res, { 'tools/list 200': (r) => r.status === 200 });
    latencyTrend.add(res.timings.duration);
    errorRate.add(res.status >= 500);
    sleep(1);
  });
}

export function agentCard() {
  group('agent-card', () => {
    const res = http.get(`${BASE_URL}/.well-known/agent-card.json`);
    check(res, {
      'agent-card 200': (r) => r.status === 200,
      'agent-card fast': (r) => r.timings.duration < SLO.AGENT_CARD_P99_MS,
    });
    latencyTrend.add(res.timings.duration);
    errorRate.add(res.status >= 500);
  });
}
