// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package meta_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
)

// Bag tests share keys declared at package level. The global key
// registry is process-wide and survives test-count > 1, so per-test
// NewKey calls would panic on duplicate registration. Declaring
// each key once matches production usage (plugin authors do the same).
// The "bag." prefix segregates these names from the other test files.
var (
	keyBagHasBasic          = meta.NewKey("bag.has.basic", meta.BoolParser)
	keyBagTombstonedBasic   = meta.NewKey("bag.tombstoned.basic", meta.BoolParser)
	keyBagRawValueBasic     = meta.NewKey("bag.rawvalue.basic", meta.StringParser)
	keyBagWinningPrecedence = meta.NewKey("bag.winning.precedence", meta.IntParser)
	keyBagPrefixRoot        = meta.NewKey("bag.prefix.root", meta.BoolParser)
	keyBagPrefixRootMethod  = meta.NewKey("bag.prefix.root.method", meta.StringParser)
	keyBagHistoryBasic      = meta.NewKey("bag.history.basic", meta.BoolParser)
	keyBagNamesAlpha        = meta.NewKey("bag.names.alpha", meta.BoolParser)
	keyBagNamesBeta         = meta.NewKey("bag.names.beta", meta.BoolParser)

	// Four segments deep on purpose: a tombstone on bag.prefix.deep is
	// two levels above this key, so resolving it exercises the ancestor
	// walk rather than a single parent lookup. Registered keys in this
	// repo really do go that deep (proto.service.rpc.stream.server.member).
	keyBagPrefixDeepGrandchild = meta.NewKey("bag.prefix.deep.child.grandchild", meta.StringParser)
)

// Prefix-tombstone targets. These are plain names, not keys:
// [meta.Bag.TombstonePrefix] takes a raw string because
// pipeline/override.go routes `-gen:meta NAME` there exactly when NAME
// is *not* a registered key, and a suppression prefix is free to name
// an interior node of the dotted namespace that no key occupies.
const (
	nameBagPrefixDeep      = "bag.prefix.deep"
	nameBagPrefixDeepChild = "bag.prefix.deep.child"
)

func TestNewBag(t *testing.T) {
	t.Parallel()

	t.Run("returns an empty bag", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		if b == nil {
			t.Fatalf("NewBag returned nil")
		}
		if names := b.Names(); len(names) != 0 {
			t.Fatalf("new bag should have no names; got %v", names)
		}
	})
}

func TestBag_Has(t *testing.T) {
	t.Parallel()

	t.Run("returns false for an absent name", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		if b.Has(keyBagHasBasic.Name()) {
			t.Fatalf("Has should be false for an absent name")
		}
	})

	t.Run("returns true after a set", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		keyBagHasBasic.Set(b, true, "test")
		if !b.Has(keyBagHasBasic.Name()) {
			t.Fatalf("Has should be true after Set")
		}
	})

	t.Run("returns false when winning entry is a tombstone", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		keyBagHasBasic.Set(b, true, "test")
		keyBagHasBasic.Tombstone(b, "test")
		if b.Has(keyBagHasBasic.Name()) {
			t.Fatalf("Has should be false when winning entry is a tombstone")
		}
	})
}

func TestBag_Tombstoned(t *testing.T) {
	t.Parallel()

	t.Run("returns false for an absent name", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		if b.Tombstoned(keyBagTombstonedBasic.Name()) {
			t.Fatalf("Tombstoned should be false for an absent name")
		}
	})

	t.Run("returns true when winning entry is a tombstone", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		keyBagTombstonedBasic.Tombstone(b, "test")
		if !b.Tombstoned(keyBagTombstonedBasic.Name()) {
			t.Fatalf("Tombstoned should be true after Tombstone")
		}
	})

	t.Run("returns false when winning entry is a value", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		keyBagTombstonedBasic.Set(b, true, "test")
		if b.Tombstoned(keyBagTombstonedBasic.Name()) {
			t.Fatalf("Tombstoned should be false when value is set")
		}
	})
}

func TestBag_RawValue(t *testing.T) {
	t.Parallel()

	t.Run("returns false for an absent name", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		if _, ok := b.RawValue(keyBagRawValueBasic.Name()); ok {
			t.Fatalf("RawValue should be false for an absent name")
		}
	})

	t.Run("returns the set value as type-erased any", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		keyBagRawValueBasic.Set(b, "hello", "test")
		got, ok := b.RawValue(keyBagRawValueBasic.Name())
		if !ok {
			t.Fatalf("RawValue should return ok after Set")
		}
		if s, _ := got.(string); s != "hello" {
			t.Fatalf("RawValue = %v, want \"hello\"", got)
		}
	})
}

