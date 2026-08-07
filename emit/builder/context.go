// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder

import (
	"errors"
	"fmt"

	"go.thesmos.sh/eidos/emit"
)

// ErrNilHost is returned by the cross-cutting slot helpers when the
// host emit value passed in is nil. Callers compare with [errors.Is].
var ErrNilHost = errors.New("builder: nil host")

// ErrUnsupportedHost is returned by the cross-cutting slot helpers
// when the supplied host doesn't expose the requested slot (e.g.
// AppendField on an *emit.Interface). Callers compare with
// [errors.Is]; the wrapped message names the offending kind.
var ErrUnsupportedHost = errors.New("builder: unsupported host kind")

// Context binds the plugin-author identity and the default
// [emit.Target] for a sequence of builder calls. A plugin creates
// one at the top of its Generate or Annotate method and threads it
// through every decl built and every slot append performed during
// that pass:
//
//	c := builder.For(p.Name())
//	pkg := c.Package("users", "example.com/users").
//	    Struct("Repo", func(s *builder.StructBuilder) { … }).
//	    Build()
//
// Decls created through c inherit c.Target() automatically; slot
// appends performed through c stamp [emit.Provenance.SetBy] with
// c.SetBy(). Context is concurrency-safe for reads — concurrent
// builder calls share the bound identity and target without locking.
// Mutation helpers ([Context.WithTarget]) return a fresh Context
// rather than mutating the receiver.
type Context struct {
	setBy  string
	target emit.Target
}

// For returns a Context bound to the supplied plugin identifier
// (typically the plugin's [plugin.Plugin.Name] return value). The
// default [emit.Target] is the zero value — plugins should leave
// placement to the pipeline's Layout phase, which composes the
// resolved Target from the framework default + per-decl origin +
// per-source `+gen:out` directives + project / CLI overrides.
//
// A plugin emitting into multiple files calls [Context.WithTarget]
// to spawn additional Contexts per target rather than mutating the
// receiver.
func For(setBy string) *Context {
	return &Context{setBy: setBy}
}

// SetBy returns the plugin identifier this Context stamps onto every
// cross-cutting slot contribution.
func (c *Context) SetBy() string { return c.setBy }

// Target returns the default [emit.Target] every decl built through
// this Context inherits.
func (c *Context) Target() emit.Target { return c.target }

// WithTarget returns a copy of c bound to a different default Target.
// Use this when one plugin emits into multiple files within a single
// Generate / Annotate pass; the original Context remains usable.
func (c *Context) WithTarget(t emit.Target) *Context {
	clone := *c
	clone.target = t
	return &clone
}

// Provenance returns an [emit.Provenance] stamped with this Context's
// plugin name. Optional id is recorded in [emit.Provenance.ID] so
// later contributions can position themselves relative to this one
// via [emit.Slot.InsertBefore] / [emit.Slot.InsertAfter]; pass no
// arguments to leave ID empty.
//
// The helper is exposed so plugin authors that bypass the
// slot-append helpers (because they want positional inserts, say)
// can still get correctly-stamped Provenance without re-declaring
// the plugin name.
func (c *Context) Provenance(id ...string) emit.Provenance {
	p := emit.Provenance{SetBy: c.setBy}
	if len(id) > 0 {
		p.ID = id[0]
	}
	return p
}

// nilCheck returns ErrNilHost wrapped with role when host is nil.
// Centralised so every slot helper produces the same diagnostic
// shape.
func nilCheck(host emit.Node, role string) error {
	if host == nil {
		return fmt.Errorf("%w: %s host is nil", ErrNilHost, role)
	}
	return nil
}
