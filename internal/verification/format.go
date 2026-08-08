package verification

import (
	"fmt"
	"strings"
)

// FormatScenarioIndexMarkdown writes the scenario index as a markdown table.
func FormatScenarioIndexMarkdown(index ScenarioIndex) string {
	var b strings.Builder
	b.WriteString("| Id | Capability area | Parity label | Outcome |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, scenario := range index {
		fmt.Fprintf(
			&b,
			"| `%s` | %s | %s | %s |\n",
			scenario.ID,
			scenario.Area,
			scenario.Label,
			scenario.Outcome,
		)
	}
	return b.String()
}

// ExtractMarkedSection returns the markdown between begin and end markers.
func ExtractMarkedSection(markdown, begin, end string) (string, error) {
	start := strings.Index(markdown, begin)
	if start < 0 {
		return "", fmt.Errorf("missing marker %q", begin)
	}
	start += len(begin)
	rest := markdown[start:]
	stop := strings.Index(rest, end)
	if stop < 0 {
		return "", fmt.Errorf("missing marker %q", end)
	}
	return strings.TrimSpace(rest[:stop]) + "\n", nil
}
