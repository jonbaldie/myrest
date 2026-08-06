# Process: maintain the PostgREST parity-target pin

**Ticket:** [Process to maintain the PostgREST v14.16 parity pin](https://github.com/jonbaldie/myrest/issues/47)  
**Closes:** deferred item 5 of [Parent spec: myrest PostgREST parity over MySQL 8](https://github.com/jonbaldie/myrest/issues/20)

A maintainer uses this process to watch upstream PostgREST, decide whether to move the **parity target**, and keep every labelled artifact honest. Accidental pin drift is out of scope for day-to-day edits: change the pin only by following the steps below.

## Canonical pin value

The current **parity target** pin lives in one place only:

**`CONTEXT.md` → Language → `Parity target`**

Read the version string from that entry. Every other mention in the repository (parent spec, ADRs, research notes, chapter prose) is a copy. When a copy disagrees with `CONTEXT.md`, `CONTEXT.md` wins until a pin-move pull request updates both.

Do not treat GitHub “latest”, the PostgREST docs “stable” brand alone, or an unpinned phrase as the pin.

## Watch upstream

Check these sources when you consider a move (and on a regular maintainer cadence):

| Source | What to read |
| --- | --- |
| [PostgREST releases](https://github.com/PostgREST/postgrest/releases) | New non-prerelease tags and release notes |
| Releases API `https://api.github.com/repos/PostgREST/postgrest/releases/latest` | Latest stable tag (same signal the original pin research used) |
| [PostgREST stable docs](https://docs.postgrest.org/en/stable/) | Documented HTTP behaviour for the release line |
| Upstream changelog / release notes for the candidate tag | Client-visible contract changes (routes, query ops, Prefer, errors, auth wire) |

A newer upstream release is a **watch signal**, not a pin move. Stay on the current pin until a recorded decision changes `CONTEXT.md`.

## Evidence a pin move needs

A pin-move pull request must include all of the following before merge:

1. **Candidate tag** — the exact PostgREST release (for example `v14.17`) proposed as the new **parity target**.
2. **Upstream evidence** — links to the release notes and the stable-docs pages that cover changed HTTP behaviour.
3. **Contract delta** — a short list of client-visible behaviour changes between the old pin and the candidate (including “none found” when the release is docs-only or ops-only).
4. **Label impact** — for each affected classified behaviour: keep, change **parity label**, or add a new labelled row under the **parity decision rule**.
5. **Re-verify plan** — which **normative scenarios** and **gap list** rows will be re-run or rewritten (see below).

Without that evidence, do not merge a pin change.

## What a pin move must re-label and re-verify

Update the canonical pin in `CONTEXT.md` first in the same pull request. Then bring every binding artifact in line with the new **parity target**.

### Must re-label (or explicitly confirm unchanged)

| Artifact | Duty |
| --- | --- |
| Parent spec ([issue #20](https://github.com/jonbaldie/myrest/issues/20) and any in-repo copy) | Replace the old pin name; adjust Solution / Implementation Decisions text that cites the pin |
| Capability-area chapter labels | Re-apply the **parity decision rule** to every behaviour the contract delta touches; leave untouched rows only when the delta does not affect them |
| **Normative scenarios** | Rewrite or re-assert each scenario whose labelled behaviour changed; keep stable ids (`area-nnn`) unless the behaviour itself is removed |
| **Gap list** | Rebuild as the derived roll-up of chapter **partial match** and **not supported** rows after label updates |
| ADRs that name the pin or freeze a boundary the delta changes | Supersede or amend so rationale matches the new pin |

### Must re-verify

| Check | Done when |
| --- | --- |
| Coverage by **parity label** | Every relabelled behaviour still has the scenario duty from ADR 0008 / the parent Verification chapter |
| Cross-area smoke set | Smoke scenarios still match the new pin’s claimed paths (or are updated in the same change) |
| Copies of the pin string | Repository search for the old pin version finds only historical research notes that mark the old value as superseded |

Research under `docs/research/` may keep the historical recommendation; add a one-line note that the live pin is `CONTEXT.md` when you touch those files for a move.

## Who decides and how it is recorded

| Role | Authority |
| --- | --- |
| Repository maintainers | Decide whether to move the pin |
| Anyone else | May open a draft pull request with the evidence above; maintainers approve |

**Record the decision** as a merged pull request that:

1. Updates the **Parity target** entry in `CONTEXT.md` to the new pin.
2. Completes the re-label and re-verify checklist in this document.
3. States in the pull-request body: old pin → new pin, links to upstream evidence, and a summary of label / scenario / **gap list** changes.
4. References this process and the parent spec deferred item it serves.

Optional: open or amend an ADR when the move changes a locked boundary already recorded under `docs/adr/`. The pull request remains the mandatory record; the ADR is for durable rationale.

## Accidental moves

Edits that change the pin string in `CONTEXT.md` without the evidence and checklist above are incomplete. Reviewers reject them. Mentions of a newer PostgREST release in issues or chat do not move the pin.
