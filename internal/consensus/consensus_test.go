package consensus

import "testing"

func TestNormalizeCollapsesCosmetics(t *testing.T) {
	cases := map[string]string{
		"$0.30":  "0.30",
		"0.30.":  "0.30",
		" 0.30 ": "0.30",
		"1,000":  "1000",
		"Blue.":  "blue",
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTallyPicksMajorityAndSortsByCount(t *testing.T) {
	attempts := []Attempt{
		{Norm: "0.30"}, {Norm: "0.30"}, {Norm: "0.80"}, {Norm: "0.30"}, {Norm: "0.80"}, {Norm: ""},
	}
	votes, majority := tally(attempts)
	if majority != "0.30" {
		t.Fatalf("majority = %q, want 0.30", majority)
	}
	if len(votes) != 2 {
		t.Fatalf("got %d distinct votes, want 2", len(votes))
	}
	if votes[0].Answer != "0.30" || votes[0].Count != 3 {
		t.Errorf("top vote = %+v, want {0.30 3}", votes[0])
	}
	if votes[1].Count != 2 {
		t.Errorf("second vote count = %d, want 2", votes[1].Count)
	}
}

func TestDistinctAnswersDedupesByNormalizedForm(t *testing.T) {
	attempts := []Attempt{
		{Answer: "$0.30", Norm: "0.30"},
		{Answer: "0.30", Norm: "0.30"},
		{Answer: "0.80", Norm: "0.80"},
		{Answer: "", Norm: ""},
	}
	got := distinctAnswers(attempts)
	if len(got) != 2 {
		t.Fatalf("distinct = %v, want 2 entries", got)
	}
}
