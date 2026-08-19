package consensus

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ethan-adams/loupe/internal/model"
)

// CodeProblem is a coding task with a deterministic test harness. The model is
// asked for a solution N times; each solution is actually executed against the
// harness. That execution is the gate: on code, "does it pass the tests" beats
// any vote or judge opinion.
type CodeProblem struct {
	Title   string // short label
	Prompt  string // the problem statement, including the exact function signature
	Harness string // Python appended after the candidate; must print "RESULT p/t"
}

// CodeAttempt is one generated solution and how it did against the tests.
type CodeAttempt struct {
	Index  int    `json:"index"`
	Code   string `json:"code"`
	Passed int    `json:"passed"`
	Total  int    `json:"total"`
	Ok     bool   `json:"ok"` // passed all tests
	Note   string `json:"note,omitempty"`
}

// CodeResult is the whole run.
type CodeResult struct {
	Title     string        `json:"title"`
	Prompt    string        `json:"prompt"`
	Attempts  []CodeAttempt `json:"attempts"`
	BestIndex int           `json:"bestIndex"` // the solution to ship (passed the most)
	Total     int           `json:"total"`     // number of test cases
	Passing   int           `json:"passing"`   // how many attempts passed everything
}

const codePrompt = `%s

Respond with ONLY a JSON object:
{"reasoning": "<one short sentence>", "code": "<the complete Python function, no markdown fences, no explanation>"}`

var resultLine = regexp.MustCompile(`RESULT (\d+)/(\d+)`)

// RunCode asks for the solution n times in parallel and runs every candidate
// against the harness.
func RunCode(ctx context.Context, p CodeProblem, n int) (*CodeResult, error) {
	m := model.NewOllama(&http.Client{Timeout: 180 * time.Second})
	res := &CodeResult{Title: p.Title, Prompt: p.Prompt, Attempts: make([]CodeAttempt, n)}

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := CodeAttempt{Index: i}
			// Two seeds to try: a no-code result is usually a transient JSON miss,
			// so one retry markedly improves yield without hiding real failures.
			for _, seed := range []int{2000 + i, 2500 + i} {
				raw, err := m.Ask(ctx, fmt.Sprintf(codePrompt, p.Prompt), map[string]any{
					"temperature": 0.8,
					"seed":        seed,
				})
				if err != nil {
					a.Note = err.Error()
					continue
				}
				var parsed struct {
					Code string `json:"code"`
				}
				_ = json.Unmarshal([]byte(raw), &parsed)
				if c := normalizeCode(stripFences(strings.TrimSpace(parsed.Code))); c != "" {
					a.Code = c
					a.Note = ""
					break
				}
				a.Note = "no code produced"
			}
			if a.Code == "" {
				res.Attempts[i] = a
				return
			}
			passed, total, note := runCandidate(ctx, a.Code, p.Harness)
			a.Passed, a.Total, a.Note = passed, total, note
			a.Ok = total > 0 && passed == total
			res.Attempts[i] = a
		}(i)
	}
	wg.Wait()

	res.BestIndex, res.Total, res.Passing = summarize(res.Attempts)
	return res, nil
}

// runCandidate writes the candidate plus the harness to a temp file, runs it in
// an isolated Python interpreter with a timeout, and reads the RESULT line. A
// candidate that will not even run counts as zero passes, which is a real
// failure mode worth showing.
func runCandidate(ctx context.Context, code, harness string) (passed, total int, note string) {
	src := code + "\n\n" + harness
	f, err := os.CreateTemp("", "loupe-cand-*.py")
	if err != nil {
		return 0, 0, err.Error()
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(src); err != nil {
		return 0, 0, err.Error()
	}
	f.Close()

	cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(cctx, "python3", "-I", f.Name()).CombinedOutput()

	if mt := resultLine.FindSubmatch(out); mt != nil {
		fmt.Sscan(string(mt[1]), &passed)
		fmt.Sscan(string(mt[2]), &total)
		return passed, total, ""
	}
	return 0, 0, "did not run to completion"
}

func summarize(attempts []CodeAttempt) (best, total, passing int) {
	best = -1
	bestPassed := -1
	for i, a := range attempts {
		if a.Total > total {
			total = a.Total
		}
		if a.Ok {
			passing++
		}
		if a.Passed > bestPassed {
			bestPassed, best = a.Passed, i
		}
	}
	if best < 0 {
		best = 0
	}
	return best, total, passing
}

// normalizeCode fixes the occasional model output that escapes newlines as the
// two literal characters backslash-n instead of real newlines, which would never
// run. Only touches code that is a single line yet contains those sequences, so
// real multi-line code (and genuine \n inside string literals) is left alone.
func normalizeCode(s string) string {
	if !strings.Contains(s, "\n") && strings.Contains(s, `\n`) {
		s = strings.NewReplacer(`\n`, "\n", `\t`, "\t").Replace(s)
	}
	return s
}

// stripFences removes a leading ```lang / trailing ``` if the model wrapped its
// code in a markdown block despite being asked not to.
func stripFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}
