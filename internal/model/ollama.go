package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// Ollama drives the loop with a real local model over Ollama's HTTP API. It is
// intentionally thin: talking to a local model is just a JSON POST, so nothing
// here needs a heavyweight SDK. The default demo uses the scripted Mock; this
// is what you switch to with `loupe demo --model ollama`.
type Ollama struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllama reads its endpoint and model from the environment so the same
// binary works against any local model without a rebuild:
//
//	LOUPE_OLLAMA_URL   (default http://localhost:11434)
//	LOUPE_OLLAMA_MODEL (default llama3.1:8b)
func NewOllama(client *http.Client) *Ollama {
	base := env("LOUPE_OLLAMA_URL", "http://localhost:11434")
	m := env("LOUPE_OLLAMA_MODEL", "qwen3:8b")
	if client == nil {
		client = http.DefaultClient
	}
	return &Ollama{baseURL: strings.TrimRight(base, "/"), model: m, client: client}
}

func (o *Ollama) Name() string { return "ollama:" + o.model }

// decisionJSON is the shape we ask the model to emit. Keeping it flat makes it
// far more likely a small local model produces valid output.
type decisionJSON struct {
	Thought string `json:"thought"`
	Tool    string `json:"tool"`
	Input   string `json:"input"`
	Final   string `json:"final"`
}

func (o *Ollama) Next(ctx context.Context, t Turn) (Decision, error) {
	raw, err := o.generate(ctx, buildPrompt(t), nil)
	if err != nil {
		return Decision{}, err
	}

	var dj decisionJSON
	if err := json.Unmarshal([]byte(raw), &dj); err != nil {
		// The model ignored the JSON instruction; treat its prose as a final
		// answer rather than crashing the run.
		return Decision{Thought: "(model returned unstructured text)", Final: strings.TrimSpace(raw)}, nil
	}

	d := Decision{Thought: strings.TrimSpace(dj.Thought)}
	if tool := strings.TrimSpace(dj.Tool); tool != "" {
		d.ToolCall = &ToolCall{Name: tool, Input: dj.Input}
	} else {
		d.Final = strings.TrimSpace(dj.Final)
	}
	return d, nil
}

// generate is one call to Ollama's /api/generate with JSON-constrained output.
// opts, if non-nil, is passed through as Ollama options (e.g. temperature, seed),
// which is how the consensus demo makes parallel attempts diverge.
func (o *Ollama) generate(ctx context.Context, prompt string, opts map[string]any) (string, error) {
	body := map[string]any{"model": o.model, "prompt": prompt, "stream": false, "format": "json"}
	if opts != nil {
		body["options"] = opts
	}
	buf, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/generate", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request failed (is `ollama serve` running?): %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned %s", resp.Status)
	}

	var envelope struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return "", fmt.Errorf("decode ollama envelope: %w", err)
	}
	return envelope.Response, nil
}

// Ask is a single-shot JSON completion. The consensus demo uses it to answer one
// question many times; opts sets Ollama options like temperature and seed.
func (o *Ollama) Ask(ctx context.Context, prompt string, opts map[string]any) (string, error) {
	return o.generate(ctx, prompt, opts)
}

func buildPrompt(t Turn) string {
	var b strings.Builder
	b.WriteString("You are an agent that fixes code by calling tools one at a time.\n")
	b.WriteString("Task: ")
	b.WriteString(t.Task)
	b.WriteString("\n\nTools you may call:\n")
	for _, ts := range t.Tools {
		fmt.Fprintf(&b, "- %s: %s\n", ts.Name, ts.Description)
	}
	if len(t.Observations) > 0 {
		b.WriteString("\nWhat you have observed so far (oldest first):\n")
		for _, o := range t.Observations {
			status := "ok"
			if o.IsError {
				status = "error"
			}
			fmt.Fprintf(&b, "- %s(%q) -> [%s] %s\n", o.Tool, o.Input, status, o.Output)
		}
		last := t.Observations[len(t.Observations)-1]
		fmt.Fprintf(&b, "\nYour most recent observation was %s -> %s\n", last.Tool, last.Output)
	}
	b.WriteString(`
Respond with ONLY a JSON object of this shape:
{"thought": "<one short sentence on what you are doing>",
 "tool": "<a tool name to call, or empty>",
 "input": "<the tool input, or empty>",
 "final": "<your final answer, only when the task is done and empty otherwise>"}

Rules:
- The repository already contains the files you need. Do not invent file names and do not create new files.
- Start by calling list_files, then read_file on the real files, before editing anything.
- Only write_file a path you have already seen from list_files or read_file.
- Look at your most recent observation before deciding.
- If the most recent run_tests observation says PASS, the task is DONE. Respond with an empty "tool" and put your summary in "final". Do not call any more tools.
- Never repeat an action whose result you already have above.
- To call write_file, put the file path on the first line of "input" and the full new file contents on the following lines.
- Call exactly one tool per response, unless you are done.`)
	return b.String()
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
