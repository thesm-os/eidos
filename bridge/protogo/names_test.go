// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protogo_test

import (
	"testing"

	"go.thesmos.sh/eidos/bridge/protogo"
)

// TestGoFieldName_Initialisms covers the initialism preservation
// rule [protogo.GoFieldName] applies. Each row asserts one
// snake_case → PascalCase translation under a documented
// initialism segment.
func TestGoFieldName_Initialisms(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"user_id", "UserID"},
		{"http_status", "HTTPStatus"},
		{"api_key", "APIKey"},
		{"user_uuid", "UserUUID"},
		{"json_payload", "JSONPayload"},
		{"http_url", "HTTPURL"},
		{"id", "ID"},
		{"created_at", "CreatedAt"},
		{"", ""},
		// Degenerate separators. protoc accepts these field names,
		// so the converter has to drop the empty segments rather
		// than emit an empty PascalCase run for each one.
		{"user__id", "UserID"},
		{"_user_id", "UserID"},
		{"user_id_", "UserID"},
		{"_", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in+" → "+tc.want, func(t *testing.T) {
			t.Parallel()
			if got := protogo.GoFieldName(tc.in); got != tc.want {
				t.Fatalf("GoFieldName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestGoPackageName covers the three resolution paths
// [protogo.GoPackageName] supports: semicolon-suffix (the
// proto3-spec form), slash-form (path/to/pkg → pkg), and
// dot-form (dotted proto qualifier → last segment).
func TestGoPackageName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"github.com/example/foo;namedfoo", "namedfoo"},
		{"github.com/example/foo", "foo"},
		{"acme.cfg.bar", "bar"},
		{"plain", "plain"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in+" → "+tc.want, func(t *testing.T) {
			t.Parallel()
			if got := protogo.GoPackageName(tc.in); got != tc.want {
				t.Fatalf("GoPackageName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestGoImportPath covers the semicolon-suffix trim rule
// [protogo.GoImportPath] implements. The non-semicolon path
// returns the value unchanged.
func TestGoImportPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in, want string
	}{
		{"github.com/example/foo;namedfoo", "github.com/example/foo"},
		{"github.com/example/foo", "github.com/example/foo"},
		{"", ""},
	}
	for _, tc := range cases {
		t.Run(tc.in+" → "+tc.want, func(t *testing.T) {
			t.Parallel()
			if got := protogo.GoImportPath(tc.in); got != tc.want {
				t.Fatalf("GoImportPath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
