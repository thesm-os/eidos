// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package leaderelection

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "leader-election"

// RoleCampaign is the callable standing for election.
const RoleCampaign = "campaign"

// RoleResign is the callable giving leadership up.
const RoleResign = "resign"

// RoleIsLeader is the callable reporting whether this instance holds
// leadership now.
const RoleIsLeader = "isleader"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleCampaign, RoleResign, RoleIsLeader}

// Contract returns the [shape.Contract] this package contributes.
// Campaign requires both Resign and IsLeader partners; the
// validator flags any Campaign declaration missing either side.
func Contract() shape.Contract {
	return shape.Contract{
		Name:     Name,
		Roles:    Roles,
		Required: map[string][]string{RoleCampaign: {RoleResign, RoleIsLeader}},
	}
}
