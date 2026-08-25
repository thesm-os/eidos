// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package directive_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
)

// newSinkAndPlugin builds a fresh diag.Sink and returns it along with
// a PluginSink wrapping it. Validation tests inspect the sink's
// Diagnostics afterwards.
func newSinkAndPlugin() (*diag.Sink, *diag.PluginSink) {
	s := diag.New()
	p := s.For("test-validator")
	return s, p
}

func TestValidate(t *testing.T) {
	t.Parallel()

	t.Run("returns true and emits nothing when every directive matches its schema", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").Build()), "register")
		d := &directive.Directive{Name: "mock", Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if !directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should return true on conformant input")
		}
		if sink.Count(diag.Error) != 0 {
			t.Fatalf("expected zero errors; got %+v", sink.Diagnostics())
		}
	})

	// A name nothing claims parses, matches nothing, stamps nothing and
	// generates nothing — output indistinguishable from a declaration
	// that asked for nothing at all.
	t.Run("rejects a directive no schema claims", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("builder").Build()), "register")
		d := &directive.Directive{Name: "buildr", Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{d}, "struct", r, plugin) {
			t.Fatalf("a directive nothing claims should fail validation")
		}
		if !errorContaining(sink, `did you mean "builder"`) {
			t.Fatalf("a near-miss should be named as one; got %+v", sink.Diagnostics())
		}
	})

	t.Run("says so plainly when nothing is close", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("builder").Build()), "register")
		d := &directive.Directive{Name: "xyzzy", Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{d}, "struct", r, plugin) {
			t.Fatalf("a directive nothing claims should fail validation")
		}
		// Suggesting the only registered name for an unrelated one would
		// send the author to rename something they never wrote.
		if errorContaining(sink, "did you mean") {
			t.Fatalf("nothing here is a plausible typo for xyzzy; got %+v", sink.Diagnostics())
		}
	})

	// The run deliberately narrower than the source it reads: every
	// unclaimed name in it is a line the author meant, for a plugin this
	// run did not register.
	t.Run("permits unclaimed names when the registry allows them", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry().AllowUnclaimed()
		d := &directive.Directive{Name: "anything", Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if !directive.Validate([]*directive.Directive{d}, "struct", r, plugin) {
			t.Fatalf("an allowing registry should not fail validation")
		}
		if sink.Count(diag.Error) != 0 {
			t.Fatalf("an allowing registry should emit no errors; got %+v", sink.Diagnostics())
		}
	})

	t.Run("rejects -gen: form when AllowNegated is false", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").DenyNegation().Build()), "register")
		d := &directive.Directive{Name: "mock", Negated: true, Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should fail on disallowed negation")
		}
		if !errorContaining(sink, "does not accept the -gen: form") {
			t.Fatalf("missing expected diagnostic; got %+v", sink.Diagnostics())
		}
	})

	t.Run("rejects directive on a node kind outside AppliesTo", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").On("interface").Build()), "register")
		d := &directive.Directive{Name: "mock", Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{d}, "struct", r, plugin) {
			t.Fatalf("Validate should fail when kind not in AppliesTo")
		}
		if !errorContaining(sink, "does not apply to") {
			t.Fatalf("missing expected diagnostic; got %+v", sink.Diagnostics())
		}
	})

	t.Run("accepts directive on any kind when AppliesTo is empty", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").Build()), "register")
		d := &directive.Directive{Name: "mock", Pos: position.At("a.go", 1, 1)}
		_, plugin := newSinkAndPlugin()
		if !directive.Validate([]*directive.Directive{d}, "anything", r, plugin) {
			t.Fatalf("empty AppliesTo should accept any kind")
		}
	})

	t.Run("requires sibling directives named in Requires", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").Requires("repo").Build()), "register")
		d := &directive.Directive{Name: "mock", Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should fail when Requires is unsatisfied")
		}
		if !errorContaining(sink, "requires") {
			t.Fatalf("missing expected diagnostic; got %+v", sink.Diagnostics())
		}
	})

	t.Run("passes when Requires is satisfied", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").Requires("repo").Build()), "register")
		// Registered as well as written: a peer satisfying Requires is a
		// directive like any other, and an unclaimed one is now refused.
		assertNoError(t, r.Register(directive.NewSchema("repo").Build()), "register")
		mock := &directive.Directive{Name: "mock", Pos: position.At("a.go", 1, 1)}
		repo := &directive.Directive{Name: "repo", Pos: position.At("a.go", 2, 1)}
		_, plugin := newSinkAndPlugin()
		if !directive.Validate([]*directive.Directive{mock, repo}, "interface", r, plugin) {
			t.Fatalf("Validate should pass when Requires is satisfied")
		}
	})

	t.Run("rejects mutually exclusive directives that co-occur", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").ExclusiveWith("skip").Build()), "register")
		mock := &directive.Directive{Name: "mock", Pos: position.At("a.go", 1, 1)}
		skip := &directive.Directive{Name: "skip", Pos: position.At("a.go", 2, 1)}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{mock, skip}, "interface", r, plugin) {
			t.Fatalf("Validate should fail when exclusion is violated")
		}
		if !errorContaining(sink, "mutually exclusive") {
			t.Fatalf("missing expected diagnostic; got %+v", sink.Diagnostics())
		}
	})

	t.Run("rejects missing RequiredKeys", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").RequiredKeys("target").Build()), "register")
		d := &directive.Directive{Name: "mock", Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should fail when a required key is missing")
		}
		if !errorContaining(sink, "requires key") {
			t.Fatalf("missing expected diagnostic; got %+v", sink.Diagnostics())
		}
	})

	t.Run("rejects KV keys outside AllowedKeys", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").AllowedKeys("target").Build()), "register")
		d := &directive.Directive{
			Name: "mock",
			KV:   map[string]string{"target": "Repo", "intruder": "x"},
			Pos:  position.At("a.go", 1, 1),
		}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should fail on a disallowed key")
		}
		if !errorContaining(sink, "does not accept key") {
			t.Fatalf("missing expected diagnostic; got %+v", sink.Diagnostics())
		}
	})

	t.Run("rejects any key when the schema denies keys", func(t *testing.T) {
		t.Parallel()
		// A directive whose contract is "takes no arguments" could not
		// state it: an empty AllowedKeys means unrestricted, so a stray
		// key parsed, validated, stamped nothing, and the author
		// believed it had an effect.
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").DenyKeys().Build()), "register")
		d := &directive.Directive{
			Name: "mock",
			KV:   map[string]string{"pkg": "mine"},
			Pos:  position.At("a.go", 1, 1),
		}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should fail on a key a key-denying schema received")
		}
		if !errorContaining(sink, "accepts no keys") {
			t.Fatalf("missing expected diagnostic; got %+v", sink.Diagnostics())
		}
	})

	t.Run("accepts a key-denying directive carrying no keys", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").DenyKeys().Build()), "register")
		d := &directive.Directive{Name: "mock", Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if !directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should pass; got %+v", sink.Diagnostics())
		}
	})

	t.Run("a schema declaring neither still accepts any key", func(t *testing.T) {
		t.Parallel()
		// The zero value must keep meaning what it means today, or
		// every existing schema changes behaviour on upgrade.
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").Build()), "register")
		d := &directive.Directive{
			Name: "mock",
			KV:   map[string]string{"anything": "goes"},
			Pos:  position.At("a.go", 1, 1),
		}
		sink, plugin := newSinkAndPlugin()
		if !directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should pass; got %+v", sink.Diagnostics())
		}
	})

	t.Run("rejects a missing required positional", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		s := directive.NewSchema("mock").Positional("variant", directive.Required()).Build()
		assertNoError(t, r.Register(s), "register")
		d := &directive.Directive{Name: "mock", Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should fail for missing required positional")
		}
		if !errorContaining(sink, "requires positional") {
			t.Fatalf("missing expected diagnostic; got %+v", sink.Diagnostics())
		}
	})

	t.Run("folds a default value into Args when the optional slot is missing", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		s := directive.NewSchema("mock").Positional("variant", directive.Default("stub")).Build()
		assertNoError(t, r.Register(s), "register")
		d := &directive.Directive{Name: "mock", Pos: position.At("a.go", 1, 1)}
		_, plugin := newSinkAndPlugin()
		if !directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should pass when default is folded")
		}
		if d.Arg(0) != "stub" {
			t.Fatalf("Args after default = %v; want [stub]", d.Args)
		}
	})

	t.Run("rejects an OneOf positional outside the enumeration", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		s := directive.NewSchema("mock").Positional("variant", directive.OneOf("a", "b")).Build()
		assertNoError(t, r.Register(s), "register")
		d := &directive.Directive{Name: "mock", Args: []string{"c"}, Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should fail when positional is outside OneOf")
		}
		if !errorContaining(sink, "must be one of") {
			t.Fatalf("missing expected diagnostic; got %+v", sink.Diagnostics())
		}
	})

	t.Run("accepts an OneOf positional inside the enumeration", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		s := directive.NewSchema("mock").Positional("variant", directive.OneOf("a", "b")).Build()
		assertNoError(t, r.Register(s), "register")
		d := &directive.Directive{Name: "mock", Args: []string{"a"}, Pos: position.At("a.go", 1, 1)}
		_, plugin := newSinkAndPlugin()
		if !directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should accept a value inside OneOf")
		}
	})

	t.Run("rejects extra positionals when AllowExtraPositional is false", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		s := directive.NewSchema("mock").Positional("variant").Build()
		assertNoError(t, r.Register(s), "register")
		d := &directive.Directive{Name: "mock", Args: []string{"a", "b"}, Pos: position.At("a.go", 1, 1)}
		sink, plugin := newSinkAndPlugin()
		if directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should fail when extra positionals are not allowed")
		}
		if !errorContaining(sink, "accepts") {
			t.Fatalf("missing expected diagnostic; got %+v", sink.Diagnostics())
		}
	})

	t.Run("accepts extra positionals when AllowExtraPositional is true", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		s := directive.NewSchema("mock").Positional("variant").AllowExtraPositional().Build()
		assertNoError(t, r.Register(s), "register")
		d := &directive.Directive{Name: "mock", Args: []string{"a", "b"}, Pos: position.At("a.go", 1, 1)}
		_, plugin := newSinkAndPlugin()
		if !directive.Validate([]*directive.Directive{d}, "interface", r, plugin) {
			t.Fatalf("Validate should pass when extra positionals are allowed")
		}
	})

	t.Run("returns false when any single directive fails", func(t *testing.T) {
		t.Parallel()
		r := directive.NewRegistry()
		assertNoError(t, r.Register(directive.NewSchema("mock").RequiredKeys("target").Build()), "register mock")
		assertNoError(t, r.Register(directive.NewSchema("repo").Build()), "register repo")
		bad := &directive.Directive{Name: "mock", Pos: position.At("a.go", 1, 1)}
		good := &directive.Directive{Name: "repo", Pos: position.At("a.go", 2, 1)}
		if directive.Validate([]*directive.Directive{bad, good}, "interface", r, mustPluginSink()) {
			t.Fatalf("Validate should return false when at least one directive fails")
		}
	})
}

// mustPluginSink returns a discard-ish PluginSink — diagnostics go into
// a freshly-allocated Sink that the test ignores. Used by checks that
// only care about the boolean Validate return.
func mustPluginSink() *diag.PluginSink {
	return diag.New().For("validator-test")
}

// errorContaining reports whether sink holds an Error diagnostic whose
// message contains substr. The check is case-sensitive and substring-
// based; it intentionally tolerates message-text refactoring as long
// as the structural fragment survives.
func errorContaining(sink *diag.Sink, substr string) bool {
	for _, d := range sink.Diagnostics() {
		if d.Severity == diag.Error && strings.Contains(d.Message, substr) {
			return true
		}
	}
	return false
}
