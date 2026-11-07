package evaluation

import (
	"encoding/json"
	"strings"
	"testing"
)

func fixture() Report {
	return Report{SchemaVersion: "skillhub.evaluation.v1", SkillDigest: "sha256:" + strings.Repeat("a", 64), Suite: Suite{SchemaVersion: "skillhub.cases.v1", Name: "api-review", Version: "1", Cases: []Case{{ID: "missing-input", Input: "Review an unspecified change", Expected: "Ask for the proposed change"}}}, Agent: "example-agent/1", Model: "example-model/1", Runner: "manual/1", Settings: "default; tools disabled", ExecutedAt: "2026-01-01T00:00:00Z", Results: []Result{{CaseID: "missing-input", Outcome: Outcome{Status: "pass", Observed: "Asked for change"}, Baseline: &Outcome{Status: "fail", Observed: "Guessed the change"}}}}
}
func TestReportStrictnessAndSummary(t *testing.T) {
	r := fixture()
	parsed, err := Parse(r.Canonical())
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Summary().Passed != 1 || parsed.Summary().BaselineTotal != 1 || parsed.Summary().BaselinePassed != 0 {
		t.Fatal("wrong summary")
	}
	if _, err = ParseSuite(mustJSON(r.Suite)); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Report){"schema": func(r *Report) { r.SchemaVersion = "future" }, "digest": func(r *Report) { r.SkillDigest = "fake" }, "duplicate": func(r *Report) { r.Suite.Cases = append(r.Suite.Cases, r.Suite.Cases[0]) }, "missing": func(r *Report) { r.Results = nil }, "unknown-case": func(r *Report) { r.Results[0].CaseID = "other" }, "status": func(r *Report) { r.Results[0].Status = "success" }, "time": func(r *Report) { r.ExecutedAt = "tomorrow" }, "large": func(r *Report) { r.Results[0].Observed = strings.Repeat("a", 8193) }} {
		t.Run(name, func(t *testing.T) {
			r := fixture()
			change(&r)
			if _, err := Parse(r.Canonical()); err == nil {
				t.Fatal("accepted invalid report")
			}
		})
	}
	for _, raw := range []string{`{"schema_version":"a","schema_version":"b"}`, `null`, strings.Replace(string(r.Canonical()), `"agent":`, `"Agent":`, 1), strings.Replace(string(r.Canonical()), `"agent":"example-agent/1"`, `"agent":null`, 1), string(r.Canonical()) + `{}`, strings.Replace(string(r.Canonical()), `"agent":`, `"unexpected":1,"agent":`, 1), strings.Repeat("[", 18) + "0" + strings.Repeat("]", 18), strings.Repeat(" ", MaxBytes+1)} {
		if _, err := Parse([]byte(raw)); err == nil {
			t.Fatal("accepted malformed input")
		}
	}
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
