# shape

`plugins/annotator/shape` is the eidos annotator that classifies
callables (free functions and methods) into named **shapes** and
records orthogonal **contract** memberships and **mixin**
attachments. Downstream generators consume the stamps without
re-deriving the analysis.

## Three orthogonal axes

Every callable carries up to three independent facts on its
[meta.Bag]:

| Axis | Meta key root | Source | Example |
|---|---|---|---|
| **Structural shape** | `shape` | Signature-driven detector | `shape = "writer"` |
| **Contract membership** | `shape.contracts`, `shape.contract.<name>.*` | `+gen:contract` directive | callable participates in protocol `tx` as role `commit` |
| **Mixin attachment** | `shape.mixins`, `shape.mixin.<name>.*` | `+gen:mixin` directive | callable is `atomic`, `idempotent`, … |

The three axes never collide — they live under disjoint key
prefixes. A `Commit` callable can simultaneously carry
`shape = "writer"`, `shape.contract.tx.role = "commit"`, and
`shape.mixin.atomic` without overwriting any of them.

## Registering the plugin

The umbrella plugin is constructed once per pipeline. Compose
the detectors, contracts, and mixins your pipeline recognises:

```go
import (
    "go.thesmos.sh/eidos/pipeline"
    "go.thesmos.sh/eidos/plugins/annotator/shape"
    "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/reader"
    "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/writer"
    "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/persister"
    "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/atomic"
    "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/idempotent"
)

s := shape.New().
    Detectors(reader.Detector(), writer.Detector()).
    Contracts(persister.Contract()).
    Mixins(atomic.Mixin(), idempotent.Mixin())

pipe, err := pipeline.New().
    WithFrontend(...).
    WithAnnotator(s).
    WithAnnotator(s.Resolver()).
    WithAnnotator(s.Validator()).
    WithGenerator(...).
    Build()
```

All three instances must be registered. Registering `s` alone
stamps shapes but leaves partner names unqualified and every
[`Contract.Required`] declaration and [`Mixin.Validate`] hook
unenforced — a silent loss of diagnostics, not a build failure.

Three plugin instances run in priority order:

1. **`s`** itself — runs at `AnnotatorShape` priority. Owns the
   `+gen:shape`, `+gen:contract`, and `+gen:mixin` directive
   schemas. Dispatches detectors and stamps directive-driven
   contract / mixin meta.
2. **`s.Resolver()`** — runs at `AnnotatorRefinement`. Rewrites
   raw partner names (`reader=GetByID`) into qualified names
   (`x.Repo.GetByID`) and back-stamps contract membership on
   resolved partners. Also rewrites mixin sibling-param values
   (`+gen:mixin readafterwrite write=Save`).
3. **`s.Validator()`** — runs at `AnnotatorValidation`. Enforces
   each contract's `Required` partners and invokes
   `Contract.Validate` / `Mixin.Validate` hooks against the
   resolved member sets.

`Detectors`, `Contracts`, and `Mixins` accept any number of
arguments and may be called multiple times; detectors are sorted
by `Detector.Priority` descending so registration order does
not affect dispatch.

## Reading shape meta in your generator

The library's public API gives you typed accessors for every
stamp. A consuming generator reads:

```go
import "go.thesmos.sh/eidos/plugins/annotator/shape"

func (g *Plugin) Generate(ctx *sdk.GeneratorContext) error {
    for _, m := range ctx.Reader.Methods().Slice() {
        bag := m.Meta()

        // Structural shape — branch on the canonical name.
        switch shape.Get(bag) {
        case "reader":
            g.emitReaderHandler(m)
        case "writer":
            g.emitWriterHandler(m)
        }

        // The universal triple — qualified type strings.
        keyType, _   := shape.MetaKeyType.Get(bag)
        valueType, _ := shape.MetaValueType.Get(bag)

        // Contract memberships — iterate the list.
        for _, contractName := range shape.Contracts(bag) {
            role, _ := shape.ContractRoleKey(contractName).Get(bag)
            partner, _ := shape.ContractPartnerKey(contractName, "reader").Get(bag)
            // `partner` is the qualified name of the paired
            // callable (resolved by Resolver).
        }

        // Mixin attachments — iterate the list.
        for _, mixinName := range shape.Mixins(bag) {
            // Per-mixin param values live under
            // shape.mixin.<name>.<param>.
            if param, ok := shape.MixinParamKey(mixinName, "duration").Get(bag); ok {
                _ = param
            }
        }
    }
    return nil
}
```

Per-shape sub-packages may stamp **extra** keys for richer
classifications. Each sub-package exports the keys it owns:

