// Load test: each virtual user submits a run and polls until it finishes, over
// and over, for the duration. The headline number is completed runs per second
// (k6's iteration rate), plus the end-to-end time from submit to a terminal
// status. Point it at the cluster (default) or a local `loupe serve`.
//
//   k6 run loadtest/load.js
//   VUS=40 DURATION=45s BASE_URL=http://localhost:8899 k6 run loadtest/load.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend } from 'k6/metrics';

const BASE = __ENV.BASE_URL || 'http://localhost:8899';
const completionMs = new Trend('run_completion_ms', true);
const JSON_HEADERS = { headers: { 'Content-Type': 'application/json' } };

export const options = {
  scenarios: {
    drain: {
      executor: 'constant-vus',
      vus: Number(__ENV.VUS || 20),
      duration: __ENV.DURATION || '30s',
    },
  },
  thresholds: {
    submit_ok: ['rate>0.99'],
  },
};

import { Rate } from 'k6/metrics';
const submitOk = new Rate('submit_ok');

export default function () {
  const start = Date.now();

  const res = http.post(`${BASE}/runs`, '{}', JSON_HEADERS);
  const ok = check(res, { 'submit 200': (r) => r.status === 200 });
  submitOk.add(ok);
  if (!ok) return;

  const id = res.json('id');
  const query = JSON.stringify({ query: `{ run(id:"${id}"){ status } }` });

  for (let i = 0; i < 200; i++) {
    const r = http.post(`${BASE}/graphql`, query, JSON_HEADERS);
    const status = r.json('data.run.status');
    if (status === 'SUCCEEDED' || status === 'FAILED') {
      completionMs.add(Date.now() - start);
      return;
    }
    sleep(0.05);
  }
}
