package readquery

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// PathStep is one leg of a PostgREST JSON arrow path inside the MySQL subset.
type PathStep struct {
	Key     string
	Index   int
	IsIndex bool
}

// JSONPath is a named MySQL-subset JSON path on a column.
type JSONPath struct {
	Steps  []PathStep
	AsText bool
}

// UnsupportedFeature is a documented MySQL gap found while preparing a read
// against a resource (for example a JSON path on a non-JSON column).
type UnsupportedFeature struct {
	Message string
}

func (e UnsupportedFeature) Error() string { return e.Message }

// parseField reads a plain column name or a JSON path field
// (col->key / col->>key / chained legs).
func parseField(raw string) (name string, path *JSONPath, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil, ParseFailure{Message: "column name is empty"}
	}
	if strings.Contains(raw, "#>") {
		return "", nil, ParseFailure{
			Message: "PostgREST #> and #>> JSON path operators are not available with MySQL",
			Gap:     true,
		}
	}
	arrow := strings.Index(raw, "->")
	if arrow < 0 {
		return raw, nil, nil
	}
	name = raw[:arrow]
	if name == "" || !isJSONPathIdentifier(name) {
		return "", nil, ParseFailure{
			Message: fmt.Sprintf("JSON path column '%s' is outside the documented MySQL subset", name),
			Gap:     true,
		}
	}
	path, err = parseJSONPath(raw[arrow:])
	if err != nil {
		return "", nil, err
	}
	return name, path, nil
}

func parseJSONPath(raw string) (*JSONPath, error) {
	path := &JSONPath{}
	rest := raw
	for rest != "" {
		asText := false
		switch {
		case strings.HasPrefix(rest, "->>"):
			asText = true
			rest = rest[3:]
		case strings.HasPrefix(rest, "->"):
			rest = rest[2:]
		default:
			return nil, ParseFailure{
				Message: "JSON path needs -> or ->> legs",
				Gap:     true,
			}
		}
		step, next, err := readPathStep(rest)
		if err != nil {
			return nil, err
		}
		path.Steps = append(path.Steps, step)
		path.AsText = asText
		rest = next
	}
	if len(path.Steps) == 0 {
		return nil, ParseFailure{
			Message: "JSON path needs at least one path leg",
			Gap:     true,
		}
	}
	return path, nil
}

func readPathStep(raw string) (PathStep, string, error) {
	if raw == "" {
		return PathStep{}, "", ParseFailure{
			Message: "JSON path leg is empty",
			Gap:     true,
		}
	}
	if raw[0] == '"' {
		return PathStep{}, "", ParseFailure{
			Message: "quoted JSON path keys are outside the documented MySQL subset",
			Gap:     true,
		}
	}
	if raw[0] == '*' {
		return PathStep{}, "", ParseFailure{
			Message: "JSON path wildcards are outside the documented MySQL subset",
			Gap:     true,
		}
	}
	end := len(raw)
	if next := strings.Index(raw, "->"); next >= 0 {
		end = next
	}
	leg := raw[:end]
	rest := raw[end:]
	if leg == "" {
		return PathStep{}, "", ParseFailure{
			Message: "JSON path leg is empty",
			Gap:     true,
		}
	}
	if isAllDigits(leg) {
		index, err := strconv.Atoi(leg)
		if err != nil {
			return PathStep{}, "", ParseFailure{
				Message: "JSON path array index is outside the documented MySQL subset",
				Gap:     true,
			}
		}
		return PathStep{IsIndex: true, Index: index}, rest, nil
	}
	if !isJSONPathIdentifier(leg) {
		return PathStep{}, "", ParseFailure{
			Message: fmt.Sprintf("JSON path key '%s' is outside the documented MySQL subset", leg),
			Gap:     true,
		}
	}
	return PathStep{Key: leg}, rest, nil
}

func isAllDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, ch := range value {
		if !unicode.IsDigit(ch) {
			return false
		}
	}
	return true
}

func isJSONPathIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for i, ch := range value {
		if i == 0 {
			if !(unicode.IsLetter(ch) || ch == '_') {
				return false
			}
			continue
		}
		if !(unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_') {
			return false
		}
	}
	return true
}
