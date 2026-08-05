// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import (
	"go.thesmos.sh/eidos/core/kind"
)

// Method is a function attached to a [Struct] or [Interface]. A Method
// on an Interface has a nil [Method.Receiver] (the receiver is
// implicit in the interface); a Method on a Struct carries the
// receiver type explicitly along with the source-level receiver
// variable name in [Method.ReceiverName].
type Method struct {
	BaseNode

	// Name is the method identifier.
	Name string `json:"name"`

	// Receiver is the receiver type of a struct method. nil for
	// methods declared inside an interface.
	Receiver *TypeRef `json:"receiver,omitempty"`

	// ReceiverName is the receiver variable name from source. For
	// `func (s *Repo) Get()` it is "s". Empty for methods declared
	// inside an interface (no receiver variable) and for receiver
	// declarations like `func (*Repo) Foo()` that use the blank
	// receiver form.
	ReceiverName string `json:"receiver_name,omitempty"`

	// Params are the method's positional parameters in source order.
	Params []*Param `json:"params,omitempty"`

	// Returns are the method's return types in source order.
	Returns []*Return `json:"returns,omitempty"`

	// TypeParams are the method's generic type parameters
	// (rarely used in Go; declared on the receiver type instead,
	// but accepted by the model).
	TypeParams []*TypeParam `json:"type_params,omitempty"`

	// Owner is the [Struct] or [Interface] that declares this
	// method. Populated by the constructing frontend.
	//
	// Owner is excluded from JSON encoding to break the host →
	// child cycle. Deserialized graphs re-wire Owner via
	// [RewireOwners].
	Owner Node `json:"-"`
}

// Kind returns [KindMethod].
func (*Method) Kind() kind.Kind { return KindMethod }

// ParamByName returns the parameter with the given name, or nil when
// no such parameter exists. An empty name argument always returns
// nil — anonymous parameters (common in interface method signatures)
// are not addressable by name and must be reached via [Method.ParamAt].
func (m *Method) ParamByName(name string) *Param {
	if name == "" {
		return nil
	}
	for _, p := range m.Params {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// ParamAt returns the parameter at the given positional index, or
// nil when i is out of range. Bounds-checked alternative to
// indexing [Method.Params] directly — useful for anonymous params.
func (m *Method) ParamAt(i int) *Param {
	if i < 0 || i >= len(m.Params) {
		return nil
	}
	return m.Params[i]
}

// ReturnAt returns the return type at the given positional index, or
// nil when i is out of range.
func (m *Method) ReturnAt(i int) *Return {
	if i < 0 || i >= len(m.Returns) {
		return nil
	}
	return m.Returns[i]
}

// ParamsWith returns parameters matching pred in declaration order.
func (m *Method) ParamsWith(pred func(*Param) bool) []*Param {
	out := make([]*Param, 0, len(m.Params))
	for _, p := range m.Params {
		if pred(p) {
			out = append(out, p)
		}
	}
	return out
}

// ReturnsWith returns the return slots matching pred in
// declaration order.
func (m *Method) ReturnsWith(pred func(*Return) bool) []*Return {
	out := make([]*Return, 0, len(m.Returns))
	for _, r := range m.Returns {
		if pred(r) {
			out = append(out, r)
		}
	}
	return out
}

// HasReceiver reports whether the method has an explicit receiver.
// Methods declared inside an interface return false; methods on a
// struct return true.
func (m *Method) HasReceiver() bool { return m.Receiver != nil }

// IsVariadic reports whether the method's last parameter is variadic.
func (m *Method) IsVariadic() bool {
	n := len(m.Params)
	return n > 0 && m.Params[n-1].Variadic
}

// IsGeneric reports whether the method declares its own generic type
// parameters (rare in Go; usually generic params live on the
// receiver type).
func (m *Method) IsGeneric() bool { return len(m.TypeParams) > 0 }

// ParamCount returns the number of declared positional parameters.
func (m *Method) ParamCount() int { return len(m.Params) }

// ReturnCount returns the number of declared return values.
func (m *Method) ReturnCount() int { return len(m.Returns) }
