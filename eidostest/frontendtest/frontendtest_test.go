// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontendtest_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/eidostest/frontendtest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// sourceDirEnv carries the SourceDir value from a test to the
// subprocess that drives the harness with it. Its presence is
// also what tells [TestHelperRunRejectsSourceDir] it is running
// as that subprocess rather than as an ordinary test.
const sourceDirEnv = "EIDOS_FRONTENDTEST_SOURCE_DIR"

// helperTestName is the subprocess entry point, named here so the
// -test.run filter cannot drift from the function it selects.
const helperTestName = "TestHelperRunRejectsSourceDir"

// TestDemoFixture covers the [frontendtest.DemoFixture] helper:
// it resolves through [runtime.Caller] and must point at the
// shared demoproject testdata regardless of the test's working
// directory — and at a directory that is actually there, which
// the shape assertions alone never established.
func TestDemoFixture(t *testing.T) {
	t.Parallel()

	t.Run("path resolves to eidostest/testdata/demoproject", func(t *testing.T) {
		t.Parallel()
		got := frontendtest.DemoFixture(t)
		if !strings.HasSuffix(got, filepath.Join("testdata", "demoproject")) {
			t.Fatalf("DemoFixture returned %q; expected suffix testdata/demoproject", got)
		}
	})

	t.Run("path is absolute", func(t *testing.T) {
		t.Parallel()
		got := frontendtest.DemoFixture(t)
		if !filepath.IsAbs(got) {
			t.Fatalf("DemoFixture returned non-absolute path %q", got)
		}
	})

	// Passes inside the workspace by construction — the fixture is
	// on disk two directories up. It is here as the regression
	// guard for the move that would silently reintroduce the
	// defect: a helper returning a well-shaped path to nothing.
	// The population it protects (a consumer resolving this from
	// the published module, where eidostest/testdata does not
	// exist) can only be reached by a smoke test run with
	// GOWORK=off against the packaged module.
	t.Run("the shared demo fixture exists at the path DemoFixture returns", func(t *testing.T) {
		t.Parallel()
		got := frontendtest.DemoFixture(t)
		if _, err := os.Stat(got); err != nil {
			t.Fatalf("DemoFixture returned %q, which does not exist: %v", got, err)
		}
	})
}

// TestRun_RejectsUnusableSourceDir pins the guard that turns a
// SourceDir the frontend cannot load into a stop at the harness
// boundary. Without it the pipeline converts the same failure
// into Error diagnostics and [frontendtest.Run] returns them in a
// [frontendtest.Result], so a test that asserts nothing about the
// store passes over an empty one.
//
// Each subtest costs one process spawn; see [runHarnessSubprocess]
// for why the fatal cannot be observed in-process.
func TestRun_RejectsUnusableSourceDir(t *testing.T) {
	t.Parallel()

	t.Run("a run against a source dir that does not exist fails the test", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "definitely-not-here")
		out := runHarnessSubprocess(t, missing)
		assertOutputMentions(t, out, "frontendtest.Run:", missing, "no such file or directory", "opts.SourceDir")
	})

	t.Run("a run against a source dir that is a file fails the test", func(t *testing.T) {
		t.Parallel()
		file := filepath.Join(t.TempDir(), "not-a-dir.go")
		if err := os.WriteFile(file, []byte("package p\n"), 0o600); err != nil {
			t.Fatalf("writing fixture file: %v", err)
		}
		out := runHarnessSubprocess(t, file)
		assertOutputMentions(t, out, "frontendtest.Run:", file, "not a directory")
	})
}

// TestHelperRunRejectsSourceDir is the body [runHarnessSubprocess]
// executes in a child process: it drives [frontendtest.Run] with
// the SourceDir carried in [sourceDirEnv] and expects the harness
// to fail the test. Without that variable set it is a skip, so an
// ordinary `go test` run neither drives it nor reports it as a
// failure.
func TestHelperRunRejectsSourceDir(t *testing.T) {
	t.Parallel()
	dir, ok := os.LookupEnv(sourceDirEnv)
	if !ok {
		t.Skipf("subprocess entry point; driven by TestRun_RejectsUnusableSourceDir via %s", sourceDirEnv)
	}
	frontendtest.Run(t, frontendtest.RunOptions{
		Frontend:      &fakeFrontend{name: "fake-fe"},
		SourceDir:     dir,
		Pattern:       "single",
		OutputPackage: "out",
	})
}

