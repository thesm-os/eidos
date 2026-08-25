// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/node"
)

// validateDirectives walks every directive-bearing node in the
// converted package and runs [directive.Validate] against the
// pipeline's directive registry. Validation runs immediately after
// frontend parsing: schema-defined applies-to, requires, mutually-
// exclusive, required-keys, allowed-keys, and positional-args checks
// surface as positioned diagnostics on the run's diag sink.
//
// Unregistered directives parse cleanly and are not flagged here —
// the forward-compatible default lets in-development plugins
// experiment with new directive names without tripping the
// validator.
//
// validateDirectives is a no-op when no registry is configured on
// the [plugin.FrontendContext].
func (c *converter) validateDirectives() {
	if c.ctx.Registry == nil {
		return
	}
	ps := c.ctx.Diag.For(FrontendName)
	if len(c.out.DirectiveList) > 0 {
		directive.Validate(c.out.DirectiveList, node.KindPackage, c.ctx.Registry, ps)
	}
	for _, f := range c.out.Files {
		directive.Validate(f.DirectiveList, node.KindFile, c.ctx.Registry, ps)
		for _, imp := range f.Imports {
			directive.Validate(imp.DirectiveList, node.KindImport, c.ctx.Registry, ps)
		}
	}
	for _, s := range c.out.Structs {
		c.validateStructDirectives(s, ps)
	}
	for _, i := range c.out.Interfaces {
		c.validateInterfaceDirectives(i, ps)
	}
	for _, fn := range c.out.Functions {
		c.validateFunctionDirectives(fn, ps)
	}
	for _, v := range c.out.Variables {
		directive.Validate(v.DirectiveList, node.KindVariable, c.ctx.Registry, ps)
	}
	for _, cst := range c.out.Constants {
		directive.Validate(cst.DirectiveList, node.KindConstant, c.ctx.Registry, ps)
	}
	for _, e := range c.out.Enums {
		directive.Validate(e.DirectiveList, node.KindEnum, c.ctx.Registry, ps)
		for _, v := range e.Variants {
			directive.Validate(v.DirectiveList, node.KindEnumVariant, c.ctx.Registry, ps)
		}
	}
	for _, a := range c.out.Aliases {
		directive.Validate(a.DirectiveList, node.KindAlias, c.ctx.Registry, ps)
	}
}

// validateStructDirectives validates a struct's own directives and
// recursively validates every directive-bearing child node.
func (c *converter) validateStructDirectives(s *node.Struct, ps *diag.PluginSink) {
	directive.Validate(s.DirectiveList, node.KindStruct, c.ctx.Registry, ps)
	for _, f := range s.Fields {
		directive.Validate(f.DirectiveList, node.KindField, c.ctx.Registry, ps)
	}
	for _, e := range s.Embeds {
		directive.Validate(e.DirectiveList, node.KindEmbed, c.ctx.Registry, ps)
	}
	for _, m := range s.Methods {
		directive.Validate(m.DirectiveList, node.KindMethod, c.ctx.Registry, ps)
	}
}

// validateInterfaceDirectives validates an interface's own
// directives and every directive-bearing child node.
func (c *converter) validateInterfaceDirectives(i *node.Interface, ps *diag.PluginSink) {
	directive.Validate(i.DirectiveList, node.KindInterface, c.ctx.Registry, ps)
	for _, m := range i.Methods {
		directive.Validate(m.DirectiveList, node.KindMethod, c.ctx.Registry, ps)
	}
	for _, e := range i.Embeds {
		directive.Validate(e.DirectiveList, node.KindEmbed, c.ctx.Registry, ps)
	}
}

// validateFunctionDirectives validates a function's directives
// and any per-parameter directives the converter recorded. Per-
// param directives are extracted via [ast.NewCommentMap] from
// comments adjacent to each parameter in source (either before or
// after); see [converter.paramDocsAndDirectives].
func (c *converter) validateFunctionDirectives(fn *node.Function, ps *diag.PluginSink) {
	directive.Validate(fn.DirectiveList, node.KindFunction, c.ctx.Registry, ps)
	for _, p := range fn.Params {
		directive.Validate(p.DirectiveList, node.KindParam, c.ctx.Registry, ps)
	}
}
