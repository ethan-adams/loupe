# Decisions

Short records of the real tradeoffs, and why the call went the way it did.

## 1. Go for the whole thing

The hard parts here are systems work: a durable queue, resumable runs, leases,
back-pressure, and streaming. Go is a good fit and keeps the stack small. The
"other languages have richer AI libraries" argument does not apply, because talking
to a local model is a plain HTTP call, not an SDK. One language, one binary, easy to
read.

## 2. Postgres is the queue, not just the store

Rather than add Redis or a broker, the queue lives in the same Postgres that stores
runs, using `SELECT ... FOR UPDATE SKIP LOCKED` to hand each run to exactly one
worker. One fewer moving part, transactional with the step log, and good enough to
hundreds of runs a second. The cost is that the store is the eventual scaling
ceiling (see `loadtest/RESULTS.md`); when that matters, the fix is to scale the
store, and the queue can move to a broker then. Not before.

## 3. Leases for liveness, not a heartbeat thread

A worker claims a run with a lease and renews it on every committed step. There is
no separate heartbeat goroutine: making progress *is* the heartbeat. If a worker
dies, it stops committing steps, the lease expires, and another worker reclaims the
run. Simple, and it ties liveness to real progress instead of a side channel.

## 4. Resume by replaying observations, not by snapshotting

On resume, the loop rebuilds the model's observation history from the stored steps
and continues; it does not re-run the tools that already ran. That keeps committed
side effects from happening twice with no distributed-transaction machinery. The
requirement it places on tools is that their effects live in durable external state
(a real repo on disk, a database), not in process memory. The demo's in-memory toy
repo is the one place that is not yet true, which is why cross-process world state is
called out as a known limit.

## 5. GraphQL for reads, SSE for the live stream

The typed reads (a run, a list of runs) go through a gqlgen Apollo Federation
subgraph, so `Run` is an entity a supergraph router can reference by id. The live
step stream does not go through GraphQL. Federation routers do not federate
subscriptions anyway, and pushing a high-rate stream through GraphQL buys nothing
here, so the live trace is Server-Sent Events, which a browser reads in one line.
The read schema stays clean; the hot path stays cheap.

## 6. Local model by default, real model optional

The runtime is model-agnostic behind a three-method interface. The default demo
uses a deterministic scripted model so it runs anywhere with no setup and the tests
are reproducible. `--model ollama` drives a real local model (qwen3:8b) for a live
run at zero metered cost. An API-key model would be a third implementation of the
same interface.