func TestBag_Winning(t *testing.T) {
	t.Parallel()

	t.Run("manual beats directive beats plugin", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		keyBagWinningPrecedence.Set(b, 1, "p")
		keyBagWinningPrecedence.SetDirective(b, 2, position.At("d.go", 1, 1))
		keyBagWinningPrecedence.SetManual(b, 3, "m")

		p, ok := b.Winning(keyBagWinningPrecedence.Name())
		if !ok {
			t.Fatalf("Winning should report ok")
		}
		if p.Authority != meta.AuthorityManual {
			t.Fatalf("authority = %v, want Manual", p.Authority)
		}
		if v, _ := p.Value.(int); v != 3 {
			t.Fatalf("value = %v, want 3", p.Value)
		}
	})

	t.Run("tombstone beats value within the same authority", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		keyBagWinningPrecedence.Set(b, 1, "p")
		keyBagWinningPrecedence.Tombstone(b, "p")
		p, ok := b.Winning(keyBagWinningPrecedence.Name())
		if !ok || !p.Tombstone {
			t.Fatalf("expected tombstone to win at AuthorityPlugin; got %+v ok=%v", p, ok)
		}
	})

	t.Run("higher-authority value beats lower-authority tombstone", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		keyBagWinningPrecedence.Tombstone(b, "p")
		keyBagWinningPrecedence.SetManual(b, 9, "m")
		p, ok := b.Winning(keyBagWinningPrecedence.Name())
		if !ok || p.Tombstone {
			t.Fatalf("manual value should beat plugin tombstone; got %+v", p)
		}
		if v, _ := p.Value.(int); v != 9 {
			t.Fatalf("value = %v, want 9", p.Value)
		}
	})

	t.Run("returns false when no entry exists", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		if _, ok := b.Winning("nonexistent-key-name"); ok {
			t.Fatalf("Winning should be false for an absent name")
		}
	})
}

