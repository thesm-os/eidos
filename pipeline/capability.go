// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"fmt"
	"strings"

	"go.thesmos.sh/eidos/plugin"
)

// validatePartialCapabilities returns one error per plugin that
// implements part of a multi-method optional capability.
//
// The detection itself lives in [plugin.Gaps], beside the
// interfaces it describes — a copy here would go stale the day a
// method is added to one of them, and would then disagree with the
// conformance suite about what "partial" means.
//
// Only the middle case is an error: an author who reached for the
// capability and missed. Declaring all of a capability's methods is
// the working case, declaring none is opting out, and both pass
// untouched.
func (b *Builder) validatePartialCapabilities() []error {
	var errs []error
	for _, p := range b.allPlugins() {
		for _, gap := range plugin.Gaps(p) {
			errs = append(errs, fmt.Errorf(
				"%w: %s declares %s but not %s, so it does not satisfy %s "+
					"and the pipeline ignores the declaration entirely",
				ErrPartialCapability,
				p.Name(),
				strings.Join(gap.Declared, " + "),
				strings.Join(gap.Missing, " + "),
				gap.Capability,
			))
		}
	}
	return errs
}

// allPlugins returns every registered plugin once, whatever roles
// it holds.
//
// Deduplicated by name because a dual-role plugin is registered
// under each role it implements — the composition
// [Builder.validateNoDuplicateNames] deliberately permits — and
// reporting its one missing method once per role would read as
// several distinct faults.
func (b *Builder) allPlugins() []plugin.Plugin {
	seen := map[string]struct{}{}
	var out []plugin.Plugin
	for _, ps := range [][]plugin.Plugin{
		widenRoles(b.frontends),
		widenRoles(b.annotators),
		widenRoles(b.generators),
		widenRoles(b.backends),
	} {
		for _, p := range ps {
			name := p.Name()
			if _, dup := seen[name]; dup {
				continue
			}
			seen[name] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}