```go
import "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multireader"
import "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/lookup"
import "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/streamreader"

valueTypes, _ := multireader.ValueTypes.Get(bag) // []string
metaType, _   := lookup.MetaType.Get(bag)        // string
variant, _    := streamreader.Variant.Get(bag)   // "seq" | "seq2"
```

## Adding your own shape

A new shape is a tiny sub-package shipping a `Detector()`
constructor:

```go
package mything

import (
    "go.thesmos.sh/eidos/lang/golang"
    "go.thesmos.sh/eidos/node"
    "go.thesmos.sh/eidos/plugins/annotator/shape"
)

const Name = "mything"

func Detector() shape.Detector {
    return shape.Detector{
        Name:     Name,
        Priority: 600,
        Detect: map[string]shape.DetectFunc{
            golang.Language: detectGolang,
        },
    }
}

func detectGolang(n node.Node) (shape.Match, bool) {
    params, returns := golang.Callable(n)
    if !matches(params, returns) {
        return shape.Match{}, false
    }
    return shape.Match{
        KeyType:   golang.QName(params[1].Type),
        ValueType: golang.QName(returns[0]),
    }, true
}
```

The query vocabulary is the language package's. `lang/golang`
answers every signature question a Go detector asks —
`Callable`, `StripContext`, `HasError`, `StripErrorTypes`,
`SliceElem`, `IsBool`, `QName` — and its predicates answer as the
union of the frontend's `go.*` stamp and the Go spelling, which a
copy written against the spelling alone cannot do.

The umbrella plugin handles every other concern — directive
override (`+gen:shape mything`), already-stamped guard, meta
writes. The sub-package owns only the detection logic and the
canonical name.

## Adding your own contract

A contract is a named multi-callable protocol with a role
vocabulary. Per-contract sub-packages export a `Contract()`
constructor:

```go
package mycontract

import "go.thesmos.sh/eidos/plugins/annotator/shape"

const Name = "mycontract"

var Roles = []string{"primary", "secondary"}

func Contract() shape.Contract {
    return shape.Contract{
        Name:     Name,
        Roles:    Roles,
        Required: map[string][]string{"primary": {"secondary"}},
        Validate: validate,
    }
}

func validate(members map[string][]shape.ContractMember) []shape.ContractViolation {
    // Walk members by role; emit violations for invariant
    // breaches. Each member carries Host (the callable) and
    // Partners (role -> qname).
    return nil
}
```

For opaque KV params that aren't sibling callables (e.g.
`version=Version` naming a field), declare `Contract.Params`.
The umbrella stamps those under `shape.contract.<name>.param.<key>`
and the resolver skips them.

## Adding your own mixin

A mixin is a per-callable behavioural decoration. Per-mixin
sub-packages export a `Mixin()` constructor:

```go
package mymixin

import "go.thesmos.sh/eidos/plugins/annotator/shape"

const Name = "mymixin"

var Params = []shape.Param{
    {Key: "limit", Kind: shape.KindOpaque},
}

func Mixin() shape.Mixin {
    return shape.Mixin{Name: Name, Params: Params}
}
```

Each param declares what its value names, and the resolver treats
it accordingly:

| Kind | The value names | Resolved through |
|---|---|---|
| `KindOpaque` | nothing to look up — a literal, a quantity, a field name | left verbatim |
| `KindCallable` | a sibling callable | the host's own scope |
| `KindVar` | a package-level var, typically a sentinel | the package |
| `KindMember` | a method on the type a role's callable answers | that type's declaration |

`KindOpaque` is the zero value, so `{Key: "limit"}` reads as a
literal. Anything else is rewritten to a qualified name, and a value
naming nothing in its scope is reported against the directive.

## Sub-package layout

```
plugins/annotator/shape/
  shape.go            — umbrella Plugin + Detector / Match / DetectFunc
  contract.go         — Contract / ContractMember / ContractViolation
  mixin.go            — Mixin / MixinAttachment / MixinViolation
  resolver.go         — refinement-bucket sibling-name rewriter
  validator.go        — validation-bucket invariant checker
  meta.go             — universal MetaShape / MetaKeyType / MetaValueType
  param.go            — Param / ParamKind / role scoping

  detectors/<name>/   — per-shape signature detectors (23 today)
  contracts/<name>/   — per-contract protocol declarations (26 today)
  mixins/<name>/      — per-mixin behavioural decorations (60 today)
  ids/                — every catalog name and param key as a constant
```

Signature primitives are not here. A Go detector composes its
query from `lang/golang`, which every Go-speaking part of a
pipeline already shares.

The umbrella library has no opinion on the shape, contract, or
mixin vocabulary — the vocabulary is the union of registered
sub-packages.
