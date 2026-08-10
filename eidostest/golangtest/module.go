// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// requirement is one local module the generated code imports.
type requirement struct {
	// path is the module path the generated code imports under.
	path string

	// dir is the checkout the replace directive points at.
	dir string
}

// WithRequire makes a local module importable from the generated code.
//
// Writes `require <modulePath> v0.0.0` plus a `replace` pointing at
// dir into the throwaway module, and raises the `go` directive to that
// module's own floor so a dependency needing a later release than the
// running toolchain's default is not rejected for it.
//
// The gap between the two things this package could already do. A
// generator emitting code that imports nothing but stdlib is served by
// [Generated.WithSource]; one whose output imports a whole third-party
// tree is served by [Generated.InModule], which copies an existing
// module wholesale. Neither serves the common case — output that
// imports the generator's *own* runtime library, one module, sitting
// in the same repository — and every consumer that hit it hand-wrote
// the same go.mod assembly.
//
// Repeatable; each call adds one requirement. The version is always
// `v0.0.0` because the replace makes it unread: the directory on disk
// is what gets built, which is what a test of a generator wants — the
// runtime it is developed against, not a published release of it.
//
// Not combinable with [Generated.InModule]. That copies a go.mod this
// would have to rewrite rather than compose with, and a caller holding
// a base module already has somewhere to declare a dependency.
func (g *Generated) WithRequire(modulePath, dir string) *Generated {
	g.requires = append(g.requires, requirement{path: modulePath, dir: dir})
	g.built = ""
	return g
}

// goModSrc renders the throwaway module's go.mod.
//
// Absolute replace targets, because the module is assembled in a temp
// directory whose depth relative to the caller's checkout is not
// knowable — a relative path would resolve against the wrong root.
func (g *Generated) goModSrc(tb testing.TB) []byte {
	tb.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "module %s\n\ngo %s\n", g.modulePathOf(), g.goFloor(tb))
	for _, r := range g.requires {
		abs, err := filepath.Abs(r.dir)
		if err != nil {
			tb.Fatalf("golangtest: WithRequire(%q, %q): %v", r.path, r.dir, err)
			return nil
		}
		if _, err := os.Stat(filepath.Join(abs, goModFilename)); err != nil {
			tb.Fatalf("golangtest: WithRequire(%q, %q): no %s there, so the replace would "+
				"point at something that is not a module: %v",
				r.path, r.dir, goModFilename, err)
			return nil
		}
		fmt.Fprintf(&b, "\nrequire %s v0.0.0\n\nreplace %s => %s\n",
			r.path, r.path, filepath.ToSlash(abs))
	}
	return []byte(b.String())
}

// goFloor returns the `go` directive the assembled module declares:
// the highest of the caller's setting and every requirement's own.
//
// A required module declaring a later floor than the throwaway one
// fails the build with a message about the *dependency's* go.mod,
// which reads as the dependency being broken rather than as the test
// module being behind it.
func (g *Generated) goFloor(tb testing.TB) string {
	tb.Helper()
	floor := g.goVersionOf()
	for _, r := range g.requires {
		dep := readGoDirective(filepath.Join(r.dir, goModFilename))
		if dep != "" && compareGoVersions(dep, floor) > 0 {
			floor = dep
		}
	}
	return floor
}

// readGoDirective returns the version on a go.mod's `go` line, or the
// empty string when the file cannot be read or declares none.
//
// Deliberately forgiving: an unreadable dependency go.mod is reported
// by [Generated.goModSrc] with the requirement that named it, and
// duplicating the failure here would report it twice.
func readGoDirective(path string) string {
	src, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for line := range strings.Lines(string(src)) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "go" {
			return fields[1]
		}
	}
	return ""
}

// compareGoVersions orders two `go` directive values numerically.
//
// String comparison gets this wrong exactly where it matters: "1.9"
// sorts above "1.26", so a dependency on a current release would lose
// to a floor no longer in use.
func compareGoVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		if d := atoiAt(as, i) - atoiAt(bs, i); d != 0 {
			return d
		}
	}
	return 0
}

// atoiAt returns the i-th field as a number, or 0 when absent or
// non-numeric — which orders `1.26` above `1.26rc1` rather than
// panicking on a toolchain spelling this package does not produce.
func atoiAt(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return 0
	}
	return n
}

// goModFilename names the module file this package reads and writes.
const goModFilename = "go.mod"
