package verification_test

import (
	"testing"

	"github.com/jonbaldie/myrest/internal/verification"
)

func TestCoverageDutyForPartialMatchNeedsSuccessAndRefuse(t *testing.T) {
	index := verification.ScenarioIndex{
		{ID: "read-003", Area: "read", Label: verification.PartialMatch, Outcome: verification.Success},
		{ID: "read-004", Area: "read", Label: verification.PartialMatch, Outcome: verification.Refuse},
	}
	gaps := []verification.GapRow{{
		Item:      "Text-case subset",
		Label:     verification.PartialMatch,
		Scenarios: []string{"read-003", "read-004"},
	}}
	if err := verification.CheckCoverage(gaps, index); err != nil {
		t.Fatalf("CheckCoverage: %v", err)
	}
}

func TestCoverageDutyRejectsPartialMatchWithoutRefuse(t *testing.T) {
	index := verification.ScenarioIndex{
		{ID: "read-003", Area: "read", Label: verification.PartialMatch, Outcome: verification.Success},
	}
	gaps := []verification.GapRow{{
		Item:      "Text-case subset",
		Label:     verification.PartialMatch,
		Scenarios: []string{"read-003"},
	}}
	if err := verification.CheckCoverage(gaps, index); err == nil {
		t.Fatal("CheckCoverage() = nil, want a missing refuse error")
	}
}

func TestCoverageDutyForNotSupportedNeedsRefuseOnly(t *testing.T) {
	index := verification.ScenarioIndex{
		{ID: "read-007", Area: "read", Label: verification.NotSupported, Outcome: verification.Refuse},
		{ID: "smoke-006", Area: "verification", Label: verification.NotSupported, Outcome: verification.Refuse},
	}
	gaps := []verification.GapRow{{
		Item:      "FTS operators",
		Label:     verification.NotSupported,
		Scenarios: []string{"read-007", "smoke-006"},
	}}
	if err := verification.CheckCoverage(gaps, index); err != nil {
		t.Fatalf("CheckCoverage: %v", err)
	}
}

func TestCoverageDutyRejectsNotSupportedSuccessPath(t *testing.T) {
	index := verification.ScenarioIndex{
		{ID: "bad-001", Area: "read", Label: verification.NotSupported, Outcome: verification.Success},
	}
	gaps := []verification.GapRow{{
		Item:      "FTS operators",
		Label:     verification.NotSupported,
		Scenarios: []string{"bad-001"},
	}}
	if err := verification.CheckCoverage(gaps, index); err == nil {
		t.Fatal("CheckCoverage() = nil, want a success-path error")
	}
}

func TestFullMatchCoverageNeedsSuccess(t *testing.T) {
	behaviours := []verification.Behaviour{
		{
			Item:      "Bearer JWT ordinary read",
			Area:      "auth",
			Label:     verification.FullMatch,
			Scenarios: []string{"auth-001"},
		},
	}
	index := verification.ScenarioIndex{
		{ID: "auth-001", Area: "auth", Label: verification.FullMatch, Outcome: verification.Success},
	}
	if err := verification.CheckBehaviourCoverage(behaviours, index); err != nil {
		t.Fatalf("CheckBehaviourCoverage: %v", err)
	}
}
