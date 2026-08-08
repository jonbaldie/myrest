package verification

import (
	"fmt"
	"regexp"
)

// Outcome is the client-visible result a normative scenario asserts.
type Outcome string

const (
	Success Outcome = "success"
	Refuse  Outcome = "refuse"
)

// Scenario is one index row for a normative scenario.
type Scenario struct {
	ID      string
	Area    string
	Label   ParityLabel
	Outcome Outcome
}

// ScenarioIndex is the Verification roll-up of every normative scenario.
type ScenarioIndex []Scenario

var scenarioIDPattern = regexp.MustCompile(`^[a-z]+-[0-9]{3}$`)

// smokeIDs is the fixed cross-area smoke set.
var smokeIDs = []string{
	"smoke-001",
	"smoke-002",
	"smoke-003",
	"smoke-004",
	"smoke-005",
	"smoke-006",
}

// Behaviour is one labelled behaviour with the scenarios that prove it.
type Behaviour struct {
	Item               string
	Label              ParityLabel
	Scenarios          []string
	ClientVisibleError bool
}

// Validate checks that every scenario has one stable, complete index row.
func (index ScenarioIndex) Validate() error {
	seen := make(map[string]struct{}, len(index))
	for _, scenario := range index {
		if !scenarioIDPattern.MatchString(scenario.ID) {
			return fmt.Errorf("scenario id %q must use area-nnn form", scenario.ID)
		}
		if _, ok := seen[scenario.ID]; ok {
			return fmt.Errorf("duplicate scenario id %s", scenario.ID)
		}
		seen[scenario.ID] = struct{}{}
		if scenario.Area == "" {
			return fmt.Errorf("scenario %s has no capability area", scenario.ID)
		}
		if !validParityLabel(scenario.Label) {
			return fmt.Errorf("scenario %s has unknown parity label %q", scenario.ID, scenario.Label)
		}
		if !validOutcome(scenario.Outcome) {
			return fmt.Errorf("scenario %s has unknown outcome %q", scenario.ID, scenario.Outcome)
		}
	}
	return nil
}

func validParityLabel(label ParityLabel) bool {
	return label == FullMatch || label == PartialMatch || label == NotSupported
}

func validOutcome(outcome Outcome) bool {
	return outcome == Success || outcome == Refuse
}

// ByID returns the scenario with the given id.
func (index ScenarioIndex) ByID(id string) (Scenario, bool) {
	for _, scenario := range index {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return Scenario{}, false
}

// RequireSmokeSet checks that smoke-001..smoke-006 are present in the index.
func (index ScenarioIndex) RequireSmokeSet() error {
	for _, id := range smokeIDs {
		if _, ok := index.ByID(id); !ok {
			return fmt.Errorf("smoke set missing %s", id)
		}
	}
	return nil
}
