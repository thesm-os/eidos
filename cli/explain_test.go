// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli_test

import (
	"encoding/json"
	"errors"
	"flag"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/cli"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// explainMetaKey is the bool meta key used by the RawMessage
// regression test below. Declared at package scope so the global
// registry registers it once.
var explainMetaKey = meta.NewKey("cli.explain.test.meta", meta.BoolParser)

// sourceFrontend is a [plugin.Frontend] that adds a fixed
// [*node.Package] to the store on Load. Source-side Explain tests
// use it to seed the node graph the source-resolution path walks.
type sourceFrontend struct {
	name string
	pkg  *node.Package
}

func (f sourceFrontend) Name() string { return f.name }
func (f sourceFrontend) Load(ctx *plugin.FrontendContext) error {
	return ctx.Store.Nodes().AddPackage(f.pkg)
}

// emittingGenerator is a [plugin.Generator] that adds a single
// fixture package to the store on Generate. Used by Explain tests
// that need a known entity in the emit graph.
type emittingGenerator struct {
	name string
	pkg  *emit.Package
}

func (g emittingGenerator) Name() string { return g.name }
func (g emittingGenerator) Generate(ctx *plugin.GeneratorContext) error {
	return ctx.Store.Emit().AddPackage(g.pkg)
}

// TestExplainCommand_EntitySelector covers the entity-only form:
// `pkg.QName` resolves to the matching emit entity and prints its
// kind summary.
func TestExplainCommand_EntitySelector(t *testing.T) {
	t.Parallel()

	t.Run("resolved entity prints Kind line", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		gen := emittingGenerator{
			name: "fixturegen",
			pkg: &emit.Package{
				Name: "users", Path: "users",
				Structs: []*emit.Struct{{
					Name: "User", Package: "users",
					Target: emit.Target{Dir: "users", Filename: "user.go", Package: "users"},
				}},
			},
		}
		cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				gen,
				stubBackend{name: "be", lang: "stub"},
			},
			Selector: "users.User",
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		out := stdout.String()
		if !strings.Contains(out, "Kind:") || !strings.Contains(out, "emit.struct") {
			t.Fatalf("expected Kind line for struct; got %q", out)
		}
	})

	t.Run("unresolved anchor exits ExitUserError", func(t *testing.T) {
		t.Parallel()
		env, _, stderr := freshEnv(t, "eidos")
		cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				stubBackend{name: "be", lang: "stub"},
			},
			Selector: "nope.Missing",
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitUserError {
			t.Fatalf("Execute = %d, want ExitUserError", code)
		}
		if !strings.Contains(stderr.String(), "resolves to no source or emit entity") {
			t.Fatalf("expected not-found diagnostic; got %q", stderr.String())
		}
	})
}

// TestExplainCommand_MetaJSONRawMessage pins the bugfix that
// surfaced when meta values arrive from the frontend cache as
// [json.RawMessage] payloads rather than typed Go values.
// Pre-fix, `%v` formatted a RawMessage as a decimal byte slice
// (`[116 114 117 101]` for "true"); the fix's formatMetaValue
// helper detects the wire form and JSON-decodes it before
// rendering, so the Metadata: section in `eidos explain` shows
// the original value regardless of whether the bag was cached.
func TestExplainCommand_MetaJSONRawMessage(t *testing.T) {
	t.Parallel()

	t.Run(
		"explain renders RawMessage-wrapped bool meta as 'true', not as bytes",
		func(t *testing.T) {
			t.Parallel()
			env, stdout, _ := freshEnv(t, "eidos")
			src := &node.Struct{Name: "User", Package: "users"}
			explainMetaKey.Set(src.EnsureMeta(), true, "fixture")
			// Round-trip the Bag through JSON so its stored values
			// become json.RawMessage — the cache path the bug fix
			// exercises in production.
			raw, err := json.Marshal(src.Meta())
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			src.MetaBag = nil
			if err := json.Unmarshal(raw, src.EnsureMeta()); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			pkg := &node.Package{
				Name: "users", Path: "users",
				Structs: []*node.Struct{src},
			}
			cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
				Plugins: []plugin.Plugin{
					sourceFrontend{name: "fe", pkg: pkg},
					stubBackend{name: "be", lang: "stub"},
				},
				Selector: "users.User",
			}}
			if code := cmd.Execute(t.Context(), env); code != cli.ExitOK {
				t.Fatalf("Execute = %d, want ExitOK", code)
			}
			out := stdout.String()
			if !strings.Contains(out, "cli.explain.test.meta = true") {
				t.Fatalf("expected bool meta rendered as 'true'; got:\n%s", out)
			}
			if strings.Contains(out, "[116 114 117 101]") {
				t.Fatalf("RawMessage leaked as raw bytes; got:\n%s", out)
			}
		},
	)
}

