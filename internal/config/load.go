package config

import (
	"fmt"
	"strings"
)

// Environment holds the process environment as variable name and value pairs.
type Environment map[string]string

// EnvironmentFrom reads os.Environ style `NAME=value` entries.
func EnvironmentFrom(entries []string) Environment {
	env := make(Environment, len(entries))
	for _, entry := range entries {
		if name, value, found := strings.Cut(entry, "="); found {
			env[name] = value
		}
	}
	return env
}

// Load resolves the settings from config file text and from the environment.
// Environment values win over file values, as the parity target does.
func Load(file string, env Environment) (Settings, error) {
	assignments, err := parseFile(file)
	if err != nil {
		return Settings{}, err
	}
	settings, err := applyFile(Defaults(), assignments)
	if err != nil {
		return Settings{}, err
	}
	return applyEnvironment(settings, env)
}

// applyFile applies the file assignments in file order.
func applyFile(settings Settings, assignments []assignment) (Settings, error) {
	definitions := definitionsByName()
	for _, item := range assignments {
		definition, known := definitions[item.knob]
		if !known {
			return Settings{}, fmt.Errorf(
				"config file line %d: %s is not a myrest knob", item.line, item.knob,
			)
		}
		applied, err := definition.apply(settings, item.value)
		if err != nil {
			return Settings{}, fmt.Errorf("config file line %d: %s: %w", item.line, item.knob, err)
		}
		settings = applied
	}
	return settings, nil
}

// applyEnvironment applies every MYREST_* variable that names a knob.
func applyEnvironment(settings Settings, env Environment) (Settings, error) {
	for _, definition := range knobDefinitions() {
		raw, present := env[definition.variable()]
		if !present {
			continue
		}
		applied, err := definition.apply(settings, raw)
		if err != nil {
			return Settings{}, fmt.Errorf("%s: %w", definition.variable(), err)
		}
		settings = applied
	}
	return settings, nil
}
