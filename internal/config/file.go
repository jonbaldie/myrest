package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// assignment is one `knob = value` line of a config file.
type assignment struct {
	line  int
	knob  string
	value string
}

// parseFile reads config file text as assignments, in file order. Blank lines
// and lines that start with `#` carry no assignment.
func parseFile(text string) ([]assignment, error) {
	var assignments []assignment
	for index, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		number := index + 1
		knob, value, err := parseAssignment(line)
		if err != nil {
			return nil, fmt.Errorf("config file line %d: %w", number, err)
		}
		assignments = append(assignments, assignment{line: number, knob: knob, value: value})
	}
	return assignments, nil
}

// parseAssignment splits one line into its knob name and its text value.
func parseAssignment(line string) (string, string, error) {
	name, rawValue, found := strings.Cut(line, "=")
	if !found {
		return "", "", errors.New(`the line is not a "knob = value" pair`)
	}
	knob := strings.TrimSpace(name)
	if knob == "" {
		return "", "", errors.New("the line has no knob name")
	}
	value, err := parseValue(strings.TrimSpace(rawValue))
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", knob, err)
	}
	return knob, value, nil
}

// parseValue reads a value that is either a quoted string or a bare token.
func parseValue(raw string) (string, error) {
	if !strings.HasPrefix(raw, `"`) {
		return raw, nil
	}
	value, err := strconv.Unquote(raw)
	if err != nil {
		return "", fmt.Errorf("%s is not a quoted value", raw)
	}
	return value, nil
}
