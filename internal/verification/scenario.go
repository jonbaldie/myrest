package verification

import "fmt"

// Outcome is the client-visible result a normative scenario asserts.
type Outcome string

const (
	Success     Outcome = "success"
	Refuse      Outcome = "refuse"
	Observation Outcome = "observation"
	Fallback    Outcome = "fallback"
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

// SmokeIDs is the fixed cross-area smoke set.
var SmokeIDs = []string{
	"smoke-001",
	"smoke-002",
	"smoke-003",
	"smoke-004",
	"smoke-005",
	"smoke-006",
}

// Behaviour is one labelled behaviour with the scenarios that prove it.
type Behaviour struct {
	Area      string
	Item      string
	Label     ParityLabel
	Scenarios []string
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
	for _, id := range SmokeIDs {
		if _, ok := index.ByID(id); !ok {
			return fmt.Errorf("smoke set missing %s", id)
		}
	}
	return nil
}
