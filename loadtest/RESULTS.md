# Load test results

The claim under test: **stateless workers draining one queue scale horizontally.**
Add worker replicas and the system takes on more load without falling over.

## Setup

- 3-node `kind` cluster (1 control-plane, 2 workers) on a laptop (Apple M4, 16 GB).
- One Postgres pod (the run store), ephemeral storage.
- The Loupe deployment: each pod serves the HTTP API and runs 4 embedded workers.
  Scaling replicas scales the workers draining the shared queue.
- Model: the deterministic scripted agent at `--pace 0`, so each run is real work
  (17 steps, each durably committed) with no artificial delay.
- Driver: `loadtest/load.js`. Each virtual user submits a run and polls it to a
  terminal status, over and over, for 30s. Throughput is completed runs per second.

Reproduce:

```
make up
make image                                   # after code changes
kubectl scale deploy/loupe --replicas=N
VUS=150 DURATION=30s make loadtest
```

## Under saturating load (150 concurrent clients)

| replicas (workers) | throughput | p95 run latency | median |
|---|---|---|---|
| 1 (4)  | 27 runs/s  | 7.78 s | 0.92 s |
| 4 (16) | 162 runs/s | 1.22 s | 0.91 s |
| 8 (32) | 150 runs/s | 1.25 s | 0.99 s |

Going from 1 to 4 replicas is a **6x throughput gain (27 to 162 runs/s)** and drops
p95 latency from **7.8s to 1.2s**: with one replica the queue backs up and runs
wait; adding workers drains it. That is the horizontal-scaling claim, demonstrated.

## Where it stops scaling, and why

From 4 to 8 replicas throughput does not improve (162 to 150 runs/s, same p95). The
workers are no longer the bottleneck. At this point two things bind:

- **Offered load.** At 150 concurrent clients and ~0.9s per run, Little's law caps
  throughput near 150 / 0.9 ≈ 165 runs/s regardless of worker count. A lighter run
  (30 clients) completes in ~60ms and the same cluster does ~370 runs/s.
- **The single run store.** Each run commits 17 steps, each a transaction (insert +
  lease renew). At ~160 runs/s that is ~5,000+ write statements/s on one Postgres
  pod, on top of the polling reads. That instance is the next ceiling.

The honest read: the **workers scale, the store does not** (yet). The next lever is
not more workers, it is the store: batch the step commits, partition runs across
shards, or move the hot append path to an append-optimized log. Noted, not hidden.

## Autoscaling

`make hpa` installs metrics-server and a HorizontalPodAutoscaler (CPU target 60%,
1 to 8 replicas), which performs the same scale-out automatically as worker CPU
rises under load. The numbers above use manual `kubectl scale` so each data point
is a clean, fixed replica count.
