# Loupe

**Run an AI agent and watch every step it takes, live.**

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

## What you are looking at

The demo is one run of the agent loop:

- **think** — the model decides what to do next
- **call** — it invokes a tool (list files, read a file, write a file, run tests)
- **observe** — the tool returns a result, or an error the agent has to handle
- **answer** — the run finishes

The bug is a real one (`add()` subtracts instead of adds), the recovery from a
wrong file path is real, and the run only succeeds once the tests actually pass.

## Status

Early. This milestone is the walking skeleton: the loop, tool dispatch, a
pluggable model (a deterministic script or a local model), and a live trace to
your terminal. On the way:

- a durable, resumable run store so a run survives a crash and picks up where it left off
- a GraphQL API over runs, steps, and traces
- a web control room that renders a run live in the browser
- horizontal scale with a load test and numbers

## Layout

```
cmd/loupe        the command line entry point
internal/agent   the plan / act / observe loop (the run engine)
internal/model   the model boundary: a scripted model and an Ollama model
internal/tools   the agent's tools and the demo repo they act on
internal/trace   the event stream every step is published to
internal/render  turns the event stream into a live terminal view
internal/demo    wires the pieces into the fix-a-failing-test scenario
```

## License

MIT. See [LICENSE](LICENSE).
