import http from 'k6/http';
import { check, sleep } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export const options = {
  vus: 10,
  duration: '30s',
  thresholds: {
    http_req_failed: ['rate<0.05'],
    http_req_duration: ['p(95)<500'],
    checks: ['rate>0.95'],
  },
};

export default function () {
  const search = http.get(`${BASE_URL}/flights?origin=SVO&destination=LED&date=2026-04-01`);
  check(search, {
    'search status 200': (r) => r.status === 200,
  });

  const flight = http.get(`${BASE_URL}/flights/1`);
  check(flight, {
    'get flight status 200': (r) => r.status === 200,
  });

  sleep(0.5);
}

export function handleSummary(data) {
  return {
    'loadtest-summary.json': JSON.stringify(data, null, 2),
  };
}
