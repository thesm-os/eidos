# Contributing to eidos

Thanks for your interest. This guide covers what you need to know to land a
change.

## Quick start

```bash
make bootstrap   # install development tools
make check       # full pre-merge gate: tidy + lint + test + coverage
```

Required toolchain: **Go 1.26+**.

## Development loop

| Task | Command |
|---|---|
| Run tests with coverage | `make test` |
| Run tests under the race detector | `make test-race` |
| Run fuzz tests | `make test-fuzz` |
| Lint (Go + Markdown + license headers) | `make lint` |
| Format (Go + Markdown) | `make fmt` |
| Module tidy | `make tidy` |
| Full pre-merge gate | `make check` |

`make check` is what CI runs. If it passes locally, your PR will pass.

## Code standards

This project holds a production-grade bar from day one. The defaults below
aren't aspirational; they're enforced by lint and reviewed in every PR.

- **Docblocks on every exported identifier.** Not `// Foo is a foo` boilerplate —
  document what callers need to know about behaviour, edge cases, and intent.
- **One source file → one accompanying `<file>_test.go`.** Helpers, stubs, and
  fixtures live in a single `helpers_test.go` per package. Black-box testing
  via `<package>_test` packages.
- **Subtest pattern** — one top-level `TestXxx` per type or method; contract
  cases as `t.Run` subtests with `t.Parallel()`.
- **Modern Go 1.26+ idioms** — `slices`, `maps`, `iter`, `cmp`, `errors.Join` /
  `errors.Is`, range-over-int / range-over-func, `any`.
- **Stdlib reuse over reinvention.** No custom `itoa` helpers, no rolled-your-own
  `slices.Contains`.
- **Sentinel errors** declared and consumed via `errors.Is`. Never
  string-compared.
- **Determinism is a contract.** No `range map` exposed in any iteration path;
  ties broken alphabetically; outputs byte-identical across runs.
- **Don't validate scenarios that can't happen.** Trust internal code and
  framework guarantees. Validate at system boundaries (user input, external
  APIs, I/O) only. Unreachable defensive guards get deleted, not tested.

The `.golangci.yml` and `.markdownlint.yml` in the repo root encode the lint
contract; `make lint` runs both.

## Commit conventions

Commits follow [Conventional Commits](https://www.conventionalcommits.org/).
`ergon check commit-msg` enforces them against the `commit:` block in
`.ergon.yaml`; the commit-msg pre-commit hook checks each
commit locally.

Allowed types: `feat`, `fix`, `refactor`, `perf`, `docs`, `test`, `build`, `ci`,
`chore`, `revert`.

Subject line:

- ≤ 80 characters
- No trailing period
- Case-insensitive (proper nouns and acronyms welcome)

Body (when present):

- Leading blank line after the subject
- Lines ≤ 100 characters
- Explain *why*, not *what* — the diff already shows what

## Pull requests

1. Fork the repo and create a topic branch from `main`.
2. Make focused commits — one logical change per commit where possible.
3. Run `make check` locally before pushing.
4. Open a PR; fill in the PR template.
5. Address review comments by pushing additional commits rather than amending
   pushed history.
6. A maintainer will squash-merge once the checks pass and the review is clean.

## Reporting bugs and requesting features

Use the issue templates in `.github/ISSUE_TEMPLATE/`. Security issues go through
the channel in `SECURITY.md`, not the public tracker.
