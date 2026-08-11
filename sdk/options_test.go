// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/sdk"
)

// wellFormedOptions is the shape a plugin is expected to bind: every
// field a supported kind, every tag option one the package defines.
type wellFormedOptions struct {
	Name  string `eidos:"name,required"`
	Count int    `eidos:"count,default=3"`
}

// unknownTagOptions carries a tag option the package does not define,
// which is the failure BindOptions turns into a panic.
type unknownTagOptions struct {
	Name string `eidos:"name,nonsense"`
}

// unsupportedTypeOptions carries a field the decoder cannot parse.
type unsupportedTypeOptions struct {
	Ratio float64 `eidos:"ratio"`
}

func TestOptionsSentinels(t *testing.T) {
	t.Parallel()

	t.Run("each sentinel is the opt sentinel it re-exports", func(t *testing.T) {
		t.Parallel()
		pairs := []struct {
			name string
			got  error
			want error
		}{
			{"ErrInvalidTag", sdk.ErrInvalidTag, opt.ErrInvalidTag},
			{"ErrUnsupportedFieldType", sdk.ErrUnsupportedFieldType, opt.ErrUnsupportedFieldType},
			{"ErrMissingRequired", sdk.ErrMissingRequired, opt.ErrMissingRequired},
			{"ErrUnknownField", sdk.ErrUnknownField, opt.ErrUnknownField},
			{"ErrInvalidValue", sdk.ErrInvalidValue, opt.ErrInvalidValue},
			{"ErrInvalidDecodeTarget", sdk.ErrInvalidDecodeTarget, opt.ErrInvalidDecodeTarget},
		}
		for _, pair := range pairs {
			if !errors.Is(pair.got, pair.want) {
				t.Errorf("sdk.%s does not match its opt sentinel", pair.name)
			}
		}
	})

	t.Run("a decode failure is not a schema failure", func(t *testing.T) {
		t.Parallel()
		// The two halves answer different questions — whether the
		// plugin author's struct is well formed, and whether the
		// caller's values fit it. A consumer branching on one must not
		// catch the other.
		if errors.Is(sdk.ErrUnknownField, sdk.ErrInvalidTag) {
			t.Error("ErrUnknownField must not match ErrInvalidTag")
		}
		if errors.Is(sdk.ErrMissingRequired, sdk.ErrInvalidValue) {
			t.Error("ErrMissingRequired must not match ErrInvalidValue")
		}
	})
}

func TestReflectOptions(t *testing.T) {
	t.Parallel()

	t.Run("a well-formed struct yields a field per option", func(t *testing.T) {
		t.Parallel()
		schema, err := sdk.ReflectOptions(wellFormedOptions{})
		if err != nil {
			t.Fatalf("well-formed options did not reflect: %v", err)
		}
		if len(schema.Fields) != 2 {
			t.Errorf("reflected %d fields, want 2", len(schema.Fields))
		}
	})

	t.Run("an unknown tag option is returned, not panicked", func(t *testing.T) {
		t.Parallel()
		// The whole reason this sits on the façade: BindOptions panics
		// here, which fails the suite before any subtest names the
		// field at fault.
		_, err := sdk.ReflectOptions(unknownTagOptions{})
		if !errors.Is(err, sdk.ErrInvalidTag) {
			t.Errorf("got %v, want an error wrapping ErrInvalidTag", err)
		}
	})

	t.Run("an unsupported field type is returned, not panicked", func(t *testing.T) {
		t.Parallel()
		_, err := sdk.ReflectOptions(unsupportedTypeOptions{})
		if !errors.Is(err, sdk.ErrUnsupportedFieldType) {
			t.Errorf("got %v, want an error wrapping ErrUnsupportedFieldType", err)
		}
	})
}

func TestSetOptionsSurfacesSentinels(t *testing.T) {
	t.Parallel()

	t.Run("an undeclared key reports ErrUnknownField", func(t *testing.T) {
		t.Parallel()
		var target wellFormedOptions
		holder := sdk.BindOptions(&target)
		err := holder.SetOptions(sdk.NewOptions(
			holder.OptionsSchema(),
			map[string]string{"name": "x", "typo": "1"},
		))
		if !errors.Is(err, sdk.ErrUnknownField) {
			t.Errorf("got %v, want an error wrapping ErrUnknownField", err)
		}
	})

	t.Run("an omitted required field reports ErrMissingRequired", func(t *testing.T) {
		t.Parallel()
		var target wellFormedOptions
		holder := sdk.BindOptions(&target)
		err := holder.SetOptions(sdk.NewOptions(
			holder.OptionsSchema(),
			map[string]string{},
		))
		if !errors.Is(err, sdk.ErrMissingRequired) {
			t.Errorf("got %v, want an error wrapping ErrMissingRequired", err)
		}
	})

	t.Run("a value outside the field's kind reports ErrInvalidValue", func(t *testing.T) {
		t.Parallel()
		var target wellFormedOptions
		holder := sdk.BindOptions(&target)
		err := holder.SetOptions(sdk.NewOptions(
			holder.OptionsSchema(),
			map[string]string{"name": "x", "count": "not-a-number"},
		))
		if !errors.Is(err, sdk.ErrInvalidValue) {
			t.Errorf("got %v, want an error wrapping ErrInvalidValue", err)
		}
	})
}
