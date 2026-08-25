// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/lang/protobuf/frontend"
	"go.thesmos.sh/eidos/node"
)

// TestConvert_Services_SourcePos covers the BaseNode.SourcePos
// contract for service interfaces and RPC methods: every
// produced node carries the originating file plus a non-zero
// line so the backend's Source: header attribution, `eidos
// explain`, and the manifest's provenance trail can resolve
// each entity back to its declaration.
func TestConvert_Services_SourcePos(t *testing.T) {
	t.Parallel()

	t.Run("each service Interface and Method carries the originating proto file path and line", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "services", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		pkgs := collectPackages(t, env)
		pkg := mainServicesPackage(t, pkgs)
		if len(pkg.Interfaces) == 0 {
			t.Fatalf("expected at least one Interface in the services fixture")
		}
		for _, iface := range pkg.Interfaces {
			ip := iface.Pos()
			if ip.File == "" {
				t.Errorf("Interface %q carries empty Pos.File", iface.Name)
			}
			if ip.Line == 0 {
				t.Errorf("Interface %q carries zero Pos.Line", iface.Name)
			}
			for _, m := range iface.Methods {
				mp := m.Pos()
				if mp.File == "" {
					t.Errorf("Method %q on %q carries empty Pos.File", m.Name, iface.Name)
				}
				if mp.Line == 0 {
					t.Errorf("Method %q on %q carries zero Pos.Line", m.Name, iface.Name)
				}
			}
		}
	})
}

// TestConvert_Services covers the service → node.Interface
// mapping: each `service X { ... }` produces one node.Interface
// named X in the producing package, with one node.Method per
// declared rpc in source order. Methods carry the request and
// response message refs as single-entry Params / Returns slices
// and have no receiver (the interface contract is implicit).
func TestConvert_Services(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "services", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkgs := collectPackages(t, env)

	t.Run("Greeter service produces a node.Interface with one Method per declared rpc", func(t *testing.T) {
		t.Parallel()
		pkg := mainServicesPackage(t, pkgs)
		iface := findInterface(pkg, "Greeter")
		if iface == nil {
			t.Fatalf("Interface %q missing", "Greeter")
		}
		if iface.Package != pkg.Path {
			t.Fatalf("Interface.Package = %q, want %q", iface.Package, pkg.Path)
		}
		gotNames := methodNames(iface)
		wantNames := []string{"SayHello", "SayHellos", "CollectNames", "Chat", "Echo"}
		if !equalStringSlices(gotNames, wantNames) {
			t.Fatalf("Greeter methods = %+v, want %+v", gotNames, wantNames)
		}
	})

	t.Run("cross-package RPC threads the foreign package qualifier through Params and Returns", func(t *testing.T) {
		t.Parallel()
		main := mainServicesPackage(t, pkgs)
		iface := findInterface(main, "Greeter")
		echo := iface.MethodByName("Echo")
		if echo == nil {
			t.Fatalf("Method Echo missing")
		}
		const wantPkg = "eidos.protobuf.testdata.services.external"
		req := echo.Params[0].Type
		if req.Package != wantPkg {
			t.Fatalf("Echo request Type.Package = %q, want %q", req.Package, wantPkg)
		}
		if req.Name != "ExternalRequest" {
			t.Fatalf("Echo request Type.Name = %q, want %q", req.Name, "ExternalRequest")
		}
		resp := echo.Returns[0].Type
		if resp.Package != wantPkg {
			t.Fatalf("Echo response Type.Package = %q, want %q", resp.Package, wantPkg)
		}
		if resp.Name != "ExternalResponse" {
			t.Fatalf("Echo response Type.Name = %q, want %q", resp.Name, "ExternalResponse")
		}
	})

	t.Run("each method carries the request type on Params and the response type on Returns", func(t *testing.T) {
		t.Parallel()
		pkg := mainServicesPackage(t, pkgs)
		iface := findInterface(pkg, "Greeter")
		say := iface.MethodByName("SayHello")
		if say == nil {
			t.Fatalf("Method SayHello missing")
		}
		if len(say.Params) != 1 {
			t.Fatalf("SayHello.Params has %d entries, want 1", len(say.Params))
		}
		req := say.Params[0]
		if req.Type == nil || req.Type.Name != "HelloRequest" {
			t.Fatalf("SayHello request Type = %+v, want TypeRefNamed(HelloRequest)", req.Type)
		}
		if req.Type.Package != pkg.Path {
			t.Fatalf("SayHello request Type.Package = %q, want %q", req.Type.Package, pkg.Path)
		}
		if len(say.Returns) != 1 {
			t.Fatalf("SayHello.Returns has %d entries, want 1", len(say.Returns))
		}
		resp := say.Returns[0].Type
		if resp.Name != "HelloReply" {
			t.Fatalf("SayHello response Name = %q, want %q", resp.Name, "HelloReply")
		}
	})

	t.Run("interface methods carry no receiver", func(t *testing.T) {
		t.Parallel()
		pkg := mainServicesPackage(t, pkgs)
		iface := findInterface(pkg, "Greeter")
		for _, m := range iface.Methods {
			if m.HasReceiver() {
				t.Fatalf("Method %q on Interface has unexpected receiver %+v", m.Name, m.Receiver)
			}
			if m.ReceiverName != "" {
				t.Fatalf("Method %q ReceiverName = %q, want empty", m.Name, m.ReceiverName)
			}
		}
	})
}

