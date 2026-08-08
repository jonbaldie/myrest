package verification_test

import (
	"strings"
	"testing"

	"github.com/jonbaldie/myrest/internal/verification"
)

func TestParseGapRowsReadsItemLabelAndScenarios(t *testing.T) {
	markdown := trimLines(`
		# Auth

		## Gap list rows

		| Item | Parity label | Scenarios |
		| --- | --- | --- |
		| Role impersonation identity | partial match | auth-004 |
		| Postgres row-level security | not supported | auth-005 |
	`)

	rows, err := verification.ParseGapRows(markdown)
	if err != nil {
		t.Fatalf("ParseGapRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].Item != "Role impersonation identity" {
		t.Errorf("rows[0].Item = %q", rows[0].Item)
	}
	if rows[0].Label != verification.PartialMatch {
		t.Errorf("rows[0].Label = %q, want partial match", rows[0].Label)
	}
	if got := strings.Join(rows[0].Scenarios, ","); got != "auth-004" {
		t.Errorf("rows[0].Scenarios = %q", got)
	}
	if rows[1].Label != verification.NotSupported {
		t.Errorf("rows[1].Label = %q, want not supported", rows[1].Label)
	}
}

func TestDeriveGapListMergesChapterRowsWithoutDrift(t *testing.T) {
	chapters := map[string]string{
		"auth.md": trimLines(`
			## Gap list rows

			| Item | Parity label | Scenarios |
			| --- | --- | --- |
			| Role impersonation identity | partial match | auth-004 |
			| Postgres row-level security | not supported | auth-005 |
		`),
		"read-parity-boundaries.md": trimLines(`
			## Gap list rows

			| Item | Parity label | Scenarios |
			| --- | --- | --- |
			| Text-case subset | partial match | read-003, read-004 |
			| FTS operators | not supported | read-007 |
		`),
	}

	gaps, err := verification.DeriveGapList(chapters)
	if err != nil {
		t.Fatalf("DeriveGapList: %v", err)
	}
	if len(gaps) != 4 {
		t.Fatalf("len(gaps) = %d, want 4", len(gaps))
	}
	if gaps[0].Chapter != "auth.md" || gaps[0].Item != "Role impersonation identity" {
		t.Errorf("first gap = %+v", gaps[0])
	}
	if gaps[2].Chapter != "read-parity-boundaries.md" {
		t.Errorf("third gap chapter = %q", gaps[2].Chapter)
	}
}

func TestParseGapRowsIgnoresFullMatchAndEmptySections(t *testing.T) {
	markdown := trimLines(`
		## Gap list rows

		This area adds no row to the gap list.
	`)
	rows, err := verification.ParseGapRows(markdown)
	if err != nil {
		t.Fatalf("ParseGapRows: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("len(rows) = %d, want 0", len(rows))
	}
}

func trimLines(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimPrefix(line, "\t")
	}
	return strings.Join(lines, "\n") + "\n"
}
