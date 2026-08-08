package verification_test

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/verification"
)

func TestScenarioIndexRejectsMalformedOrDuplicateRows(t *testing.T) {
	valid := verification.Scenario{
		ID: "read-001", Area: "read", Label: verification.FullMatch, Outcome: verification.Success,
	}
	cases := map[string]verification.ScenarioIndex{
		"malformed id":    {{ID: "read-1", Area: "read", Label: verification.FullMatch, Outcome: verification.Success}},
		"duplicate id":    {valid, valid},
		"missing area":    {{ID: "read-001", Label: verification.FullMatch, Outcome: verification.Success}},
		"unknown label":   {{ID: "read-001", Area: "read", Label: "planned", Outcome: verification.Success}},
		"unknown outcome": {{ID: "read-001", Area: "read", Label: verification.FullMatch, Outcome: "planned"}},
		"full match observation": {{
			ID: "read-001", Area: "read", Label: verification.FullMatch, Outcome: "observation",
		}},
	}
	for name, index := range cases {
		t.Run(name, func(t *testing.T) {
			if err := index.Validate(); err == nil {
				t.Fatalf("Validate(%+v) = nil, want an invalid-index error", index)
			}
		})
	}
	if err := (verification.ScenarioIndex{valid}).Validate(); err != nil {
		t.Fatalf("Validate(valid): %v", err)
	}
}
