# How to move generated output somewhere else

By default a generated file lands beside the source that caused it,
named `<source-basename><plugin-suffix>`. This page covers moving it.

Everything below is verified against a real run. If you want the
precedence rules rather than the recipes, read
[plugin/routing.md](../plugin/routing.md).

## Move one type's output to a subdirectory

Add `+gen:out` to the declaration:

```go
// +gen:repo
// +gen:out generated/ pkg=generated
type User struct { /* … */ }
```

```sh
eidos run ./...
```

`blog/user_repo.go` becomes `blog/generated/user_repo.go`, and its
package clause reads `package generated`.

Two details decide most of what happens here:

- **The path stacks onto the origin's directory.** `generated/` beside
  `blog/user.go` means `blog/generated/`, not a top-level
  `generated/`. The trailing slash makes it a directory rather than a
  filename.
- **`pkg=` sets the package clause.** Without it the package name is
  derived from the directory basename, which is usually what you want
  and occasionally is not.

## Move only one plugin's output

The example above moves **every** plugin's output for that
declaration. If `User` also had `+gen:builder` and an interface mock,
all three files move together:

```
blog/generated/user_builder.go
blog/generated/user_mock_test.go
blog/generated/user_repo.go
```

That is deliberate — a companion plugin emitting against the same
declaration inherits the routing, so you do not have to repeat it. To
move just one, name it:

```go
// +gen:out plugin=repogen generated/ pkg=generated
```

```
blog/generated/user_repo.go      ← moved
blog/user_builder.go             ← stayed
blog/user_mock_test.go           ← stayed
```

## Move one output of a multi-output plugin

A plugin emitting several files per source tags each one. Scope by
tag:

```go
// +gen:out tag=test testing/
```

Unlike `plugin=`, a `tag=` is namespaced to the declaring plugin,
because tag names are the plugin's own.

**A bare filename against a multi-output plugin is rejected.** It
would force every output to share one name; the run reports
`ErrUnscopedMultiOutputOverride` rather than silently collapsing them.
Use a directory, or scope with `tag=`.

## Move a whole package's output

Put the directive above the `package` clause:

```go
// +gen:out generated/ pkg=generated
package blog
```

It applies to every declaration in the package. An entity-level
directive overrides it, so the common shape is a package-wide default
with a handful of exceptions.

## Move everything, project-wide

Config, not directives:

```yaml
output:
  layout: centralised
  dir: internal/generated
  package: generated
```

`alongside-source` (the default) writes next to each source.
`centralised` collects everything under `dir`. See
[reference/config.md](../reference/config.md).

## Clean up what you moved

**`run` never deletes.** After moving output, the file at the old path
is still there:

```sh
eidos run ./...
ls blog/user_repo.go            # still present
eidos prune ./...               # deleted
```

`prune` is the command that removes what the current run no longer
claims. Run it after any routing change, or the old copy stays and
compiles alongside the new one — two definitions of the same type, and
a build error that names neither the directive nor the move.

## When the file is not where you expect

```sh
eidos explain blog.User
```

It reports which plugin put what where, and which precedence layer
supplied each part of the path. That answers "why here" faster than
re-reading the ladder.

## One case that cannot be routed

**Output carrying methods cannot leave its type's package.** Go
permits a method declaration only in the package declaring the
receiver, so an `+gen:out` moving such a file produces Go that names
an undefined type — and the error lands in your build, against a file
you did not write.

If a plugin's primary output carries methods, its documentation should
say so. Mock and stub outputs usually can move; repository
implementations that add methods to your type usually cannot.