// runHarnessSubprocess re-executes this test binary against
// [helperTestName] with sourceDir in the environment, and returns
// the child's combined output after asserting it exited non-zero.
//
// The re-exec is not incidental. [frontendtest.Run] takes a
// concrete *testing.T, so its t.Fatalf cannot be routed to a
// stand-in the way a testing.TB-shaped harness allows; and a
// failure recorded on any *testing.T propagates up every parent,
// so a subtest cannot absorb it either. A child process is the
// only boundary that contains the failure while still exercising
// the real wiring rather than a guard function called in
// isolation.
//
// Cost is one process spawn (tens of milliseconds) per call. The
// child shares nothing with the caller but a copied environment,
// so callers stay parallel-safe.
func runHarnessSubprocess(t *testing.T, sourceDir string) string {
	t.Helper()
	//nolint:gosec // the command is this test binary plus two constant flags; sourceDir travels in the environment, never on the command line
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^"+helperTestName+"$", "-test.v")
	cmd.Env = append(os.Environ(), sourceDirEnv+"="+sourceDir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("frontendtest.Run accepted SourceDir %q; expected the harness to fail the test:\n%s", sourceDir, out)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("running %s: %v:\n%s", os.Args[0], err, out)
	}
	return string(out)
}

// assertOutputMentions fails t for every want absent from out. The
// harness's fatal has to name what failed and what the reader does
// next, so the message text is part of the contract, not incidental
// phrasing.
func assertOutputMentions(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Errorf("harness output does not mention %q:\n%s", w, out)
		}
	}
}

// TestLoadDirect_DrivesFrontendLoad covers the happy path of
// [frontendtest.LoadDirect]: a frontend invoked through the
// helper populates the returned [Result.Store] from its Load
// pass and the helper supplies a fresh diagnostic sink the
// caller inspects on return.
func TestLoadDirect_DrivesFrontendLoad(t *testing.T) {
	t.Parallel()
	result := frontendtest.LoadDirect(t, frontendtest.RunOptions{
		Frontend:  &fakeFrontend{name: "fake-fe"},
		SourceDir: "/synthetic",
		Pattern:   "single",
	})
	if result.Store == nil {
		t.Fatalf("LoadDirect did not populate Result.Store")
	}
	if result.Diag == nil {
		t.Fatalf("LoadDirect did not populate Result.Diag")
	}
	if got := result.Store.Nodes().Packages().Len(); got != 1 {
		t.Fatalf("Result.Store has %d packages; expected 1", got)
	}
}

// TestLoadDirect_ForwardsOptionsToFrontend covers the
// options-forwarding contract: the harness sets `dir` from
// [RunOptions.SourceDir] and merges [RunOptions.FrontendOptions]
// over that default; the frontend sees the merged map on its
// [plugin.OptionsProvider.SetOptions] call.
func TestLoadDirect_ForwardsOptionsToFrontend(t *testing.T) {
	t.Parallel()
	f := &fakeFrontend{name: "fake-fe"}
	frontendtest.LoadDirect(t, frontendtest.RunOptions{
		Frontend:        f,
		SourceDir:       "/synthetic",
		Pattern:         "single",
		FrontendOptions: map[string]string{"label": "alpha"},
	})
	if f.opts.Label != "alpha" {
		t.Fatalf("frontend did not receive Label=alpha; got %q", f.opts.Label)
	}
	if f.opts.Dir != "/synthetic" {
		t.Fatalf("frontend did not receive dir=/synthetic; got %q", f.opts.Dir)
	}
}

// TestLoadDirect_PluginOptionsBeatsFrontendOptions pins the
// merge precedence documented on [RunOptions.FrontendOptions]:
// per-plugin overrides win over the frontend-specific shortcut.
func TestLoadDirect_PluginOptionsBeatsFrontendOptions(t *testing.T) {
	t.Parallel()
	f := &fakeFrontend{name: "fake-fe"}
	frontendtest.LoadDirect(t, frontendtest.RunOptions{
		Frontend:        f,
		SourceDir:       "/synthetic",
		Pattern:         "single",
		FrontendOptions: map[string]string{"label": "from-fe-opts"},
		PluginOptions:   map[string]map[string]string{"fake-fe": {"label": "from-plugin-opts"}},
	})
	if f.opts.Label != "from-plugin-opts" {
		t.Fatalf("expected PluginOptions to win; got Label=%q", f.opts.Label)
	}
}

