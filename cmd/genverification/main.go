package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jonbaldie/myrest/internal/verification"
)

func main() {
	docsDir := "docs"
	if len(os.Args) > 1 {
		docsDir = os.Args[1]
	}
	chapters, err := verification.LoadChapters(docsDir)
	if err != nil {
		log.Fatalf("genverification: %v", err)
	}
	gaps, err := verification.DeriveGapList(chapters)
	if err != nil {
		log.Fatalf("genverification: %v", err)
	}
	index := verification.NormativeScenarios()
	gapTable := verification.FormatGapListMarkdown(gaps)
	indexTable := verification.FormatScenarioIndexMarkdown(index)
	doc := `# Verification: scenario index, gap list, and smoke set

**Ticket:** [Verification roll-up: scenario index, gap list, and smoke set](https://github.com/jonbaldie/myrest/issues/48)
**Closes:** [Parent spec: myrest PostgREST parity over MySQL 8](https://github.com/jonbaldie/myrest/issues/20) once Implementation done holds on the HTTP seam
**Parity target:** PostgREST v14.16 (see ` + "`CONTEXT.md`" + `)

A client author and an operator can read one accurate statement of what myrest
supports, and one command proves it. This page is the Verification roll-up:
method, done rules, the scenario index, the derived **gap list**, and the fixed
cross-area smoke set. Capability-area chapters stay the source of truth for
**parity labels**. See [ADR 0008](adr/0008-normative-verification-approach.md).

## Method

- Prove parity at the HTTP API boundary with rewritten **normative scenarios**.
- Coverage follows the **parity label**:
  - **full match** — one success path. When the labelled item is itself a client-visible error, one refuse path instead.
  - **partial match** — one in-subset success and one outside-subset refuse
  - **not supported** — one stable refuse and no success claim for that behaviour. Refuse is an error envelope, or a stable non-offer when the chapter names no client input path.
- Scenario bodies live in capability-area chapters and in the HTTP acceptance
  tests under ` + "`test/acceptance`" + `. This page indexes them by stable ` + "`area-nnn`" + ` id.

## Done rules

- **Capability area done:** every labelled behaviour in that area has its
  required normative scenario(s).
- **Verification done:** the smoke set passes at the HTTP seam; the gap list
  matches a fresh derivation from chapter Gap list rows; every labelled
  behaviour meets its coverage duty.
- **Implementation done (parent #20):** normative scenarios pass at the HTTP
  seam (` + "`make scenarios`" + `); deferred Representation, OpenAPI, CORS,
  transaction, gap-code, and related edges are labelled under the parity
  decision rule or refused in the derived gap list.

## Cross-area smoke set

| Id | Intent |
| --- | --- |
| ` + "`smoke-001`" + ` | Anonymous database role ordinary read succeeds |
| ` + "`smoke-002`" + ` | JWT → database role ordinary read succeeds |
| ` + "`smoke-003`" + ` | Write with Prefer return=representation succeeds with a representation body |
| ` + "`smoke-004`" + ` | POST /rpc/... succeeds |
| ` + "`smoke-005`" + ` | Embed read on a cache relationship succeeds |
| ` + "`smoke-006`" + ` | Deliberate not-supported path (FTS) stable refuse |

## Run the whole scenario set

` + "`make scenarios`" + ` runs the normative scenario packages: ` + "`./cmd/myrest`" + `,
` + "`./internal/httpapi`" + `, ` + "`./test/acceptance`" + `, and ` + "`./internal/verification`" + `.
` + "`make test`" + ` runs the full suite.

## Scenario index

<!-- scenario-index:begin -->
` + indexTable + `<!-- scenario-index:end -->

## Gap list (derived)

This table is derived from the ` + "`## Gap list rows`" + ` section of each capability-area chapter.
Do not edit it by hand. When a chapter label changes, update that chapter and
re-derive; ` + "`go test ./internal/verification`" + ` fails when this table drifts.

<!-- gap-list:begin -->
` + gapTable + `<!-- gap-list:end -->
`
	if err := os.WriteFile(filepath.Join(docsDir, "verification.md"), []byte(doc), 0o644); err != nil {
		log.Fatalf("genverification: %v", err)
	}
	fmt.Printf("wrote verification.md with %d scenarios and %d gap rows\n", len(index), len(gaps))
}
