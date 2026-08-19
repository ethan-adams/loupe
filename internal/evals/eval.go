package evals

import (
	"context"
	"encoding/json"
	"os"

	"github.com/ethan-adams/loupe/internal/consensus"
)

// TaskScore is how a model did on one problem: N solutions were generated and run
// against the problem's tests; the best one's pass rate is the headline number.
type TaskScore struct {
	Title      string  `json:"title"`
	N          int     `json:"n"`          // solutions generated
	Total      int     `json:"total"`      // test cases
	BestPassed int     `json:"bestPassed"` // tests the best solution passed
	BestRate   float64 `json:"bestRate"`   // BestPassed / Total, 0..1
	PassedAll  int     `json:"passedAll"`  // solutions that passed every test
	AvgPassed  float64 `json:"avgPassed"`  // mean tests passed across solutions
}

// Scorecard is one full run of the suite.
type Scorecard struct {
	Model   string      `json:"model"`
	RanAt   string      `json:"ranAt"` // set by the caller (Go time), so this stays pure
	Overall float64     `json:"overall"`
	Tasks   []TaskScore `json:"tasks"`
}

// Run executes every problem in the suite, N solutions each, and scores them.
func Run(ctx context.Context, suite []consensus.CodeProblem, n int, model string) (*Scorecard, error) {
	card := &Scorecard{Model: model}
	var sum float64
	for _, p := range suite {
		res, err := consensus.RunCode(ctx, p, n)
		if err != nil {
			return nil, err
		}
		card.Tasks = append(card.Tasks, score(p.Title, n, res))
	}
	for _, t := range card.Tasks {
		sum += t.BestRate
	}
	if len(card.Tasks) > 0 {
		card.Overall = sum / float64(len(card.Tasks))
	}
	return card, nil
}

func score(title string, n int, res *consensus.CodeResult) TaskScore {
	ts := TaskScore{Title: title, N: n, Total: res.Total, PassedAll: res.Passing}
	if res.Total > 0 && len(res.Attempts) > 0 {
		best := res.Attempts[res.BestIndex]
		ts.BestPassed = best.Passed
		ts.BestRate = float64(best.Passed) / float64(res.Total)
		var sum int
		for _, a := range res.Attempts {
			sum += a.Passed
		}
		ts.AvgPassed = float64(sum) / float64(len(res.Attempts))
	}
	return ts
}

// Regression is a task whose best pass rate dropped from the previous run.
type Regression struct {
	Title string
	Was   float64
	Now   float64
}

// Regressions compares a new scorecard against the previous one and flags any
// task whose best pass rate fell by more than a small margin.
func Regressions(prev, cur *Scorecard) []Regression {
	if prev == nil {
		return nil
	}
	was := map[string]float64{}
	for _, t := range prev.Tasks {
		was[t.Title] = t.BestRate
	}
	const eps = 0.001
	var out []Regression
	for _, t := range cur.Tasks {
		if w, ok := was[t.Title]; ok && t.BestRate < w-eps {
			out = append(out, Regression{Title: t.Title, Was: w, Now: t.BestRate})
		}
	}
	return out
}

// LoadHistory reads the scorecard history file (missing file is an empty history).
func LoadHistory(path string) ([]*Scorecard, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var cards []*Scorecard
	if err := json.Unmarshal(b, &cards); err != nil {
		return nil, err
	}
	return cards, nil
}

// SaveHistory appends a scorecard and writes the history back, keeping the most
// recent 50 so the file cannot grow without bound.
func SaveHistory(path string, cards []*Scorecard, add *Scorecard) error {
	cards = append(cards, add)
	if len(cards) > 50 {
		cards = cards[len(cards)-50:]
	}
	b, err := json.MarshalIndent(cards, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
