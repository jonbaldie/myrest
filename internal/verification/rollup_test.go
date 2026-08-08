package verification_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonbaldie/myrest/internal/verification"
)

func TestDerivedGapListMatchesVerificationDoc(t *testing.T) {
	docsDir := filepath.Join("..", "..", "docs")
	chapters, err := verification.LoadChapters(docsDir)
	if err != nil {
		t.Fatalf("LoadChapters: %v", err)
	}
	gaps, err := verification.DeriveGapList(chapters)
	if err != nil {
		t.Fatalf("DeriveGapList: %v", err)
	}
	if len(gaps) == 0 {
		t.Fatal("derived gap list is empty")
	}

	index := verification.NormativeScenarios()
	if err := index.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := index.RequireSmokeSet(); err != nil {
		t.Fatal(err)
	}
	if err := verification.CheckCoverage(gaps, index); err != nil {
		t.Fatalf("gap coverage: %v", err)
	}
	fullMatches, err := verification.DeriveFullMatchBehaviours(chapters)
	if err != nil {
		t.Fatalf("derive full matches: %v", err)
	}
	if len(fullMatches) == 0 {
		t.Fatal("derived full match rows are empty")
	}
	if err := verification.CheckBehaviourCoverage(fullMatches, index); err != nil {
		t.Fatalf("full match coverage: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(docsDir, "verification.md"))
	if err != nil {
		t.Fatalf("read verification.md: %v", err)
	}
	gotGaps, err := verification.ExtractMarkedSection(
		string(body),
		"<!-- gap-list:begin -->",
		"<!-- gap-list:end -->",
	)
	if err != nil {
		t.Fatal(err)
	}
	wantGaps := verification.FormatGapListMarkdown(gaps)
	if gotGaps != wantGaps {
		t.Fatalf("verification.md gap list drifted from capability chapters\nwant:\n%s\ngot:\n%s", wantGaps, gotGaps)
	}

	gotIndex, err := verification.ExtractMarkedSection(
		string(body),
		"<!-- scenario-index:begin -->",
		"<!-- scenario-index:end -->",
	)
	if err != nil {
		t.Fatal(err)
	}
	wantIndex := verification.FormatScenarioIndexMarkdown(index)
	if gotIndex != wantIndex {
		t.Fatalf("verification.md scenario index drifted\nwant:\n%s\ngot:\n%s", wantIndex, gotIndex)
	}
}
