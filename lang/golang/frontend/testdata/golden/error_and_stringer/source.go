// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises the go.isError and go.isStringer
// stamping paths. Run returns the predeclared error type; Label
// satisfies fmt.Stringer via a String() method, so any reference
// to *Label (and Label) gets stamped accordingly.
package fixture

// Run returns an error to drive the go.isError stamping path. The
// returned reference's type-ref meta must carry the flag.
func Run() error { return nil }

// Label is a small type that implements fmt.Stringer. Any type-ref
// pointing at Label carries go.isStringer because the converter
// can prove the method set satisfies the predeclared Stringer
// interface.
type Label struct {
	Text string
}

// String renders Label.Text. The exact signature `String() string`
// is what the converter probes for when stamping go.isStringer.
func (l Label) String() string { return l.Text }

// Render returns a Label so the function's return type-ref carries
// the go.isStringer marker — the value of having a dedicated
// fixture is in pinning that downstream wiring.
func Render() Label { return Label{} }