// TestConvert_Services_StreamingMeta pins the streaming-meta
// vocabulary on every RPC: proto.service.rpc.stream.client and
// proto.service.rpc.stream.server stamp true only on the matching
// streaming side; unary RPCs carry neither key.
func TestConvert_Services_StreamingMeta(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "services", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := mainServicesPackage(t, collectPackages(t, env))
	iface := findInterface(pkg, "Greeter")

	cases := []struct {
		method       string
		wantClient   bool
		wantServer   bool
		clientStamps bool
		serverStamps bool
	}{
		{"SayHello", false, false, false, false},
		{"SayHellos", false, true, false, true},
		{"CollectNames", true, false, true, false},
		{"Chat", true, true, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.method+" streaming meta matches the RPC declaration", func(t *testing.T) {
			t.Parallel()
			m := iface.MethodByName(tc.method)
			if m == nil {
				t.Fatalf("Method %q missing", tc.method)
			}
			gotClient, clientOK := frontend.MetaRPCStreamClient.Get(m.Meta())
			if clientOK != tc.clientStamps {
				t.Fatalf(
					"%s proto.service.rpc.stream.client presence = %v, want %v",
					tc.method, clientOK, tc.clientStamps,
				)
			}
			if clientOK && gotClient != tc.wantClient {
				t.Fatalf(
					"%s proto.service.rpc.stream.client = %v, want %v",
					tc.method, gotClient, tc.wantClient,
				)
			}
			gotServer, serverOK := frontend.MetaRPCStreamServer.Get(m.Meta())
			if serverOK != tc.serverStamps {
				t.Fatalf(
					"%s proto.service.rpc.stream.server presence = %v, want %v",
					tc.method, serverOK, tc.serverStamps,
				)
			}
			if serverOK && gotServer != tc.wantServer {
				t.Fatalf(
					"%s proto.service.rpc.stream.server = %v, want %v",
					tc.method, gotServer, tc.wantServer,
				)
			}
		})
	}
}

// TestConvert_Services_DocsAndDirectives pins the service-side
// doc-attachment surface: leading comments populate DocLines,
// trailing same-line comments stamp the per-host trailing_doc
// meta, and `+gen:` lines parse through the pipeline's directive
// parser into DirectiveList.
func TestConvert_Services_DocsAndDirectives(t *testing.T) {
	t.Parallel()

	env := loadFixture(t, "services", "./...")
	if env.diag.HasErrors() {
		t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
	}
	pkg := mainServicesPackage(t, collectPackages(t, env))
	iface := findInterface(pkg, "Greeter")

	t.Run("leading comments on the service land on Interface.DocLines", func(t *testing.T) {
		t.Parallel()
		const want = "Greeter is the demo service."
		if !containsLine(iface.DocLines, want) {
			t.Fatalf("Greeter.DocLines = %+v, want a line containing %q", iface.DocLines, want)
		}
	})

	t.Run("trailing comment on the service stamps proto.service.trailing_doc", func(t *testing.T) {
		t.Parallel()
		got, ok := frontend.MetaServiceTrailingDoc.Get(iface.Meta())
		if !ok {
			t.Fatalf("proto.service.trailing_doc missing on Greeter")
		}
		const want = "canonical greeter"
		if !strings.Contains(got, want) {
			t.Fatalf("proto.service.trailing_doc = %q, want a substring %q", got, want)
		}
	})

	t.Run("trailing comment on an rpc stamps proto.service.rpc.trailing_doc", func(t *testing.T) {
		t.Parallel()
		say := iface.MethodByName("SayHello")
		got, ok := frontend.MetaRPCTrailingDoc.Get(say.Meta())
		if !ok {
			t.Fatalf("proto.service.rpc.trailing_doc missing on SayHello")
		}
		const want = "unary"
		if !strings.Contains(got, want) {
			t.Fatalf("proto.service.rpc.trailing_doc on SayHello = %q, want a substring %q", got, want)
		}
	})

	t.Run("+gen:mock on a service lands on Interface.DirectiveList", func(t *testing.T) {
		t.Parallel()
		var found bool
		for _, d := range iface.DirectiveList {
			if string(d.Name) == "mock" && !d.Negated {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Greeter.DirectiveList missing a positive `mock` directive; got %+v", iface.DirectiveList)
		}
	})
}

// mainServicesPackage returns the services-fixture's primary
// package (the one declaring Greeter / SilentService). The
// fixture spans two packages — the main `services` package and
// the sibling `services.external` package — so callers asking
// for the main package go through this helper rather than
// requireSinglePackage.
func mainServicesPackage(t *testing.T, pkgs []*node.Package) *node.Package {
	t.Helper()
	const mainPath = "eidos.protobuf.testdata.services"
	for _, p := range pkgs {
		if p.Path == mainPath {
			return p
		}
	}
	t.Fatalf("services-fixture main package %q missing; got %+v", mainPath, packagePaths(pkgs))
	return nil
}

// findInterface returns the Interface whose Name matches name, or
// nil when absent.
func findInterface(pkg *node.Package, name string) *node.Interface {
	for _, i := range pkg.Interfaces {
		if i.Name == name {
			return i
		}
	}
	return nil
}

// methodNames returns the Name of every method on iface.
func methodNames(iface *node.Interface) []string {
	out := make([]string, 0, len(iface.Methods))
	for _, m := range iface.Methods {
		out = append(out, m.Name)
	}
	return out
}

// equalStringSlices reports whether got and want are
// element-for-element equal.
func equalStringSlices(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
