package evals

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ethan-adams/loupe/internal/consensus"
)

func TestScoreSummarizesACodeResult(t *testing.T) {
	res := &consensus.CodeResult{
		Total:     10,
		BestIndex: 0,
		Passing:   1,
		Attempts: []consensus.CodeAttempt{
			{Passed: 10, Total: 10, Ok: true},
			{Passed: 4, Total: 10},
			{Passed: 6, Total: 10},
		},
	}
	ts := score("T", 3, res)
	if ts.BestPassed != 10 || ts.BestRate != 1.0 {
		t.Fatalf("best = %d (%.2f), want 10 (1.00)", ts.BestPassed, ts.BestRate)
	}
	if ts.PassedAll != 1 {
		t.Errorf("passedAll = %d, want 1", ts.PassedAll)
	}
	if ts.AvgPassed < 6.6 || ts.AvgPassed > 6.7 {
		t.Errorf("avgPassed = %.2f, want ~6.67", ts.AvgPassed)
	}
}

func TestRegressionsFlagsOnlyDrops(t *testing.T) {
	prev := &Scorecard{Tasks: []TaskScore{
		{Title: "a", BestRate: 0.9},
		{Title: "b", BestRate: 0.6},
	}}
	cur := &Scorecard{Tasks: []TaskScore{
		{Title: "a", BestRate: 0.7}, // dropped
		{Title: "b", BestRate: 0.6}, // same
		{Title: "c", BestRate: 0.1}, // new, not a regression
	}}
	regs := Regressions(prev, cur)
	if len(regs) != 1 || regs[0].Title != "a" {
		t.Fatalf("regressions = %+v, want just a", regs)
	}
	if regs[0].Was != 0.9 || regs[0].Now != 0.7 {
		t.Errorf("regression detail = %+v", regs[0])
	}
}

func TestRegressionsNilPrevIsEmpty(t *testing.T) {
	if r := Regressions(nil, &Scorecard{}); r != nil {
		t.Errorf("nil prev should give no regressions, got %+v", r)
	}
}

func TestHistoryRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hist.json")

	if h, err := LoadHistory(path); err != nil || h != nil {
		t.Fatalf("missing file should load empty: %v, %v", h, err)
	}
	if err := SaveHistory(path, nil, &Scorecard{Model: "m", Overall: 0.8}); err != nil {
		t.Fatal(err)
	}
	h, err := LoadHistory(path)
	if err != nil || len(h) != 1 || h[0].Overall != 0.8 {
		t.Fatalf("round trip failed: %v, %v", h, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("history file missing: %v", err)
	}
}
