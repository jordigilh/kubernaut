import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { SLO, BASE_URL } from '../config.js';

const errorRate = new Rate('errors');
const healthDuration = new Trend('health_duration_ms');

export const options = {
  stages: [
    { duration: '30s', target: 10 },
    { duration: '1m', target: 50 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_duration: [`p(95)<${SLO.HTTP_P95_MS}`],
    errors: [`rate<${SLO.ERROR_RATE}`],
    health_duration_ms: [`p(99)<${SLO.AGENT_CARD_P99_MS}`],
  },
};

export default function () {
  const healthRes = http.get(`${BASE_URL}/healthz`);
  check(healthRes, {
    'healthz returns 200': (r) => r.status === 200,
  });
  healthDuration.add(healthRes.timings.duration);
  errorRate.add(healthRes.status >= 500);

  const readyRes = http.get(`${BASE_URL}/readyz`);
  check(readyRes, {
    'readyz returns 200': (r) => r.status === 200,
  });
  errorRate.add(readyRes.status >= 500);

  const cardRes = http.get(`${BASE_URL}/.well-known/agent-card.json`);
  check(cardRes, {
    'agent-card returns 200': (r) => r.status === 200,
    'agent-card is JSON': (r) => r.headers['Content-Type'] === 'application/json',
  });
  errorRate.add(cardRes.status >= 500);

  sleep(0.5);
}
