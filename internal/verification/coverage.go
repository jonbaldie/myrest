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
	if behaviour.Item == "" {
		return fmt.Errorf("no behaviour item")
	}
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
	success bool
	refuse  bool
}

func outcomeFlags(behaviour Behaviour, index ScenarioIndex) (outcomeSet, error) {
	var flags outcomeSet
	for _, id := range behaviour.Scenarios {
		scenario, ok := index.ByID(id)
		if !ok {
			return outcomeSet{}, fmt.Errorf("%s: unknown scenario %s", behaviour.Item, id)
		}
		if scenario.Label != behaviour.Label {
			return outcomeSet{}, fmt.Errorf(
				"%s: scenario %s has parity label %q, want %q",
				behaviour.Item, id, scenario.Label, behaviour.Label,
			)
		}
		switch scenario.Outcome {
		case Success:
			flags.success = true
		case Refuse:
			flags.refuse = true
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
		return fullMatchDuty(behaviour, flags)
	case PartialMatch:
		return partialMatchDuty(behaviour.Item, flags)
	case NotSupported:
		return notSupportedDuty(behaviour.Item, flags)
	default:
		return fmt.Errorf("%s: unknown parity label %q", behaviour.Item, behaviour.Label)
	}
}

func fullMatchDuty(behaviour Behaviour, flags outcomeSet) error {
	if behaviour.ClientVisibleError {
		if !flags.refuse {
			return fmt.Errorf("%s: claimed client-visible error needs a refuse scenario", behaviour.Item)
		}
		return nil
	}
	if !flags.success {
		return fmt.Errorf("%s: full match needs a success scenario", behaviour.Item)
	}
	return nil
}

func partialMatchDuty(item string, flags outcomeSet) error {
	if !flags.success {
		return fmt.Errorf("%s: partial match needs an in-subset success scenario", item)
	}
	if !flags.refuse {
		return fmt.Errorf("%s: partial match needs an outside-subset refuse scenario", item)
	}
	return nil
}

func notSupportedDuty(item string, flags outcomeSet) error {
	if !flags.refuse {
		return fmt.Errorf("%s: not supported needs a refuse scenario", item)
	}
	if flags.success {
		return fmt.Errorf("%s: not supported must not claim a success scenario", item)
	}
	return nil
}
