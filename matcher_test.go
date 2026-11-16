package modelfamily

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func candidate(t *testing.T) *Rules {
	t.Helper()
	raw, err := os.ReadFile("examples/candidate.json")
	if err != nil {
		t.Fatal(err)
	}
	var rules Rules
	if err = json.Unmarshal(raw, &rules); err != nil {
		t.Fatal(err)
	}
	return &rules
}

func approved(t *testing.T) *Rules {
	t.Helper()
	rules, err := Approve(candidate(t), "Example reviewer", "Checked the synthetic manual and option constraints")
	if err != nil {
		t.Fatal(err)
	}
	return rules
}

func TestPendingRequiresReview(t *testing.T) {
	if _, err := Compile(candidate(t), "v1"); err == nil {
		t.Fatal("pending candidate compiled")
	}
}

func TestOptionsConstraintsAndNoPrefixGuessing(t *testing.T) {
	index, err := Compile(approved(t), "v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"METER100-A", "meter100-a-x", "METER100-B-X"} {
		if got := index.Resolve(input); got.Status != "resolved" || got.Match.Model != "METER100" {
			t.Errorf("%s: %+v", input, got)
		}
	}
	for _, input := range []string{"METER100-B", "METER100-C-X", "METER100-A-X-Z", "METER1000-A", "VendorMETER100-A"} {
		if got := index.Resolve(input); got.Status != "not_found" {
			t.Errorf("%s: %+v", input, got)
		}
	}
}

func TestStaleSourceFingerprintRejected(t *testing.T) {
	rules := approved(t)
	rules.Selection[0].Rule = json.RawMessage(strings.Replace(string(rules.Selection[0].Rule), "Synthetic selection table", "Changed source table", 1))
	if _, err := Compile(rules, "v2"); err == nil {
		t.Fatal("changed evidence compiled without review")
	}
}

func TestConflictingIdentitiesRemainAmbiguous(t *testing.T) {
	rules := approved(t)
	rules.Models["OTHER"] = json.RawMessage(`{"family":"OTHER","aliases":["METER100-A"]}`)
	index, err := Compile(rules, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if got := index.Resolve("METER100-A"); got.Status != "needs_clarification" {
		t.Fatalf("%+v", got)
	}
}

func TestMissingEvidenceAndUnsupportedScopeRejected(t *testing.T) {
	for _, test := range []string{"sha", "pages", "scope"} {
		rules := candidate(t)
		var value map[string]any
		_ = json.Unmarshal(rules.Selection[0].Rule, &value)
		switch test {
		case "sha":
			value["source_sha256"] = "missing"
		case "pages":
			value["physical_pages"] = []int{}
		case "scope":
			rules.Selection[0].Scope = "custom"
		}
		rules.Selection[0].Rule, _ = json.Marshal(value)
		if _, err := Approve(rules, "Reviewer", "Review"); err == nil {
			t.Errorf("accepted invalid %s", test)
		}
	}
}

func TestMultiChoiceOrderAndRepeatLimits(t *testing.T) {
	rules := candidate(t)
	var raw map[string]any
	_ = json.Unmarshal(rules.Selection[0].Rule, &raw)
	raw["dims"] = []any{map[string]any{"type": "multi", "required": true, "codes": map[string]any{"A": map[string]any{}, "B": map[string]any{}}}}
	raw["deterministic"] = map[string]any{"version": 1, "patterns": []string{"METER100-①"}, "dimensions": map[string]any{"1": map[string]any{"order": []string{"B", "A"}, "max_count": 2, "separator": "", "permutations": false, "repeat_limits": map[string]int{"B": 2}}}}
	rules.Selection[0].Rule, _ = json.Marshal(raw)
	rules, err := Approve(rules, "Reviewer", "Reviewed multi-choice ordering")
	if err != nil {
		t.Fatal(err)
	}
	index, err := Compile(rules, "v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{"METER100-BB", "METER100-BA"} {
		if index.Resolve(input).Status != "resolved" {
			t.Error(input)
		}
	}
	for _, input := range []string{"METER100-AB", "METER100-AA", "METER100-BBB"} {
		if index.Resolve(input).Status != "not_found" {
			t.Error(input)
		}
	}
}

func TestBlockedIdentityAndInvalidInput(t *testing.T) {
	rules := approved(t)
	rules.Models["BLOCKED"] = json.RawMessage(`{"family":"BLOCKED","blocked":true}`)
	index, err := Compile(rules, "v1")
	if err != nil {
		t.Fatal(err)
	}
	if index.Resolve("BLOCKED").Status != "not_found" || index.Resolve("").Status != "invalid_request" {
		t.Fatal("bad resolution")
	}
}
