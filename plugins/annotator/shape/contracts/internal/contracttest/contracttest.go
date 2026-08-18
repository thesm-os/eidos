// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package contracttest provides shared assertions for the
// per-contract sub-packages.
//
// Two surfaces:
//
//   - [AssertIdentity] for the canonical `TestContract_Identity`
//     test that pins a contract's Name + Roles against the
//     package constants.
//   - [RunPipeline] + [AssertRole] / [AssertPartner] /
//     [AssertContainsDiag] for integration tests that exercise
//     the umbrella → resolver → validator round-trip on a
//     specific catalog contract.
//
// Internal — importable only by [shape/contracts/...] children;
// not part of the shape library's public API.
package contracttest

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// frontendMarker mirrors the umbrella plugin's package-private
// frontend lookup key so fixtures stamp the marker on test
// packages without re-implementing the lookup.
//
//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = sdk.EnsureKey("frontend", sdk.StringParser)

// AssertIdentity fails the test when c does not match the
// expected name + roles. Use as the canonical body of every
// per-contract `TestContract_Identity` test:
//
//	contracttest.AssertIdentity(t, watcher.Contract(),
//	    watcher.Name, watcher.Roles)
func AssertIdentity(t *testing.T, c shape.Contract, wantName string, wantRoles []string) {
	t.Helper()
	if c.Name != wantName {
		t.Fatalf("Contract().Name = %q, want %q", c.Name, wantName)
	}
	if !reflect.DeepEqual(c.Roles, wantRoles) {
		t.Fatalf("Contract().Roles = %v, want %v", c.Roles, wantRoles)
	}
	assertParamRoles(t, c)
}

// assertParamRoles fails when a [shape.Param.Role] names a role the
// contract does not declare.
//
// A mis-scoped role is silent in production: the param simply never
// applies, so its key routes as a partner reference and the directive
// that meant to set it reports nothing. There is no registration-time
// seam to catch it — [shape.Plugin.Contracts] is a map write — so the
// check lives in the identity test every contract package already
// calls, which puts the contract author's typo in the contract
// author's own test.
func assertParamRoles(t *testing.T, c shape.Contract) {
	t.Helper()
	for _, p := range c.Params {
		if p.Role == "" || slices.Contains(c.Roles, p.Role) {
			continue
		}
		t.Errorf("Contract().Params[%q].Role = %q, which is not in Roles %v",
			p.Key, p.Role, c.Roles)
	}
}

// HostDirective builds a `+gen:contract <name> role=<role>
// [<partner>=<sibling>]...` directive for test fixtures. The KV
// map's `role` key is set unconditionally; supply partner roles
// keyed by role name.
func HostDirective(contractName, role string, partners map[string]string) *sdk.Directive {
	kv := map[string]string{"role": role}
	maps.Copy(kv, partners)
	return &sdk.Directive{
		Name: shape.ContractDirectiveName,
		Args: []string{contractName},
		KV:   kv,
	}
}

// RunPipeline wires pkg into a fresh store with "golang" frontend
// marker and runs the full umbrella → resolver → validator
// sequence with c as the sole registered contract. Returns the
// accumulated diagnostic snapshot for assertion.
func RunPipeline(t *testing.T, c shape.Contract, pkg *sdk.Package) []sdk.Diag {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	umbrella := shape.New().Contracts(c)
	sink := diag.New()
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   sink,
	}
	if err := umbrella.Annotate(ctx); err != nil {
		t.Fatalf("umbrella.Annotate: %v", err)
	}
	if err := umbrella.Resolver().Annotate(ctx); err != nil {
		t.Fatalf("resolver.Annotate: %v", err)
	}
	if err := umbrella.Validator().Annotate(ctx); err != nil {
		t.Fatalf("validator.Annotate: %v", err)
	}
	return sink.Diagnostics()
}

// AssertRole fails when the role stamp for contractName on bag
// does not equal want.
func AssertRole(t *testing.T, bag *sdk.Bag, contractName, want string) {
	t.Helper()
	got, ok := shape.ContractRoleKey(contractName).Get(bag)
	if !ok {
		t.Fatalf("role for %q unstamped; want %q", contractName, want)
	}
	if got != want {
		t.Fatalf("role for %q = %q, want %q", contractName, got, want)
	}
}

// AssertPartner fails when the partner stamp for (contractName,
// role) on bag does not equal want.
func AssertPartner(t *testing.T, bag *sdk.Bag, contractName, role, want string) {
	t.Helper()
	got, ok := shape.ContractPartnerKey(contractName, role).Get(bag)
	if !ok {
		t.Fatalf("partner %q for %q unstamped; want %q", role, contractName, want)
	}
	if got != want {
		t.Fatalf("partner %q for %q = %q, want %q", role, contractName, got, want)
	}
}

// AssertContainsDiag fails when no diagnostic in diags matches
// both sev and contains substr in its message. The failure
// includes the full diagnostic list so the reader sees what was
// (or wasn't) emitted.
func AssertContainsDiag(t *testing.T, diags []sdk.Diag, sev sdk.Severity, substr string) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == sev && strings.Contains(d.Message, substr) {
			return
		}
	}
	t.Fatalf("no %v diagnostic containing %q; got %d diags: %+v",
		sev, substr, len(diags), diags)
}

// AssertNoErrorDiag fails when any [sdk.SeverityError] (or higher)
// diagnostic appears in diags. Use as the happy-path negative
// assertion that a valid contract membership produces no
// validator failures.
func AssertNoErrorDiag(t *testing.T, diags []sdk.Diag) {
	t.Helper()
	for _, d := range diags {
		if d.Severity >= sdk.SeverityError {
			t.Fatalf("unexpected error diagnostic: %+v", d)
		}
	}
}
