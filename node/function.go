// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import (
	"go.thesmos.sh/eidos/core/kind"
)

// Function is a standalone (non-method) function declaration.
// Methods are modelled separately by [Method] so consumers can
// distinguish receiver-bound code without inspecting Owner.
type Function struct {
	BaseNode

	// Name is the function identifier.
	Name string `json:"name"`

	// Package is the source package path.
	Package string `json:"package,omitempty"`

	// Params are the function's positional parameters.
	Params []*Param `json:"params,omitempty"`

	// Returns are the function's return types in source order.
	Returns []*Return `json:"returns,omitempty"`

	// TypeParams are the function's generic type parameters.
	TypeParams []*TypeParam `json:"type_params,omitempty"`
}

// Kind returns [KindFunction].
func (*Function) Kind() kind.Kind { return KindFunction }

// QName returns the qualified name "Package.Name", or just "Name"
// when Package is empty.
func (f *Function) QName() string {
	if f.Package == "" {
		return f.Name
	}
	return f.Package + "." + f.Name
}

// ParamByName returns the parameter with the given name, or nil when
// no such parameter exists. An empty name argument always returns
// nil — anonymous parameters are not addressable by name and must
// be reached via [Function.ParamAt].
func (f *Function) ParamByName(name string) *Param {
	if name == "" {
		return nil
	}
	for _, p := range f.Params {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// ParamAt returns the parameter at the given positional index, or
// nil when i is out of range. Bounds-checked alternative to
// indexing [Function.Params] directly — useful for anonymous params.
func (f *Function) ParamAt(i int) *Param {
	if i < 0 || i >= len(f.Params) {
		return nil
	}
	return f.Params[i]
}

// ReturnAt returns the return type at the given positional index, or
// nil when i is out of range.
func (f *Function) ReturnAt(i int) *Return {
	if i < 0 || i >= len(f.Returns) {
		return nil
	}
	return f.Returns[i]
}

// ParamsWith returns parameters matching pred in declaration order.
func (f *Function) ParamsWith(pred func(*Param) bool) []*Param {
	out := make([]*Param, 0, len(f.Params))
	for _, p := range f.Params {
		if pred(p) {
			out = append(out, p)
		}
	}
	return out
}

// ReturnsWith returns the return slots matching pred in
// declaration order.
func (f *Function) ReturnsWith(pred func(*Return) bool) []*Return {
	out := make([]*Return, 0, len(f.Returns))
	for _, r := range f.Returns {
		if pred(r) {
			out = append(out, r)
		}
	}
	return out
}

// IsVariadic reports whether the function's last parameter is variadic.
func (f *Function) IsVariadic() bool {
	n := len(f.Params)
	return n > 0 && f.Params[n-1].Variadic
}

// IsGeneric reports whether the function declares generic type
// parameters.
func (f *Function) IsGeneric() bool { return len(f.TypeParams) > 0 }

// ParamCount returns the number of declared positional parameters.
func (f *Function) ParamCount() int { return len(f.Params) }

// ReturnCount returns the number of declared return values.
func (f *Function) ReturnCount() int { return len(f.Returns) }
