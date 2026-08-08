package verification

import "fmt"

// CheckCoverage confirms every gap row meets the coverage duty for its label.
func CheckCoverage(gaps []GapRow, index ScenarioIndex) error {
	for _, gap := range gaps {
		behaviour := Behaviour{
			Item:      gap.Item,
			Label:     gap.Label,
			Scenarios: gap.Scenarios,
		}
		if err := checkOne(behaviour, index); err != nil {
			return err
		}
	}
	return nil
}

// CheckBehaviourCoverage confirms every labelled behaviour meets its duty.
func CheckBehaviourCoverage(behaviours []Behaviour, index ScenarioIndex) error {
	for _, behaviour := range behaviours {
		if err := checkOne(behaviour, index); err != nil {
			return err
		}
	}
	return nil
}

func checkOne(behaviour Behaviour, index ScenarioIndex) error {
	if len(behaviour.Scenarios) == 0 {
		return fmt.Errorf("%s (%s): no scenarios", behaviour.Item, behaviour.Label)
	}
	flags, err := outcomeFlags(behaviour, index)
	if err != nil {
		return err
	}
	return dutyError(behaviour, flags)
}

type outcomeSet struct {
	success     bool
	refuse      bool
	observation bool
	fallback    bool
}

func outcomeFlags(behaviour Behaviour, index ScenarioIndex) (outcomeSet, error) {
	var flags outcomeSet
	for _, id := range behaviour.Scenarios {
		scenario, ok := index.ByID(id)
		if !ok {
			return outcomeSet{}, fmt.Errorf("%s: unknown scenario %s", behaviour.Item, id)
		}
		switch scenario.Outcome {
		case Success:
			flags.success = true
		case Refuse:
			flags.refuse = true
		case Observation:
			flags.observation = true
		case Fallback:
			flags.fallback = true
		default:
			return outcomeSet{}, fmt.Errorf(
				"%s: scenario %s has unknown outcome %q",
				behaviour.Item, id, scenario.Outcome,
			)
		}
	}
	return flags, nil
}

func dutyError(behaviour Behaviour, flags outcomeSet) error {
	switch behaviour.Label {
	case FullMatch:
		return fullMatchDuty(behaviour.Item, flags)
	case PartialMatch:
		return partialMatchDuty(behaviour.Item, flags)
	case NotSupported:
		return notSupportedDuty(behaviour.Item, flags)
	default:
		return fmt.Errorf("%s: unknown parity label %q", behaviour.Item, behaviour.Label)
	}
}

func fullMatchDuty(item string, flags outcomeSet) error {
	// A full-match success path needs Success. A claimed client-visible
	// error for a full-match behaviour is a Refuse scenario on its own.
	if !flags.success && !flags.refuse {
		return fmt.Errorf("%s: full match needs a success or claimed-error scenario", item)
	}
	return nil
}

func partialMatchDuty(item string, flags outcomeSet) error {
	if !flags.success && !flags.observation {
		return fmt.Errorf("%s: partial match needs an in-subset success scenario", item)
	}
	haveOutside := flags.refuse || flags.fallback
	// Observational partial matches (for example CURRENT_USER after role
	// switch) have no outside-subset path of their own.
	observationOnly := flags.observation && !flags.success && !haveOutside
	if !haveOutside && !observationOnly {
		return fmt.Errorf("%s: partial match needs an outside-subset refuse scenario", item)
	}
	return nil
}

func notSupportedDuty(item string, flags outcomeSet) error {
	// A stable refuse or a documented non-offer observation both satisfy
	// the parent "refuse / non-offer" duty.
	if !flags.refuse && !flags.observation {
		return fmt.Errorf("%s: not supported needs a refuse scenario", item)
	}
	if flags.success || flags.fallback {
		return fmt.Errorf("%s: not supported must not claim a success scenario", item)
	}
	return nil
}