func TestBag_PrefixTombstone(t *testing.T) {
	t.Parallel()

	t.Run("a directive prefix tombstone covers a plugin-set descendant", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		keyBagPrefixRootMethod.Set(b, "Write", "p")
		b.TombstonePrefix(keyBagPrefixRoot.Name(), meta.AuthorityDirective, "directive", position.At("d.go", 1, 1))

		p, ok := b.Winning(keyBagPrefixRootMethod.Name())
		if !ok || !p.Tombstone {
			t.Fatalf("descendant should be tombstoned via prefix; got %+v ok=%v", p, ok)
		}
		if p.CoveredBy != keyBagPrefixRoot.Name() {
			t.Fatalf("CoveredBy = %q, want %q", p.CoveredBy, keyBagPrefixRoot.Name())
		}
	})

	t.Run("descendant set later still sees the prefix tombstone", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		b.TombstonePrefix(keyBagPrefixRoot.Name(), meta.AuthorityDirective, "directive", position.At("d.go", 1, 1))
		keyBagPrefixRootMethod.Set(b, "Write", "p")
		if _, ok := keyBagPrefixRootMethod.Get(b); ok {
			t.Fatalf("Set after prefix tombstone should still resolve as tombstoned")
		}
	})

	t.Run("higher-authority descendant value beats lower-authority prefix tombstone", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		b.TombstonePrefix(keyBagPrefixRoot.Name(), meta.AuthorityPlugin, "p", position.Pos{})
		keyBagPrefixRootMethod.SetManual(b, "override", "m")
		got, ok := keyBagPrefixRootMethod.Get(b)
		if !ok || got != "override" {
			t.Fatalf("manual value should beat plugin prefix tombstone; got %q ok=%v", got, ok)
		}
	})

	// Pins the arm order inside entryAtAuthorityLocked: the prefix-
	// tombstone arm must sit above the direct-value arm. A mutant that
	// swaps them keeps every other subtest here green, because they all
	// split the value and the tombstone across *different* authorities,
	// where the authority walk in winningLocked picks a winner before
	// arm order can matter. Only a same-authority pair reaches the
	// switch with both a direct value and a covering tombstone — and
	// that is the primary production shape, not a corner: one file
	// carrying `-gen:meta bag.prefix.root` plus `+gen:meta
	// bag.prefix.root.method=Write` lands both at AuthorityDirective
	// (pipeline/override.go setMeta and tombstoneMeta).
	t.Run("a prefix tombstone beats a descendant value set at the same authority", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		pos := position.At("d.go", 1, 1)
		keyBagPrefixRootMethod.SetDirective(b, "Write", pos)
		b.TombstonePrefix(keyBagPrefixRoot.Name(), meta.AuthorityDirective, "directive", pos)

		p, ok := b.Winning(keyBagPrefixRootMethod.Name())
		if !ok {
			t.Fatalf("Winning should resolve the descendant; got ok=false")
		}
		if !p.Tombstone {
			t.Fatalf("prefix tombstone should beat the same-authority value; got Tombstone=false value=%v", p.Value)
		}
		if p.CoveredBy != keyBagPrefixRoot.Name() {
			t.Fatalf("CoveredBy = %q, want %q", p.CoveredBy, keyBagPrefixRoot.Name())
		}
		if leaked, isSet := keyBagPrefixRootMethod.Get(b); isSet {
			t.Fatalf("suppressed key still readable; Get = %q, want not-set", leaked)
		}
	})

	// Pins the ancestor *walk* in prefixTombstoneLocked. A mutant that
	// collapses the loop to a single check on the immediate parent
	// survives every other subtest here, because their fixture pair
	// (bag.prefix.root / bag.prefix.root.method) is exactly one level
	// apart. Suppression must cascade the whole dotted chain: real key
	// names in this repo run six segments deep, so `-gen:meta` on an
	// interior node has several levels to cross before it reaches them.
	t.Run("a prefix tombstone covers a descendant more than one level below it", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		keyBagPrefixDeepGrandchild.Set(b, "Write", "p")
		b.TombstonePrefix(nameBagPrefixDeep, meta.AuthorityDirective, "directive", position.At("d.go", 1, 1))

		p, ok := b.Winning(keyBagPrefixDeepGrandchild.Name())
		if !ok {
			t.Fatalf("Winning should resolve the grandchild via the ancestor walk; got ok=false")
		}
		if !p.Tombstone {
			t.Fatalf("walk must not stop at the immediate parent; got Tombstone=false value=%v", p.Value)
		}
		if p.CoveredBy != nameBagPrefixDeep {
			t.Fatalf("CoveredBy = %q, want %q", p.CoveredBy, nameBagPrefixDeep)
		}
	})

	// Pins first-match (nearest ancestor) over last-match (outermost)
	// in prefixTombstoneLocked. A mutant that keeps walking and returns
	// the farthest tombstoned ancestor still resolves the name as
	// suppressed, so only CoveredBy exposes it — and no other subtest
	// puts two tombstoned ancestors in the bag at once, which is the
	// sole configuration where nearest and farthest differ. CoveredBy
	// is what --explain prints, so a wrong answer here sends whoever is
	// hunting a vanished fact to the wrong `-gen:` directive.
	t.Run("a suppression is attributed to the closest tombstoned ancestor", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		pos := position.At("d.go", 1, 1)
		b.TombstonePrefix(nameBagPrefixDeep, meta.AuthorityDirective, "directive", pos)
		b.TombstonePrefix(nameBagPrefixDeepChild, meta.AuthorityDirective, "directive", pos)

		p, ok := b.Winning(keyBagPrefixDeepGrandchild.Name())
		if !ok || !p.Tombstone {
			t.Fatalf("grandchild should resolve as tombstoned; got %+v ok=%v", p, ok)
		}
		if p.CoveredBy != nameBagPrefixDeepChild {
			t.Fatalf("CoveredBy = %q, want the closest ancestor %q", p.CoveredBy, nameBagPrefixDeepChild)
		}
	})
}

func TestBag_History(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for an absent name", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		if got := b.History("nothing-here"); got != nil {
			t.Fatalf("History on absent name = %v, want nil", got)
		}
	})

	t.Run("returns entries in ascending authority order", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		// Write in non-ascending authority order to confirm sort.
		keyBagHistoryBasic.SetManual(b, true, "m")
		keyBagHistoryBasic.Set(b, false, "p")
		keyBagHistoryBasic.SetDirective(b, true, position.At("d.go", 1, 1))

		got := b.History(keyBagHistoryBasic.Name())
		if len(got) != 3 {
			t.Fatalf("History should record three entries; got %d", len(got))
		}
		want := []meta.Authority{meta.AuthorityPlugin, meta.AuthorityDirective, meta.AuthorityManual}
		for i, p := range got {
			if p.Authority != want[i] {
				t.Fatalf("History[%d].Authority = %v, want %v", i, p.Authority, want[i])
			}
		}
	})
}

func TestBag_Names(t *testing.T) {
	t.Parallel()

	t.Run("returns sorted names of every entry, including tombstones", func(t *testing.T) {
		t.Parallel()
		b := meta.NewBag()
		keyBagNamesBeta.Set(b, true, "p")
		keyBagNamesAlpha.Tombstone(b, "p")

		want := []string{keyBagNamesAlpha.Name(), keyBagNamesBeta.Name()}
		slices.Sort(want)
		got := b.Names()
		if !slices.Equal(got, want) {
			t.Fatalf("Names = %v, want %v", got, want)
		}
	})
}
