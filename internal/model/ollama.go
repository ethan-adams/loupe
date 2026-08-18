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
	m := env("LOUPE_OLLAMA_MODEL", "llama3.1:8b")
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
	body := map[string]any{
		"model":  o.model,
		"prompt": buildPrompt(t),
		"stream": false,
		"format": "json", // ask Ollama to constrain output to JSON
	}
	buf, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/generate", bytes.NewReader(buf))
	if err != nil {
		return Decision{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return Decision{}, fmt.Errorf("ollama request failed (is `ollama serve` running?): %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Decision{}, fmt.Errorf("ollama returned %s", resp.Status)
	}

	var envelope struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return Decision{}, fmt.Errorf("decode ollama envelope: %w", err)
	}

	var dj decisionJSON
	if err := json.Unmarshal([]byte(envelope.Response), &dj); err != nil {
		// The model ignored the JSON instruction; treat its prose as a final
		// answer rather than crashing the run.
		return Decision{Thought: "(model returned unstructured text)", Final: strings.TrimSpace(envelope.Response)}, nil
	}

	d := Decision{Thought: strings.TrimSpace(dj.Thought)}
	if tool := strings.TrimSpace(dj.Tool); tool != "" {
		d.ToolCall = &ToolCall{Name: tool, Input: dj.Input}
	} else {
		d.Final = strings.TrimSpace(dj.Final)
	}
	return d, nil
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
		b.WriteString("\nWhat you have observed so far:\n")
		for _, o := range t.Observations {
			status := "ok"
			if o.IsError {
				status = "error"
			}
			fmt.Fprintf(&b, "- %s(%q) -> [%s] %s\n", o.Tool, o.Input, status, o.Output)
		}
	}
	b.WriteString(`
Respond with ONLY a JSON object of this shape:
{"thought": "<one short sentence on what you are doing>",
 "tool": "<a tool name to call, or empty>",
 "input": "<the tool input, or empty>",
 "final": "<your final answer, only when the task is done and empty otherwise>"}
To call write_file, put the file path on the first line of "input" and the full new file contents on the following lines.
Call exactly one tool per response, or set "final" when the tests pass.`)
	return b.String()
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
