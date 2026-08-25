# Directive reference

Directives are the annotations you write in your own source to drive
generation. They are comments, so a codebase carrying them still
compiles, still vets, and still reads normally to anyone without the
tool.

```go
//+gen:enum
type Status int
```

## Syntax

```
//<prefix>:<name> [positional...] [key=value...]
//-<prefix>:<name>            ← negated form
```

- **`prefix`** defaults to `gen`, so directives read `+gen:`. A tool
  branding itself `testkit` configures `prefix: testkit` and its users
  write `//testkit:enum`. See
  [config.md](config.md#directives).
- **`name`** is a letter followed by letters, digits, `_` and `-`. It
  can never contain a dot, which is why `myco.repo` names a metadata
  key and can never name a directive.
- **Positional arguments** come before key/value pairs.
- **`-` instead of `+`** negates: `//-gen:enum` opts a declaration
  *out*. Some directives forbid negation, because deleting the line is
  the way to opt out and a negated form would mean nothing.

A comment that does not match this shape is an ordinary comment. That
is the failure mode to watch: **a misspelled directive is silent.**
`//+gen:enums` on a type produces no error, no warning, and no output.

## Placement

A directive attaches to the declaration it annotates, in either
position:

```go
//+gen:enum
type Status int              // leading

type Status int //+gen:enum  // trailing
```

Both work on structs, interfaces, methods, fields, functions, consts,
vars and type declarations. Order within a declaration follows the
source: leading directives are recorded before trailing ones.

The trailing position matters most in a `const` block, which is a
table where per-row metadata belongs on the row:

```go
const (
    StatusDraft  Status = iota + 1 //+gen:value draft
    StatusLive                     //+gen:value live
)
```

A **package-level** directive sits above the `package` clause and
applies to every entity in the package, with an entity-level directive
overriding it:

```go
//+gen:out generated/ pkg=generated
package shop
```

## Framework directives

Three names are the framework's own. A plugin cannot declare them —
the build fails the same way it would for two plugins colliding — and
they are accepted on every declaration a plugin routes.

### `+gen:out` — where output lands

```go
//+gen:out repo/user_repo.go
//+gen:out testing/                 ← directory only
//+gen:out mocks/ pkg=usermocks     ← directory + package name
//+gen:out plugin=mockgen mocks/    ← scoped to one plugin
//+gen:out tag=test testing/        ← scoped to one output
```

| Key | Effect |
|---|---|
| `pkg=` | Package name for the routed file. |
| `plugin=` | Apply to that plugin's output only. |
| `tag=` | Apply to that output tag only. |

A bare filename pins the filename. A value with a directory component
stacks onto the origin's directory, so `+gen:out test/x.go` beside
`foo/y.go` lands at `foo/test/x.go`.

**Scope it when the plugin emits more than one file.** An unscoped
filename override against a multi-output plugin would force every
output to share one name; the framework rejects that rather than
silently collapsing them.

**Two directives of equal scope conflict.** If both apply and neither
is more specific, the run reports an error rather than letting
declaration order decide.

### `+gen:value` — the rendered textual form

Pins the string a value renders as, overriding whatever a generator
would derive:

```go
const (
    StatusDraft Status = iota + 1 //+gen:value draft_state
)
```

### `+gen:meta` / `-gen:meta` — override a classification

Sets or clears a metadata key at an authority above anything an
annotator wrote:

```go
//+gen:meta shape.reader=true
//-gen:meta shape.writer
```

This is the escape hatch for a heuristic that got your code wrong. The
annotator cannot detect that it was overruled, and that is deliberate:
the user's statement is final.

## Plugin directives

Everything else is a plugin's own. `//+gen:enum`, `//+gen:mock`,
`//+gen:repo` — each is declared by exactly one plugin, which
documents its own positional arguments and keys.

The framework additionally accepts `out=`, `pkg=` and `tag=` on *any*
plugin directive, so routing can be expressed on the line that
triggers the generation rather than as a second annotation:

```go
//+gen:mock out=mocks/ pkg=usermocks
type UserRepo interface { /* … */ }
```

`out=` and `pkg=` written this way are unscoped, so a companion plugin
emitting against the same declaration inherits the routing. `tag=`
scopes to the declaring plugin, because tags are plugin-namespaced.

## A name nothing claims is an error

A directive whose name no registered schema declares is reported,
with a "did you mean?" where something registered is close enough to
be the typo:

```
user.go:12:1: pipeline: no directive named "buildr" is registered — did you mean "builder"?
```

The alternative is worse than it looks. `//+gen:buildr` parses, matches
no schema, stamps nothing and generates nothing — output identical to a
declaration carrying no directive at all. The line you wrote is the
only evidence you asked for anything, and the run's evidence is
silence.

Every directive in the loaded graph is checked, whichever language it
was read from and whether or not the frontend served it from cache.

### When you want the run to be narrower than the source

A project registering three plugins over a tree annotated for eight has
names nothing in that run claims, and every one is a line its author
meant. Turn the check off for those runs:

```go
pipeline.New().WithUnclaimedDirectives()
```

It is off by default, and the default is the point: you are trading the
typo back for the flexibility.

## Discovering what is available

`eidos plan` lists every registered plugin and its declared
directives, including the positional arguments and keys each accepts.
That is the authoritative list for your project — this page documents
the framework's own, and every other directive belongs to a plugin you
have registered.

## See also

- [CLI reference](cli.md)
- [Configuration reference](config.md)
- [plugin/routing.md](../plugin/routing.md) — the full precedence ladder
