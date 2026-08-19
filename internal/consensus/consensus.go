// Package consensus runs one question many times and uses a gate to turn a spread
// of unreliable single answers into one reliable one. This is the thing a single
// agent (or a single model call) cannot do: sample the same task N ways, watch the
// answers diverge, and pick the right one by majority vote and by an independent
// judge. It is the front door to evals.
package consensus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ethan-adams/loupe/internal/model"
)

// Attempt is one independent answer to the question.
type Attempt struct {
	Index     int    `json:"index"`
	Reasoning string `json:"reasoning"`
	Answer    string `json:"answer"` // as the model gave it
	Norm      string `json:"norm"`   // normalized, for voting
	Err       string `json:"err,omitempty"`
}

// Vote is a normalized answer and how many attempts produced it.
type Vote struct {
	Answer string `json:"answer"`
	Count  int    `json:"count"`
}

// Result is the whole run: every attempt, the vote tally, and both gates.
type Result struct {
	Question string    `json:"question"`
	Expected string    `json:"expected,omitempty"`
	Attempts []Attempt `json:"attempts"`
	Tally    []Vote    `json:"tally"`    // most votes first
	Majority string    `json:"majority"` // the normalized answer with the most votes
	Judge    string    `json:"judge"`    // an independent judge's chosen answer
	JudgeWhy string    `json:"judgeWhy"`
}

const attemptPrompt = `Answer the question. Think briefly, then give your final answer.
Question: %s

Respond with ONLY a JSON object:
{"reasoning": "<2 to 3 sentences of your working>", "answer": "<the final answer only, as short as possible, no units or extra words>"}`

const judgePrompt = `A question was answered independently several times and the answers disagree.
Decide which one is correct. Work it out yourself; do not just pick the most common.

Question: %s

Candidate answers:
%s

Respond with ONLY a JSON object:
{"answer": "<the correct final answer, as short as possible>", "why": "<one sentence>"}`

// Run asks the question n times in parallel, tallies the answers, and judges them.
func Run(ctx context.Context, question, expected string, n int) (*Result, error) {
	m := model.NewOllama(&http.Client{Timeout: 120 * time.Second})
	res := &Result{Question: question, Expected: expected, Attempts: make([]Attempt, n)}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := Attempt{Index: i}
			// A different seed per attempt makes them diverge, and keeps the run
			// reproducible so a good instance can be recorded for the site demo.
			raw, err := m.Ask(ctx, fmt.Sprintf(attemptPrompt, question), map[string]any{
				"temperature": 0.7,
				"seed":        1000 + i,
			})
			if err != nil {
				a.Err = err.Error()
				res.Attempts[i] = a
				return
			}
			var parsed struct {
				Reasoning string `json:"reasoning"`
				Answer    string `json:"answer"`
			}
			if json.Unmarshal([]byte(raw), &parsed) != nil {
				a.Answer = strings.TrimSpace(raw)
			} else {
				a.Reasoning = strings.TrimSpace(parsed.Reasoning)
				a.Answer = strings.TrimSpace(parsed.Answer)
			}
			a.Norm = normalize(a.Answer)
			res.Attempts[i] = a
		}(i)
	}
	wg.Wait()

	res.Tally, res.Majority = tally(res.Attempts)

	if distinct := distinctAnswers(res.Attempts); len(distinct) > 0 {
		raw, err := m.Ask(ctx, fmt.Sprintf(judgePrompt, question, bulletList(distinct)),
			map[string]any{"temperature": 0.2, "seed": 7})
		if err == nil {
			var j struct {
				Answer string `json:"answer"`
				Why    string `json:"why"`
			}
			if json.Unmarshal([]byte(raw), &j) == nil {
				res.Judge = strings.TrimSpace(j.Answer)
				res.JudgeWhy = strings.TrimSpace(j.Why)
			}
		}
	}
	return res, nil
}

func tally(attempts []Attempt) ([]Vote, string) {
	counts := map[string]int{}
	rep := map[string]string{} // normalized form -> a representative original answer
	for _, a := range attempts {
		if a.Norm == "" {
			continue
		}
		counts[a.Norm]++
		if _, ok := rep[a.Norm]; !ok {
			rep[a.Norm] = a.Answer
		}
	}
	votes := make([]Vote, 0, len(counts))
	for k, v := range counts {
		votes = append(votes, Vote{Answer: rep[k], Count: v})
	}
	// Most votes first; ties broken by answer text so the order is stable.
	sort.Slice(votes, func(i, j int) bool {
		if votes[i].Count != votes[j].Count {
			return votes[i].Count > votes[j].Count
		}
		return votes[i].Answer < votes[j].Answer
	})
	majority := ""
	if len(votes) > 0 {
		majority = votes[0].Answer
	}
	return votes, majority
}

func distinctAnswers(attempts []Attempt) []string {
	seen := map[string]bool{}
	var out []string
	for _, a := range attempts {
		if a.Answer == "" || seen[a.Norm] {
			continue
		}
		seen[a.Norm] = true
		out = append(out, a.Answer)
	}
	return out
}

func bulletList(items []string) string {
	var b strings.Builder
	for _, it := range items {
		fmt.Fprintf(&b, "- %s\n", it)
	}
	return b.String()
}

// normalize collapses cosmetic differences so equal answers vote together.
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".")
	s = strings.NewReplacer("$", "", ",", "", " ", "").Replace(s)
	return s
}