// TestRun_HappyPath covers [frontendtest.Run] driving a fake
// frontend through a full pipeline build with a stub backend.
// Verifies the harness wires the pipeline correctly and the
// returned [Result] carries Store / Diag / Sink populated by
// the run.
//
// SourceDir is an empty temp directory rather than the synthetic
// path the LoadDirect cases use: the frontend ignores it, but Run
// stats it, so it has to be somewhere real.
func TestRun_HappyPath(t *testing.T) {
	t.Parallel()
	result := frontendtest.Run(t, frontendtest.RunOptions{
		Frontend:      &fakeFrontend{name: "fake-fe"},
		SourceDir:     t.TempDir(),
		Pattern:       "single",
		OutputPackage: "out",
	})
	if result.Store == nil {
		t.Fatalf("Run did not populate Result.Store")
	}
	if result.RunErr != nil {
		t.Fatalf("Run returned RunErr=%v", result.RunErr)
	}
	if got := result.Store.Nodes().Packages().Len(); got != 1 {
		t.Fatalf("Result.Store has %d packages; expected 1", got)
	}
}

// fakeFrontend is an in-memory frontend that the harness tests
// drive. Implements [plugin.OptionsProvider] so the options
// merge tests have a real decode target.
type fakeFrontend struct {
	name string
	opts fakeFrontendOpts
}

// fakeFrontendOpts is the bound options shape the frontend
// exposes through [plugin.OptionsProvider]. Fields cover both
// the `dir` default and a free-text `label` so tests can
// observe the merge order.
type fakeFrontendOpts struct {
	Dir   string `eidos:"dir"`
	Label string `eidos:"label"`
}

// Name returns the configured identifier.
func (f *fakeFrontend) Name() string { return f.name }

// OptionsSchema returns the reflected schema of
// [fakeFrontendOpts].
func (*fakeFrontend) OptionsSchema() opt.Schema { return opt.Reflect(fakeFrontendOpts{}) }

// SetOptions decodes opts into the frontend's options struct.
func (f *fakeFrontend) SetOptions(opts opt.Options) error {
	if err := opts.Decode(&f.opts); err != nil {
		return fmt.Errorf("fakeFrontend: SetOptions: %w", err)
	}
	return nil
}

// Load adds a single synthetic package keyed by the pattern.
func (*fakeFrontend) Load(ctx *plugin.FrontendContext) error {
	switch ctx.Pattern {
	case "single":
		pkg := &node.Package{Name: "fake", Path: "example.com/fake"}
		pkg.Structs = []*node.Struct{{Name: "User", Package: pkg.Path}}
		if err := ctx.Store.Nodes().AddPackage(pkg); err != nil {
			return fmt.Errorf("fakeFrontend: AddPackage: %w", err)
		}
	default:
		// Unknown patterns add nothing; the harness's
		// no-output path still completes cleanly.
	}
	return nil
}

// ExampleRun shows the shape a frontend author's test assembles:
// the frontend under test, the source fixture it loads from, and
// whichever downstream plugins the test wants in the chain — then
// assertions against the store the run populated.
//
// The choice between [Run] and [LoadDirect] is the decision worth
// getting right. Run builds a full pipeline, so it enforces the
// pipeline's build invariants and reaches the layout and render
// phases; use it when the test cares about how the frontend's output
// travels through the rest of the chain. LoadDirect calls the
// frontend's Load surface and nothing else; use it when the test is
// about source mapping or parse diagnostics and the pipeline is
// noise.
//
// [Run] takes the enclosing test's `*testing.T` — it reads
// `t.Context()` for the run's cancellation scope — which an
// [Example] function is never given, so the body below is written as
// the helper a frontend author calls from their own
// `func TestMyFrontend(t *testing.T)` and is not invoked here. There
// is deliberately no `// Output:` block: without one Go compiles and
// type-checks the example without running it, which is exactly the
// check the package docblock's prose snippet cannot provide.
// Execution is covered by the TestRun_* cases above.
func ExampleRun() {
	assertLoadsFakePackage := func(t *testing.T) {
		t.Helper()

		// A real test passes its own frontend and its own testdata
		// directory — or frontendtest.DemoFixture(t), which resolves
		// only inside an eidos checkout. Whatever it names, Run stats
		// it: an empty temp directory stands in here because this
		// frontend synthesises its output from Pattern alone.
		result := frontendtest.Run(t, frontendtest.RunOptions{
			Frontend:  &fakeFrontend{name: "fake-fe"},
			SourceDir: t.TempDir(),
			Pattern:   "single",
			// Command pins the header line the backend would
			// otherwise derive from os.Args, which differs between a
			// plain `go test` and a run under coverage tooling.
			Command: "eidos gen ./...",
		})

		if result.RunErr != nil {
			t.Fatalf("run: %v (diagnostics: %+v)", result.RunErr, result.Diag.Diagnostics())
		}
		if _, ok := result.Store.Nodes().Structs().ByQName("example.com/fake.User"); !ok {
			t.Fatalf("frontend did not load the expected struct")
		}
	}

	// An Example has no *testing.T to hand over; see the docblock.
	_ = assertLoadsFakePackage
}
