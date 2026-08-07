// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin

// Capability describes one multi-method optional capability as its
// constituent single-method interfaces, so a consumer can tell
// "declared none of them" from "declared some of them".
//
// The description lives beside the interfaces it describes on
// purpose. Held anywhere else it silently goes stale the day a
// method is added to one of them: the probe would still report
// "complete" for a plugin missing the new method, which is the same
// class of quiet drift the partial-implementation check exists to
// close.
//
// Only multi-method capabilities appear here. A single-method
// capability ([FilenameProvider], [Versioned], [NodesOnly], …)
// cannot be partially implemented — the assertion either succeeds
// or the author never opted in — so there is nothing to detect.
type Capability struct {
	// Name is the interface a plugin was reaching for, rendered
	// into diagnostics so an author can look it up.
	Name string

	// Methods are the capability's methods in declaration order, so
	// a diagnostic listing several reads in the order the godoc
	// does.
	Methods []CapabilityMethod
}

// CapabilityMethod is one method of a [Capability] plus the probe
// reporting whether a plugin declares it.
type CapabilityMethod struct {
	// Name is the bare method name, which is the only actionable
	// content in a partial-implementation diagnostic.
	Name string

	// Declared reports whether p declares this method, by asserting
	// the single-method interface that carries it.
	Declared func(p Plugin) bool
}

// Gap is one plugin's partial implementation of one capability.
type Gap struct {
	// Capability is the [Capability.Name] that was partly declared.
	Capability string

	// Declared and Missing partition the capability's methods.
	// Both are non-empty for every Gap — a plugin declaring all or
	// none produces no Gap at all.
	Declared []string
	Missing  []string
}

// Capabilities returns every multi-method optional capability the
// framework probes for partial implementation.
//
// Adding a capability here is what makes it checked; a new
// multi-method capability that skips this list reintroduces the
// silent-skip failure for itself alone.
func Capabilities() []Capability {
	return []Capability{
		{
			Name: "plugin.TemplateProvider",
			Methods: []CapabilityMethod{
				{"Templates", func(p Plugin) bool { _, ok := any(p).(TemplateSource); return ok }},
				{"TemplateFuncs", func(p Plugin) bool { _, ok := any(p).(TemplateFuncSource); return ok }},
				{"TemplateOverrides", func(p Plugin) bool { _, ok := any(p).(TemplateOverrideSource); return ok }},
			},
		},
		{
			Name: "plugin.CapabilityProvider",
			Methods: []CapabilityMethod{
				{"Priority", func(p Plugin) bool { _, ok := any(p).(PrioritySource); return ok }},
				{"Provides", func(p Plugin) bool { _, ok := any(p).(ProvidesSource); return ok }},
				{"Requires", func(p Plugin) bool { _, ok := any(p).(RequiresSource); return ok }},
			},
		},
	}
}

// Gaps returns one [Gap] per capability p implements partially, and
// nil for a plugin that implements each of them wholly or not at
// all.
//
// A Go interface assertion is all-or-nothing, so a plugin declaring
// two of three methods satisfies neither the capability nor any
// consumer's check for it — every consumer skips it in silence and
// the contribution vanishes. This is the single detection behind
// both [pipeline.Builder.Build]'s rejection and the conformance
// suite's check; holding it once is what keeps the two from
// disagreeing about what "partial" means.
//
// Declaring none of a capability's methods is the common case and
// produces no Gap: opting out of templates or of ordering is a
// legitimate choice the framework must not tax.
func Gaps(p Plugin) []Gap {
	var out []Gap
	for _, c := range Capabilities() {
		declared, missing := c.partition(p)
		if len(declared) == 0 || len(missing) == 0 {
			continue
		}
		out = append(out, Gap{Capability: c.Name, Declared: declared, Missing: missing})
	}
	return out
}

// partition splits c's methods into those p declares and those it
// does not.
func (c Capability) partition(p Plugin) (declared, missing []string) {
	for _, m := range c.Methods {
		if m.Declared(p) {
			declared = append(declared, m.Name)
			continue
		}
		missing = append(missing, m.Name)
	}
	return declared, missing
}
