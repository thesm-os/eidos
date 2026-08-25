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
| [0002](0002-compile-plugins-as-static-imports.md) | Compile plugins into the binary as static imports | Accepted |
| [0003](0003-metadata-as-the-extension-mechanism.md) | Use metadata as the sole inter-plugin extension mechanism | Accepted |
| [0004](0004-compose-output-through-slots.md) | Compose generated output through slots, not inheritance | Accepted |
| [0005](0005-own-templates-per-backend-and-plugin.md) | Own templates per backend and per plugin | Accepted |
| [0006](0006-one-backend-per-pipeline-run.md) | Target one backend per pipeline run | Accepted |
| [0007](0007-group-language-support-by-language.md) | Group language support by language, one module apiece | Accepted |
| [0008](0008-map-typescript-interfaces-to-node-struct.md) | Map TypeScript interfaces to node.Struct | Proposed |

## Writing one

Numbers are monotonic, zero-padded to four, never reused and never
renumbered — a superseded ADR keeps its number and stays on disk.

The section that earns the document is **Alternatives Considered**. An
alternative described weakly convinces nobody and will be re-proposed
within the year; each one needs a fair description and the specific
reason it lost. "Too complex" is not a reason.

If there were genuinely no alternatives — a process bootstrap — say
so explicitly rather than leaving the section empty.

## Reading order

[ADR-0002](0002-compile-plugins-as-static-imports.md) through
[ADR-0006](0006-one-backend-per-pipeline-run.md) are the five choices
that shape everything else, and they build on each other in that order:
plugins are compiled in, so they exchange facts through typed metadata
rather than calls; facts are not output, so shared output composes
through slots; slots render through templates their owners control; and
rendering answers for one language at a time.
