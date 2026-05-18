import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '30s', target: 10 },
    { duration: '5m', target: 10 },
    { duration: '30s', target: 0 },
  ],
  thresholds: {
    http_req_duration: ['p(95)<50', 'p(99)<100'],
    http_req_failed: ['rate<0.001'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'https://localhost:8443';

export default function () {
  const res = http.get(`${BASE_URL}/healthz`);
  check(res, {
    'status is 200': (r) => r.status === 200,
    'body is ok': (r) => r.body === 'ok',
    'latency < 50ms': (r) => r.timings.duration < 50,
  });
  sleep(0.1);
}
