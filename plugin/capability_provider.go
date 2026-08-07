// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin

import "go.thesmos.sh/eidos/priority"

// CapabilityProvider is the optional capability for plugins that
// participate in pipeline ordering. Plugins implementing this
// interface advertise their priority bucket and the capability
// names they Provide / Require; the pipeline groups by Priority and
// topo-sorts within each bucket using Provides / Requires.
//
// Capability names are package-namespaced strings (e.g.
// "shape.writer", "gen.repository"). Provides names declared by one
// plugin are matched against Requires names of other plugins in the
// SAME priority bucket; cross-bucket name resolution is intentionally
// not performed so plugin authors don't have to coordinate across
// teams to introduce a new bucket.
//
// Plugins that don't implement CapabilityProvider land in
// [priority.Default] and run in registration order within that
// bucket.
type CapabilityProvider interface {
	PrioritySource
	ProvidesSource
	RequiresSource
}

// PrioritySource is the bucket-declaring half of
// [CapabilityProvider], declared separately so the pipeline can
// distinguish a plugin that opted out of ordering from one that
// tried to opt in and missed a method. See [TemplateSource] for the
// reasoning; the failure here is a declared priority the pipeline
// silently ignores.
type PrioritySource interface {
	// Priority returns the bucket the plugin runs in.
	Priority() priority.Priority
}

// ProvidesSource is the capability-advertising half of
// [CapabilityProvider].
type ProvidesSource interface {
	// Provides returns the capability names this plugin produces
	// that other plugins may depend on. Returning nil indicates no
	// declared capabilities.
	Provides() []string
}

// RequiresSource is the capability-consuming half of
// [CapabilityProvider].
type RequiresSource interface {
	// Requires returns the capability names this plugin consumes.
	// The pipeline orders matching providers before this plugin
	// within the same bucket; unresolved names within a bucket emit
	// a verbose-mode info diagnostic and are otherwise ignored.
	Requires() []string
}
