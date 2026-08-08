package verification

import (
	"fmt"
	"sort"
	"strings"
)

// ParityLabel is one label of the parity decision rule.
type ParityLabel string

const (
	FullMatch    ParityLabel = "full match"
	PartialMatch ParityLabel = "partial match"
	NotSupported ParityLabel = "not supported"
)

// GapRow is one partial match or not supported item from a capability chapter.
type GapRow struct {
	Chapter   string
	Item      string
	Label     ParityLabel
	Scenarios []string
}

// ParseGapRows reads the Gap list rows table from one capability chapter.
// Chapters stay the source of truth for labels; Verification only indexes them.
func ParseGapRows(markdown string) ([]GapRow, error) {
	section, ok := sectionAfterHeading(markdown, "## Gap list rows")
	if !ok {
		return nil, nil
	}
	table, ok := firstMarkdownTable(section)
	if !ok {
		return nil, nil
	}
	return rowsFromGapTable(table)
}

// ParseFullMatchRows reads the Full match rows table from one capability chapter.
func ParseFullMatchRows(markdown string) ([]Behaviour, error) {
	section, ok := sectionAfterHeading(markdown, "## Full match rows")
	if !ok {
		return nil, nil
	}
	table, ok := firstMarkdownTable(section)
	if !ok {
		return nil, nil
	}
	headers := splitRow(table[0])
	itemCol, labelCol, scenarioCol, err := gapColumns(headers)
	if err != nil {
		return nil, err
	}
	rows := make([]Behaviour, 0, len(table)-2)
	for _, line := range table[2:] {
		cells := splitRow(line)
		if len(cells) <= itemCol || len(cells) <= labelCol {
			continue
		}
		label := ParityLabel(strings.Trim(strings.TrimSpace(cells[labelCol]), "*"))
		if label != FullMatch {
			return nil, fmt.Errorf("full match row has parity label %q", cells[labelCol])
		}
		row := Behaviour{Item: strings.TrimSpace(cells[itemCol]), Label: label}
		if scenarioCol >= 0 && scenarioCol < len(cells) {
			row.Scenarios = splitScenarios(cells[scenarioCol])
		}
		if row.Item != "" {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func rowsFromGapTable(table []string) ([]GapRow, error) {
	headers := splitRow(table[0])
	itemCol, labelCol, scenarioCol, err := gapColumns(headers)
	if err != nil {
		return nil, err
	}
	rows := make([]GapRow, 0, len(table)-2)
	for _, line := range table[2:] {
		row, ok, err := gapRowFromCells(splitRow(line), itemCol, labelCol, scenarioCol)
		if err != nil {
			return nil, err
		}
		if ok {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func gapRowFromCells(cells []string, itemCol, labelCol, scenarioCol int) (GapRow, bool, error) {
	if len(cells) <= labelCol || len(cells) <= itemCol {
		return GapRow{}, false, nil
	}
	label, err := parseGapLabel(cells[labelCol])
	if err != nil {
		return GapRow{}, false, err
	}
	row := GapRow{
		Item:  strings.TrimSpace(cells[itemCol]),
		Label: label,
	}
	if scenarioCol >= 0 && scenarioCol < len(cells) {
		row.Scenarios = splitScenarios(cells[scenarioCol])
	}
	if row.Item == "" {
		return GapRow{}, false, nil
	}
	return row, true, nil
}

// DeriveGapList merges Gap list rows from every capability chapter.
// Sort order is chapter path, then item name, so the roll-up is stable.
func DeriveGapList(chapters map[string]string) ([]GapRow, error) {
	names := make([]string, 0, len(chapters))
	for name := range chapters {
		names = append(names, name)
	}
	sort.Strings(names)

	gaps := make([]GapRow, 0)
	for _, name := range names {
		rows, err := ParseGapRows(chapters[name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		for _, row := range rows {
			row.Chapter = name
			gaps = append(gaps, row)
		}
	}
	return gaps, nil
}

// DeriveFullMatchBehaviours merges Full match rows from capability chapters.
func DeriveFullMatchBehaviours(chapters map[string]string) ([]Behaviour, error) {
	names := make([]string, 0, len(chapters))
	for name := range chapters {
		names = append(names, name)
	}
	sort.Strings(names)

	behaviours := make([]Behaviour, 0)
	for _, name := range names {
		rows, err := ParseFullMatchRows(chapters[name])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		behaviours = append(behaviours, rows...)
	}
	return behaviours, nil
}

// FormatGapListMarkdown writes the derived gap list as a markdown table.
func FormatGapListMarkdown(gaps []GapRow) string {
	var b strings.Builder
	b.WriteString("| Chapter | Item | Parity label | Scenarios |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, gap := range gaps {
		fmt.Fprintf(
			&b,
			"| %s | %s | %s | %s |\n",
			gap.Chapter,
			gap.Item,
			gap.Label,
			strings.Join(gap.Scenarios, ", "),
		)
	}
	return b.String()
}

func parseGapLabel(raw string) (ParityLabel, error) {
	label := ParityLabel(strings.Trim(strings.TrimSpace(raw), "*"))
	switch label {
	case PartialMatch, NotSupported:
		return label, nil
	default:
		return "", fmt.Errorf("gap list label %q must be partial match or not supported", raw)
	}
}

func gapColumns(headers []string) (itemCol, labelCol, scenarioCol int, err error) {
	itemCol, labelCol, scenarioCol = -1, -1, -1
	for i, header := range headers {
		switch strings.ToLower(strings.TrimSpace(header)) {
		case "item":
			itemCol = i
		case "parity label", "label":
			labelCol = i
		case "scenarios", "scenario":
			scenarioCol = i
		}
	}
	if itemCol < 0 || labelCol < 0 {
		return 0, 0, 0, fmt.Errorf("gap list table needs Item and Parity label columns")
	}
	return itemCol, labelCol, scenarioCol, nil
}

func splitScenarios(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		id := strings.TrimSpace(part)
		if id == "" {
			continue
		}
		out = append(out, id)
	}
	return out
}

func sectionAfterHeading(markdown, heading string) (string, bool) {
	lines := strings.Split(markdown, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	lineCount := end
	for i := start; i < lineCount; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") {
			end = i
			break
		}
	}
	return strings.Join(lines[start:end], "\n"), true
}

func firstMarkdownTable(section string) ([]string, bool) {
	lines := strings.Split(section, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, false
	}
	table := make([]string, 0)
	for _, line := range lines[start:] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") {
			break
		}
		table = append(table, trimmed)
	}
	if len(table) < 2 {
		return nil, false
	}
	return table, true
}

func splitRow(line string) []string {
	trimmed := strings.Trim(strings.TrimSpace(line), "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, part := range parts {
		cells[i] = strings.TrimSpace(part)
	}
	return cells
}
