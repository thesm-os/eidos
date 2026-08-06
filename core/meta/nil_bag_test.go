// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package meta_test

import (
	"reflect"
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
)

// readMethods are the exported [meta.Bag] methods that must tolerate
// a nil receiver, each mapped to arguments that exercise it. Any
// exported method absent from this map is a build-time-invisible gap,
// which is what [TestNilBag_EveryExportedMethodIsClassified] closes.
var readMethods = map[string][]reflect.Value{
	"Has":         {reflect.ValueOf("name")},
	"Tombstoned":  {reflect.ValueOf("name")},
	"RawValue":    {reflect.ValueOf("name")},
	"Winning":     {reflect.ValueOf("name")},
	"History":     {reflect.ValueOf("name")},
	"Names":       nil,
	"MarshalJSON": nil,
	"AddObserver": nil, // classified below as a writer
}

// writeMethods are the exported [meta.Bag] methods that must panic on
// a nil receiver — the signal that a caller took a write path without
// asking its owner for a bag via EnsureMeta.
var writeMethods = map[string][]reflect.Value{
	"TombstonePrefix": {
		reflect.ValueOf("prefix"),
		reflect.ValueOf(meta.AuthorityPlugin),
		reflect.ValueOf("test"),
		reflect.ValueOf(position.Pos{}),
	},
	"AddObserver":   {reflect.ValueOf(meta.Observer(func(string) {}))},
	"UnmarshalJSON": {reflect.ValueOf([]byte(`{"entries":[]}`))},
}

// TestNilBag_ReadsAreEmpty covers the contract that makes the nil
// bag publishable: every read answers what an empty bag would.
//
// Without it, moving the allocation out of the Meta accessors would
// relocate the defect into every caller — an accessor documented as
// a read would start nil-panicking instead of over-allocating, which
// is a worse failure than the one being fixed.
func TestNilBag_ReadsAreEmpty(t *testing.T) {
	t.Parallel()

	var b *meta.Bag

	t.Run("Has reports nothing set", func(t *testing.T) {
		t.Parallel()
		if b.Has("name") {
			t.Fatalf("nil bag reported a value")
		}
	})

	t.Run("Tombstoned reports no tombstone", func(t *testing.T) {
		t.Parallel()
		if b.Tombstoned("name") {
			t.Fatalf("nil bag reported a tombstone")
		}
	})

	t.Run("RawValue reports unset", func(t *testing.T) {
		t.Parallel()
		if v, ok := b.RawValue("name"); ok || v != nil {
			t.Fatalf("nil bag returned (%v, %v)", v, ok)
		}
	})

	t.Run("Winning reports no entry", func(t *testing.T) {
		t.Parallel()
		if p, ok := b.Winning("name"); ok || p != (meta.Provenance{}) {
			t.Fatalf("nil bag returned (%+v, %v)", p, ok)
		}
	})

	t.Run("History is empty", func(t *testing.T) {
		t.Parallel()
		if got := b.History("name"); len(got) != 0 {
			t.Fatalf("nil bag returned history %+v", got)
		}
	})

	t.Run("Names is empty", func(t *testing.T) {
		t.Parallel()
		if got := b.Names(); len(got) != 0 {
			t.Fatalf("nil bag returned names %v", got)
		}
	})

	t.Run("MarshalJSON encodes null", func(t *testing.T) {
		t.Parallel()
		// Found by the classification guard below rather than by
		// hand: it is a read method, it took the lock unguarded, and
		// it is reachable from anyone marshalling a bag straight
		// from the accessor.
		got, err := b.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON on a nil bag: %v", err)
		}
		if string(got) != "null" {
			t.Fatalf("nil bag encoded as %s, want null", got)
		}
	})
}

// TestNilBag_EveryExportedMethodIsClassified is the guard that keeps
// nil-tolerance from rotting.
//
// Nil-tolerance implemented as a hand-written list of guarded methods
// is a snapshot: the day someone adds an exported read method without
// a guard, a documented read path nil-derefs, and nothing fails until
// it reaches a user. Reflecting over the type instead means a new
// method is covered the day it lands — it either behaves like the
// empty bag or panics, and the author has to say which.
func TestNilBag_EveryExportedMethodIsClassified(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[*meta.Bag]()
	for m := range typ.Methods() {
		name := m.Name
		_, isRead := readMethods[name]
		_, isWrite := writeMethods[name]
		if !isRead && !isWrite {
			t.Fatalf("exported *Bag method %q is classified neither read nor write; "+
				"add it to readMethods (and give it a nil guard) or to writeMethods", name)
		}
	}
}

// TestNilBag_ReadsDoNotPanic drives every classified read through
// reflection, so the behaviour is asserted for the whole set rather
// than only the six spelled out above.
func TestNilBag_ReadsDoNotPanic(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[*meta.Bag]()
	recv := reflect.ValueOf((*meta.Bag)(nil))
	for name, args := range readMethods {
		if _, isWrite := writeMethods[name]; isWrite {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, ok := typ.MethodByName(name)
			if !ok {
				t.Fatalf("no exported method %q on *Bag; readMethods is stale", name)
			}
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("read method %q panicked on a nil bag: %v", name, r)
				}
			}()
			m.Func.Call(append([]reflect.Value{recv}, args...))
		})
	}
}

// TestNilBag_WritesPanic pins the other half. A panic here is the
// design working: it names a caller that reached for a write without
// EnsureMeta, and on the plugin boundary diag.RecoverAs turns it into
// an attributed diagnostic rather than a crash.
func TestNilBag_WritesPanic(t *testing.T) {
	t.Parallel()

	typ := reflect.TypeFor[*meta.Bag]()
	recv := reflect.ValueOf((*meta.Bag)(nil))
	for name, args := range writeMethods {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			m, ok := typ.MethodByName(name)
			if !ok {
				t.Fatalf("no exported method %q on *Bag; writeMethods is stale", name)
			}
			defer func() {
				if recover() == nil {
					t.Fatalf("write method %q did not panic on a nil bag", name)
				}
			}()
			m.Func.Call(append([]reflect.Value{recv}, args...))
		})
	}
}

// TestNilBag_TypedKeyReads covers the [meta.Key] surface, which is
// how plugins actually reach a bag. Key's read methods delegate to
// Bag's, so this asserts the delegation holds rather than re-testing
// the guards.
func TestNilBag_TypedKeyReads(t *testing.T) {
	t.Parallel()

	key := meta.EnsureKey("nilbag.test.flag", meta.BoolParser)

	t.Run("Get reports unset", func(t *testing.T) {
		t.Parallel()
		if v, ok := key.Get(nil); ok || v {
			t.Fatalf("Get on a nil bag returned (%v, %v)", v, ok)
		}
	})

	t.Run("Has reports unset", func(t *testing.T) {
		t.Parallel()
		if key.Has(nil) {
			t.Fatalf("Has on a nil bag reported set")
		}
	})

	t.Run("Set panics", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if recover() == nil {
				t.Fatalf("Set through a nil bag did not panic")
			}
		}()
		key.Set(nil, true, "test")
	})
}
