// Package evals turns the best-of-N code gate into a measurement tool: run a
// suite of coding problems against a model, score each by how well its best
// solution does on hidden tests, and compare a run against the last one to catch
// regressions. Evals is a named applied-AI skill, and this is a small honest
// version of it: the score is real (tests actually run), not a vibe.
package evals

import "github.com/ethan-adams/loupe/internal/consensus"

// DefaultSuite is a handful of small, self-contained coding problems chosen to
// span difficulty for this class of local model: one it nearly always nails, one
// it usually gets, and one where its solutions reliably diverge.
func DefaultSuite() []consensus.CodeProblem {
	return []consensus.CodeProblem{VersionProblem, QueryProblem, DurationProblem}
}

// VersionProblem: compare two dotted version strings. The model gets the numeric
// comparison right but trips on trailing-zero equality (1.0 vs 1.0.0).
var VersionProblem = consensus.CodeProblem{
	Title: "Compare version strings",
	Prompt: `Write a Python function compare_versions(a, b) that compares two dot-separated version strings such as '1.2.0' or '2.0'.
Return -1 if a is a lower version than b, 0 if they are the same version, and 1 if a is higher.
Define only the function compare_versions.`,
	Harness: `def _check():
    cases = [
        ("1.0.0", "1.0.0", 0),
        ("1.2.0", "1.1.9", 1),
        ("1.9.0", "1.10.0", -1),
        ("1.0", "1.0.0", 0),
        ("2.0.0", "1.9.9", 1),
        ("1.0.1", "1.0.10", -1),
        ("0.9.0", "1.0.0", -1),
        ("1.0.0", "1.2.0", -1),
        ("1.10.2", "1.10.2", 0),
        ("1.2", "1.10", -1),
    ]
    p = 0
    for a, b, exp in cases:
        try:
            if compare_versions(a, b) == exp:
                p += 1
        except Exception:
            pass
    print("RESULT %d/%d" % (p, len(cases)))

_check()`,
}

// QueryProblem: parse a URL query string. Solutions diverge and share blind spots
// (no-value keys, a leading '?').
var QueryProblem = consensus.CodeProblem{
	Title: "Parse a query string",
	Prompt: `Write a Python function parse_query(s) that parses a URL query string like 'a=1&b=2' into a dictionary.
Each key maps to the list of its values, since the same key can appear more than once.
Define only the function parse_query.`,
	Harness: `def _check():
    cases = [
        ("a=1&b=2", {"a": ["1"], "b": ["2"]}),
        ("a=1&a=2", {"a": ["1", "2"]}),
        ("", {}),
        ("flag", {"flag": [""]}),
        ("a=1&b=", {"a": ["1"], "b": [""]}),
        ("?x=9", {"x": ["9"]}),
        ("a=b=c", {"a": ["b=c"]}),
        ("a=1&a=2&a=3", {"a": ["1", "2", "3"]}),
        ("k", {"k": [""]}),
        ("p=1&q=2&p=3", {"p": ["1", "3"], "q": ["2"]}),
    ]
    p = 0
    for s, exp in cases:
        try:
            if parse_query(s) == exp:
                p += 1
        except Exception:
            pass
    print("RESULT %d/%d" % (p, len(cases)))

_check()`,
}

// DurationProblem: parse a duration string into milliseconds. The 'ms' unit is a
// consistent trap, so scores stay low; a good task for spotting a real drop.
var DurationProblem = consensus.CodeProblem{
	Title: "Parse a duration string",
	Prompt: `Write a Python function parse_duration(s) that parses a duration string into a total number of milliseconds (an integer), or returns None if the string is not a valid duration.
Rules:
- A duration is one or more number+unit pairs written together, for example '1h30m', '500ms', '2h', '90m', '1d12h', '45s'.
- Units are: 'ms' = milliseconds, 's' = seconds, 'm' = minutes, 'h' = hours, 'd' = days.
- Sum all the number+unit pairs and return the total in milliseconds.
- If the string is empty, contains no number, or contains an unknown unit, return None.
Define only the function parse_duration.`,
	Harness: `def _check():
    cases = [
        ("1h30m", 5400000),
        ("2h", 7200000),
        ("90m", 5400000),
        ("1d12h", 129600000),
        ("45s", 45000),
        ("500ms", 500),
        ("250ms", 250),
        ("2d", 172800000),
        ("abc", None),
        ("1h30m15s", 5415000),
    ]
    p = 0
    for s, exp in cases:
        try:
            got = parse_duration(s)
            ok = (got is None) if exp is None else (got is not None and int(got) == exp)
            if ok:
                p += 1
        except Exception:
            pass
    print("RESULT %d/%d" % (p, len(cases)))

_check()`,
}
