// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package main

import (
	"path/filepath"
	"runtime"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	frontendgolang "go.thesmos.sh/eidos/frontend/golang"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/streamconsumer"
	"go.thesmos.sh/eidos/store"
)

// TestShapeClassification_EndToEnd joins the layers the per-package
// suites cover separately: real Go source, through the frontend's
// type-ref stamping, into the detector catalog's dispatch.
//
// It earns its place through the string coupling. `streamconsumer`
// resolves `go.isInterface` through the meta registry by name rather
// than importing `frontend/golang`, which the plugins depguard rule
// denies. Its unit tests stamp that fact themselves, so they agree
// with a typo; only a run where the frontend does the stamping can
// prove the two halves use the same key.
//
// It lives here because this is the module that depends on both a
// frontend and the plugin catalog — the layering forbids the
// detector package from doing so.
func TestShapeClassification_EndToEnd(t *testing.T) {
	t.Parallel()

	shapes := classifyFixture(t)

	tests := []struct {
		method string
		want   string
		why    string
	}{
		{"Load", streamconsumer.Name, "io.Reader consumed, count returned"},
		{"Decode", streamconsumer.Name, "io.Reader consumed, document returned"},
		{"Get", "reader", "a string key is a key"},
		{"Put", "writer", "a value in, error only out"},
	}
	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			t.Parallel()
			if got := shapes[tc.method]; got != tc.want {
				t.Fatalf("%s classified as %q, want %q (%s)", tc.method, got, tc.want, tc.why)
			}
		})
	}
}

// TestShapeClassification_StreamTypeNotKeyType pins that a consumed
// stream never reaches `shape.key_type`, which is the claim the
// shape exists to prevent.
func TestShapeClassification_StreamTypeNotKeyType(t *testing.T) {
	t.Parallel()

	for _, m := range loadShapeFixture(t) {
		if m.Name != "Load" {
			continue
		}
		if got, ok := shape.MetaKeyType.Get(m.Meta()); ok && got != "" {
			t.Fatalf("Load carries shape.key_type = %q; a drained stream is not a key", got)
		}
		if got, _ := streamconsumer.MetaStreamType.Get(m.Meta()); got != "io.Reader" {
			t.Fatalf("Load carries shape.stream_type = %q, want io.Reader", got)
		}
		return
	}
	t.Fatalf("fixture has no method Load")
}

// TestShapeClassification_ReceiptWriteIsAKnownGap records the case
// no shape expresses: a write that returns a receipt.
//
// `writer` requires error as its only return, so this falls to
// `reader`, which labels the written document a key and the receipt
// a value — both types recorded, both under the wrong role. That is
// wrong, and deliberately left wrong: inventing the shape belongs
// with the catalog addition, not with the fix that exposed it.
//
// It is asserted rather than commented so the limitation cannot
// drift silently. When the shape lands, this test is what says so.
func TestShapeClassification_ReceiptWriteIsAKnownGap(t *testing.T) {
	t.Parallel()

	for _, m := range loadShapeFixture(t) {
		if m.Name != "Insert" {
			continue
		}
		if got := shape.Get(m.Meta()); got != "reader" {
			t.Fatalf("Insert(ctx, Doc) (ID, error) now classifies as %q; if a receipt-write "+
				"shape has landed, retire this test and assert the new stamp instead", got)
		}
		key, _ := shape.MetaKeyType.Get(m.Meta())
		val, _ := shape.MetaValueType.Get(m.Meta())
		if key != "example.com/streamsrc.Document" || val != "example.com/streamsrc.ID" {
			t.Fatalf("known-gap stamp moved: key_type=%q value_type=%q", key, val)
		}
		return
	}
	t.Fatalf("fixture has no method Insert")
}

// classifyFixture returns the stamped shape per fixture method.
func classifyFixture(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range loadShapeFixture(t) {
		out[m.Name] = shape.Get(m.Meta())
	}
	return out
}

// loadShapeFixture parses testdata/streamsrc through the Go frontend
// and runs the full detector catalog over the result.
func loadShapeFixture(t *testing.T) []*node.Method {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	srcDir := filepath.Join(filepath.Dir(thisFile), "testdata", "streamsrc")

	parser, err := directive.NewParser("gen")
	if err != nil {
		t.Fatalf("NewParser: %v", err)
	}
	front := frontendgolang.New()
	if err := front.SetOptions(opt.New(front.OptionsSchema(), map[string]string{
		"dir": srcDir,
		// The fixture is a self-contained module deliberately kept
		// out of go.work.
		"ignore_workspace": "true",
	})); err != nil {
		t.Fatalf("SetOptions: %v", err)
	}

	s := store.New()
	d := diag.New()
	if err := front.Load(&plugin.FrontendContext{
		Store:    s,
		Diag:     d,
		Registry: directive.NewRegistry(),
		Parser:   parser,
		Pattern:  "./...",
	}); err != nil {
		t.Fatalf("frontend Load: %v", err)
	}
	if d.HasErrors() {
		t.Fatalf("frontend diagnostics: %+v", d.Diagnostics())
	}

	if err := shape.New().Detectors(detectors.All()...).Annotate(&plugin.AnnotatorContext{
		Store: s, Reader: store.NewReader(s), Diag: d,
	}); err != nil {
		t.Fatalf("shape Annotate: %v", err)
	}
	return store.NewReader(s).Methods().Slice()
}
