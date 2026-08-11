// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package scheduled recognises the scheduled mixin — the assertion
// that a task registered for a future time has run once the clock
// passes it.
//
// The `schedule` param names the registration and `fired` the
// accessor reporting how many tasks have run, which is what a law
// compares before and after advancing the clock. Neither is derivable:
// a registration and a counter are ordinary signatures that say
// nothing about being a pair.
//
// The recognised directive is:
//
//	//+gen:mixin scheduled schedule=After fired=FiredCount
package scheduled
