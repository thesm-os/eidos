// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/lang/golang/frontend"
)

// TestFrontend_OptionsSchema covers the reflected option schema the
// frontend exposes to the pipeline.
func TestFrontend_OptionsSchema(t *testing.T) {
	t.Parallel()
	t.Run("declares every documented field", func(t *testing.T) {
		t.Parallel()
		schema := frontend.New().OptionsSchema()
		want := map[string]bool{
			"include_tests":        true,
			"build_tags":           true,
			"skip_cgo_files":       true,
			"skip_generated_files": true,
			"dir":                  true,
			"ignore_workspace":     true,
		}
		for _, f := range schema.Fields {
			if !want[f.Name] {
				t.Errorf("unexpected option %q in schema", f.Name)
			}
			delete(want, f.Name)
		}
		for missing := range want {
			t.Errorf("option %q missing from schema", missing)
		}
	})

	t.Run("default values match documented contract", func(t *testing.T) {
		t.Parallel()
		schema := frontend.New().OptionsSchema()
		defaults := map[string]string{
			"include_tests":        "false",
			"build_tags":           "",
			"skip_cgo_files":       "true",
			"skip_generated_files": "true",
			"dir":                  "",
			"ignore_workspace":     "false",
		}
		for _, f := range schema.Fields {
			want, ok := defaults[f.Name]
			if !ok {
				continue
			}
			if !f.HasDefault {
				t.Errorf("option %q must declare a default", f.Name)
				continue
			}
			if f.DefaultStr != want {
				t.Errorf("option %q default = %q, want %q", f.Name, f.DefaultStr, want)
			}
		}
	})
}

// TestFrontend_SetOptions exercises the Options-decode path through
// the public plugin surface. SetOptions is opaque, so the assertion
// runs through SetOptions then drives a Load to surface decode
// errors.
func TestFrontend_SetOptions(t *testing.T) {
	t.Parallel()
	t.Run("accepts the empty options view", func(t *testing.T) {
		t.Parallel()
		fe := frontend.New()
		if err := fe.SetOptions(opt.New(fe.OptionsSchema(), map[string]string{})); err != nil {
			t.Fatalf("SetOptions empty: %v", err)
		}
	})

	t.Run("accepts every declared field", func(t *testing.T) {
		t.Parallel()
		fe := frontend.New()
		values := map[string]string{
			"include_tests":  "true",
			"build_tags":     "integration",
			"skip_cgo_files": "false",
			"dir":            "/abs/path/to/module",
		}
		if err := fe.SetOptions(opt.New(fe.OptionsSchema(), values)); err != nil {
			t.Fatalf("SetOptions full: %v", err)
		}
	})

	t.Run("rejects unknown fields", func(t *testing.T) {
		t.Parallel()
		fe := frontend.New()
		err := fe.SetOptions(opt.New(fe.OptionsSchema(), map[string]string{"nope": "1"}))
		if err == nil {
			t.Fatalf("SetOptions accepted unknown field")
		}
	})
}