// TestExplainCommand_SlotSelector covers the slot form: anchor
// resolves to an entity that owns slots, modifier names a slot
// with at least one contribution.
func TestExplainCommand_SlotSelector(t *testing.T) {
	t.Parallel()

	t.Run("slot contributions print with set-by attribution", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		host := &emit.Struct{
			Name: "User", Package: "users",
			Target: emit.Target{Dir: "users", Filename: "user.go", Package: "users"},
		}
		// Pre-seed a slot contribution so the generator runs and the
		// host carries one entry when Explain inspects it.
		_ = host.FieldsSlot().Append(
			&emit.Field{Name: "Email", Type: emit.Builtin("string")},
			emit.Provenance{SetBy: "validationgen"},
		)
		gen := emittingGenerator{
			name: "fixturegen",
			pkg: &emit.Package{
				Name: "users", Path: "users",
				Structs: []*emit.Struct{host},
			},
		}
		cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				gen,
				stubBackend{name: "be", lang: "stub"},
			},
			Selector: "users.User:fields",
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout.String(), "validationgen") {
			t.Fatalf("expected set-by attribution; got %q", stdout.String())
		}
	})

	t.Run("empty slot exits ExitUserError", func(t *testing.T) {
		t.Parallel()
		env, _, stderr := freshEnv(t, "eidos")
		gen := emittingGenerator{
			name: "fixturegen",
			pkg: &emit.Package{
				Name: "users", Path: "users",
				Structs: []*emit.Struct{{
					Name: "User", Package: "users",
					Target: emit.Target{Dir: "users", Filename: "user.go", Package: "users"},
				}},
			},
		}
		cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				gen,
				stubBackend{name: "be", lang: "stub"},
			},
			Selector: "users.User:fields",
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitUserError {
			t.Fatalf("Execute = %d, want ExitUserError", code)
		}
		if !strings.Contains(stderr.String(), "no contributions") {
			t.Fatalf("expected no-contributions diagnostic; got %q", stderr.String())
		}
	})
}

// TestParseSelector covers the parser's three forms plus the
// reject paths for empty inputs / missing modifiers. The parser
// is unexported so the test exercises it via Execute's
// command-line surface (an unparseable selector returns
// ExitUserError without running the pipeline).
func TestParseSelector(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		selector string
		wantCode int
		mustErr  string
	}{
		{
			name:     "empty selector",
			selector: "",
			wantCode: cli.ExitUserError,
			mustErr:  "selector is required",
		},
		{
			name:     "anchor missing in slot form",
			selector: ":fields",
			wantCode: cli.ExitUserError,
			mustErr:  "anchor and slot name are both required",
		},
		{
			name:     "modifier missing in meta form",
			selector: "users.User#",
			wantCode: cli.ExitUserError,
			mustErr:  "anchor and key are both required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env, _, stderr := freshEnv(t, "eidos")
			cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
				Plugins: []plugin.Plugin{
					stubFrontend{name: "fe"},
					stubBackend{name: "be", lang: "stub"},
				},
				Selector: tc.selector,
			}}
			code := cmd.Execute(t.Context(), env)
			if code != tc.wantCode {
				t.Fatalf("Execute = %d, want %d", code, tc.wantCode)
			}
			if !strings.Contains(stderr.String(), tc.mustErr) {
				t.Fatalf("stderr should contain %q; got %q", tc.mustErr, stderr.String())
			}
		})
	}
}

