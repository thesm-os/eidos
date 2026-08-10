# Architecture Decision Records

One decision per file, recorded so that the reasoning survives the
people who made it.

An Accepted ADR is never edited in place. When a decision changes, a
new ADR supersedes it and the old one's `status` and `superseded-by`
frontmatter are updated — its argument stays as written, because it is
evidence of what was true at the time.

| ADR | Title | Status |
|---|---|---|
| [0001](0001-use-adrs-for-architecture-decisions.md) | Use ADRs for architecture decisions | Accepted |

## Writing one

Numbers are monotonic, zero-padded to four, never reused and never
renumbered — a superseded ADR keeps its number and stays on disk.

The section that earns the document is **Alternatives Considered**. An
alternative described weakly convinces nobody and will be re-proposed
within the year; each one needs a fair description and the specific
reason it lost. "Too complex" is not a reason.

If there were genuinely no alternatives — a process bootstrap — say
so explicitly rather than leaving the section empty.

## Not yet recorded

`README.md`'s `## Design decisions` section states five architectural
choices without the alternatives they beat: static plugin imports,
metadata as the extension mechanism, slot composition, per-backend
template ownership, and one backend per run. Each is ADR-shaped and
none is written up. See [ADR-0001](0001-use-adrs-for-architecture-decisions.md).
