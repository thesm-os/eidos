// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipelinetest

import (
	"bytes"
	"testing"

	"go.thesmos.sh/eidos/emit"
)

// FileAssertion accumulates per-file expectations against a captured
// rendered file. Each assertion failure calls t.Errorf so chained
// expectations all report; tests that need stop-on-first-failure
// semantics insert a [testing.TB.FailNow] between assertions.
//
// FileAssertion values are produced by [Pipeline.AssertFile] /
// [Pipeline.AssertFileInDir] — never constructed directly.
type FileAssertion struct {
	t      testing.TB
	target emit.Target
	body   []byte
}

// Target returns the [emit.Target] the file was rendered against.
// Used by tests that want to assert on routing metadata in addition
// to content.
func (a *FileAssertion) Target() emit.Target { return a.target }

// path names the file for a failure message.
//
// [emit.Target.JoinPath] returns "" whenever either component is
// empty, and a routed target with no directory is ordinary — a
// centralised layout with no configured Dir, or an [emit.File]
// carrying an explicit Target that pins only the filename, both
// produce one. Interpolating JoinPath alone printed
// `testpipe: file  does not contain …` for every one of them, in
// the package whose entire job is to say what went wrong. The
// filename is the identity [Pipeline.AssertFile] selects on, so it
// is the honest fallback.
func (a *FileAssertion) path() string {
	if p := a.target.JoinPath(); p != "" {
		return p
	}
	return a.target.Filename
}

// Bytes returns a copy of the file's rendered content. Mutations on
// the returned slice do not affect the assertion's state or later
// chained calls.
func (a *FileAssertion) Bytes() []byte {
	dup := make([]byte, len(a.body))
	copy(dup, a.body)
	return dup
}

// String returns the file's content as a string. Convenient for
// debugging tests via Logf("%s", a.String()).
func (a *FileAssertion) String() string { return string(a.body) }

// Contains fails the test (without stopping) when substr does not
// appear in the file's content.
func (a *FileAssertion) Contains(substr string) *FileAssertion {
	a.t.Helper()
	if !bytes.Contains(a.body, []byte(substr)) {
		a.t.Errorf("testpipe: file %s does not contain %q\n----- file body -----\n%s\n----- end -----",
			a.path(), substr, a.body)
	}
	return a
}

// NotContains fails the test (without stopping) when substr appears
// in the file's content.
func (a *FileAssertion) NotContains(substr string) *FileAssertion {
	a.t.Helper()
	if bytes.Contains(a.body, []byte(substr)) {
		a.t.Errorf("testpipe: file %s unexpectedly contains %q\n----- file body -----\n%s\n----- end -----",
			a.path(), substr, a.body)
	}
	return a
}

// Equals fails the test (without stopping) when the file's content
// is not byte-identical to expected. The failure message names the
// captured target's path so the user can identify the file at a
// glance.
func (a *FileAssertion) Equals(expected string) *FileAssertion {
	a.t.Helper()
	if string(a.body) != expected {
		a.t.Errorf(
			"testpipe: file %s did not match\n----- got -----\n%s\n----- want -----\n%s\n----- end -----",
			a.path(), a.body, expected,
		)
	}
	return a
}

// EqualsBytes is the bytes-typed companion to [FileAssertion.Equals].
func (a *FileAssertion) EqualsBytes(expected []byte) *FileAssertion {
	a.t.Helper()
	if !bytes.Equal(a.body, expected) {
		a.t.Errorf(
			"testpipe: file %s did not match\n----- got -----\n%s\n----- want -----\n%s\n----- end -----",
			a.path(), a.body, expected,
		)
	}
	return a
}
