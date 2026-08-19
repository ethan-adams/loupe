# Loupe

**Run an AI agent and watch every step it takes, live.**

![The Loupe control room: a run's steps streaming in as the agent works](docs/control-room.png)

> One queue, stateless workers: 4 replicas take **6x the throughput of 1** (27 to
> 162 completed runs/sec) and hold p95 latency at **1.2s** where a single replica
> hits 7.8s. Every run is durable and resumable, and streams live to the browser.

An agent normally works behind a curtain. You give it a task, it thinks, calls
tools, makes mistakes, retries, and hands you a final answer, and you never get
to see what happened in between. Loupe removes the curtain. Every step the agent
takes, every tool it calls, every error it recovers from, is streamed as it
happens and kept so you can replay the whole run afterward.

> Start an agent, watch it plan, call tools, hit an error, and recover, live,
> with every step traced.

## Quickstart

Needs Go. No model server, no API key, nothing else to install:

```
make demo
```

You will watch an agent take on a small repo with a failing test: it runs the
tests, opens the wrong file, recovers, finds the bug, writes the fix, and
confirms the tests pass. Every step prints as it happens.

Drive it with a real local model instead of the built-in script (needs
[Ollama](https://ollama.com) running with a model pulled, for example
`ollama pull qwen3:8b`):

```
make demo-ollama
```

## Runs survive a crash

A run is durable: every step is committed as it happens, so a run that dies part
way through is picked up by another worker and finished from where it stopped,
without redoing the work that already succeeded. Watch it, no setup required:

```
make demo-resume
```

One worker takes the task, fixes the bug, and loses power the instant the fix is
committed. Its claim expires, a second worker reclaims the same run, runs the
tests (which pass, because the fix is real), and finishes.

The durable store behind that is Postgres. To run it for real:

```
make db-up                 # start Postgres in Docker
make submit                # enqueue a run, prints its id
make worker                # claim and run work, streamed live (Ctrl-C to stop)
```

Start more than one `make worker` and they share the queue. Kill one mid-run and
another reclaims its run once the lease expires.

## Best of N: run every candidate against the tests

A single model call is often wrong or shallow. Ask it to solve a real coding
problem several times and the solutions diverge, most with bugs. The gate here is
not a vote or an opinion: every candidate is executed against a hidden test suite.
Needs Ollama:

```
make code-consensus
```

The built-in problem parses a URL query string. Eight solutions come back and, on
the recorded run, none passes every test: they all share the same blind spots
(dropping keys with no value, not stripping a leading `?`). Sampling or a majority
vote would agree on the same wrong output; only running the tests surfaces the gap,
so you ship the best and know exactly what it misses. This is the front door to
evaluating agents.

A self-contained visual replay of a real recorded run is at
[`web/demo.html`](web/demo.html) (open it in a browser).

![Best of N: eight code solutions stream in and are graded against a hidden test suite](docs/bestofn.gif)

A lighter variant for short-answer questions, where the gate is a majority vote
plus an independent judge instead of tests, is `make consensus`.

## Evals: score a model across a suite, catch regressions

The same test gate, run as measurement. `make eval` runs a suite of coding
problems against the model, several solutions each, and prints a scorecard of how
well the best solution does on each problem's hidden tests. It stores every run,
so the next run flags any task whose score dropped.

```
  TASK                       BEST     PASS-RATE PASSED-ALL
  Compare version strings    9/10     90%       0/5
  Parse a query string       7/10     70%       0/5
  Parse a duration string    8/10     80%       0/5

  overall pass-rate: 80%   model: qwen3:8b
  regressions vs last run: none
```

This is the honest core of evals: real scores from real execution, tracked over
time, so a prompt or model change that quietly makes things worse shows up as a
number going down.

## Query it, stream it

`make serve` runs an HTTP server with embedded workers, then open
`http://localhost:8080` for the **control room**: a list of runs, a New run
button, and a live timeline of an agent working. Each step is a card (thinking,
tool call, result), errors flagged in red, the final answer highlighted. A
running agent streams in live; click any card to see the full input and output.

```
make db-up
make serve                 # control room + GraphQL + live stream on :8080
```

Under the control room:

- **GraphQL** at `/graphql` (a playground at `/graphql/playground`): typed reads
  over runs and their steps. It is an Apollo Federation subgraph, so a supergraph
  router can reference a run by id and other services can extend it.
- **Live trace** at `GET /runs/<id>/stream`: the run's steps as Server-Sent
  Events, which a browser reads with a one-line `EventSource`.

The split is deliberate. Typed, low-rate reads go through GraphQL; the high-rate
step stream does not. It is the same reason you would not push a 60fps feed
through a GraphQL subscription.

## What you are looking at

The demo is one run of the agent loop:

- **think** — the model decides what to do next
- **call** — it invokes a tool (list files, read a file, write a file, run tests)
- **observe** — the tool returns a result, or an error the agent has to handle
- **answer** — the run finishes

The bug is a real one (`add()` subtracts instead of adds), the recovery from a
wrong file path is real, and the run only succeeds once the tests actually pass.

## On Kubernetes

The whole thing runs on a local `kind` cluster with one command, and there is a
load test with real numbers:

```
make up                    # 3-node kind cluster + Postgres + Loupe, on localhost:8899
make hpa                   # metrics-server + a CPU autoscaler (optional)
VUS=150 DURATION=30s make loadtest
make down
```

Scaling the worker replicas takes on more load: 1 to 4 replicas is a 6x throughput
gain and cuts p95 latency from 7.8s to 1.2s. Past that the single Postgres run
store is the ceiling. The numbers and the honest bottleneck read are in
[`loadtest/RESULTS.md`](loadtest/RESULTS.md).

## More

- [`ARCHITECTURE.md`](ARCHITECTURE.md): the pieces, the run lifecycle, how resume works.
- [`DECISIONS.md`](DECISIONS.md): the real tradeoffs and why each call went the way it did.

The quick browser demo uses an in-memory repo for zero setup. For the real thing,
`make fix-demo` runs the same loop against real files on disk, with `run_tests`
actually running `python3`: the agent lists files, reads the source, writes a fix,
and the test really passes. Because those edits live on disk, they are durable, so
a resumed run picks up correct world state (unlike the in-memory toy repo).

```
make fix-demo               # scripted, no model server
make fix-demo MODEL=ollama  # a real local model fixes the real file
```

## Layout

```
cmd/loupe        the command line entry point
internal/agent   the plan / act / observe loop (the run engine), resumable
internal/model   the model boundary: a scripted model and an Ollama model
internal/tools   the agent's tools and the demo repo they act on
internal/trace   the event stream every step is published to
internal/render  turns the event stream into a live terminal view
internal/store   the durable run store: an interface, an in-memory and a Postgres impl
internal/worker  claims runs from the store, resumes crashed ones
internal/gql     the GraphQL federation subgraph (gqlgen) over runs and steps
internal/server  the HTTP surface: the control room UI, GraphQL, a playground, the SSE stream
internal/demo    wires the pieces into the runnable scenarios
```

## License

MIT. See [LICENSE](LICENSE).
