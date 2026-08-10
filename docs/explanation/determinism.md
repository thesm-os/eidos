# Determinism and provenance

Every file the Go backend writes ends in a two-line footer:

```go
// <brand>: end of generated content.
// <brand>:provenance <sha256-of-body-bytes>
```

The hash is over the body bytes alone (header and footer excluded), so
the same emit graph produces an identical hash across runs regardless
of `Command` or `Plugins` header text. The header itself carries no
timestamp — two runs over the same input produce byte-identical files,
header and footer included. `Brand` defaults to `eidos`; library
embedders set `BackendContext.Brand` to re-brand their output.

The provenance trail is queryable in-process: every `meta.Entry`
carries `(setBy, authority, sourcePos)`; every slot contribution
carries the contributing plugin's name; every emit entity threads its
`OriginNode` back to the source-side IR. See
[`../backend/golang.md`](../backend/golang.md) for the full
envelope contract and the `imp` / `slot` / `provenance` template
funcmap entries.