// TestExplainCommand_ProtoServiceShape covers the source-anchored
// explain path against a proto-flavoured service: a Greeter
// node.Interface carrying streaming meta on its methods renders
// the Kind line, the methods, and the meta keys downstream
// tooling expects when inspecting a proto-derived RPC.
func TestExplainCommand_ProtoServiceShape(t *testing.T) {
	t.Parallel()

	t.Run("explain against a proto-derived service renders the Kind line and streaming meta", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		streamServer := meta.EnsureKey("proto.service.rpc.stream.server", meta.BoolParser)
		// SayHellos models the server-streaming RPC's emitted
		// Method shape: one positional param, one return ref, and
		// the streaming-meta stamp the protobuf frontend produces.
		stream := &node.Method{Name: "SayHellos"}
		streamServer.Set(stream.EnsureMeta(), true, "protobuf")
		iface := &node.Interface{
			Name: "Greeter", Package: "eidos.protobuf.testdata.services",
			Methods: []*node.Method{
				{Name: "SayHello"},
				stream,
			},
		}
		pkg := &node.Package{
			Name: "services", Path: "eidos.protobuf.testdata.services",
			Interfaces: []*node.Interface{iface},
		}
		cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
			Plugins: []plugin.Plugin{
				sourceFrontend{name: "fe", pkg: pkg},
				stubBackend{name: "be", lang: "stub"},
			},
			Selector: "services.Greeter",
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		out := stdout.String()
		if !strings.Contains(out, "interface") {
			t.Fatalf("expected Kind line for interface; got:\n%s", out)
		}
		if !strings.Contains(out, "Greeter") {
			t.Fatalf("expected service name in output; got:\n%s", out)
		}
	})

	t.Run("explain on an RPC member surfaces its streaming meta", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		streamServer := meta.EnsureKey("proto.service.rpc.stream.server.member", meta.BoolParser)
		stream := &node.Method{Name: "SayHellos"}
		streamServer.Set(stream.EnsureMeta(), true, "protobuf")
		iface := &node.Interface{
			Name: "Greeter", Package: "eidos.protobuf.testdata.services",
			Methods: []*node.Method{stream},
		}
		pkg := &node.Package{
			Name: "services", Path: "eidos.protobuf.testdata.services",
			Interfaces: []*node.Interface{iface},
		}
		cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
			Plugins: []plugin.Plugin{
				sourceFrontend{name: "fe", pkg: pkg},
				stubBackend{name: "be", lang: "stub"},
			},
			Selector: "services.Greeter.SayHellos",
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK; stdout:\n%s", code, stdout.String())
		}
		out := stdout.String()
		if !strings.Contains(out, "SayHellos") {
			t.Fatalf("expected method name in output; got:\n%s", out)
		}
		if !strings.Contains(out, "proto.service.rpc.stream.server.member = true") {
			t.Fatalf("expected streaming meta rendered; got:\n%s", out)
		}
	})
}

// TestExplainCommand_ConfigError covers the user-error path.
func TestExplainCommand_ConfigError(t *testing.T) {
	t.Parallel()

	t.Run("missing plugin exits ExitUserError", func(t *testing.T) {
		t.Parallel()
		env, _, stderr := freshEnv(t, "eidos")
		cfg := cli.DefaultConfig()
		cfg.Plugins = []cli.ConfigPlugin{{Name: "ghost"}}
		cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{
			File: cfg,
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				stubBackend{name: "be", lang: "stub"},
			},
			Selector: "anything",
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitUserError {
			t.Fatalf("Execute = %d, want ExitUserError", code)
		}
		if !strings.Contains(stderr.String(), "ghost") {
			t.Fatalf("stderr should name the missing plugin; got %q", stderr.String())
		}
	})
}

// errSentinelImport keeps the errors import referenced in case
// further test helpers grow.
var _ = errors.New

// TestExplainCommand_RegisterFlags_Routing pins the routing-flag
// wiring on ExplainCommand — parsed values land on
// [cli.ExplainConfig.Routing].
func TestExplainCommand_RegisterFlags_Routing(t *testing.T) {
	t.Parallel()

	t.Run("Routing flags parse onto ExplainCommand.Config.Routing", func(t *testing.T) {
		t.Parallel()
		var cmd cli.ExplainCommand
		fs := flag.NewFlagSet("explain", flag.ContinueOnError)
		cmd.RegisterFlags(fs)
		assertRoutingFlagsParse(t, fs, &cmd.Config.Routing)
	})
}
