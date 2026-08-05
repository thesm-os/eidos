// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Disk is a filesystem-backed [Cache] rooted at a configurable
// directory. Entries are stored under "<root>/<bucket>/<name>",
// where both segments derive from the hex SHA-256 of the key
// ("<h[:2]>/<h>"), with a deterministic ".eidos.tmp" suffix used
// during writes for atomic temp+rename. Bucketing on the digest
// rather than on the key spreads entries across 256 directories
// whatever shape the caller's keys take, and keeps every path
// segment hex so the layout stays portable across filesystems.
// See [Disk.keyPath] for why the key is not used verbatim.
//
// Keys are opaque to Disk: it imposes no structure on them and
// reads none.
//
// Concurrent writers to the same key are serialised through a
// per-instance mutex; entries are content-addressed so concurrent
// Puts converge on the same value regardless of the order they hit
// the disk.
type Disk struct {
	root string
	mu   sync.Mutex
}

// tempSuffix is appended to a key path while a Put is in progress.
// The same value is reused across writes; the per-instance mutex
// guarantees only one write per key is in flight at a time.
const tempSuffix = ".eidos.tmp"

// NewDisk returns a Disk cache rooted at root. The directory is
// created lazily on the first [Disk.Put]; callers do not need to
// ensure it exists.
func NewDisk(root string) *Disk {
	return &Disk{root: root}
}

// Root returns the configured root directory.
func (d *Disk) Root() string { return d.root }

// Get returns the bytes cached under key. Missing entries return
// (nil, false); read errors other than "not found" return (nil,
// false) as well — the caller treats a missing cache entry the same
// as one that failed to read, falling back to a fresh recompute.
//
// An empty key is a programmer error and returns (nil, false) — the
// non-error miss matches the spirit of the interface, where Get is
// not allowed to surface non-existence as an error.
func (d *Disk) Get(key string) ([]byte, bool) {
	if key == "" {
		return nil, false
	}
	body, err := os.ReadFile(d.keyPath(key))
	if err != nil {
		return nil, false
	}
	return body, true
}

// Put stores value under key atomically via temp+rename in the
// key's directory. Returns [ErrInvalidKey] when key is empty;
// filesystem errors propagate wrapped with the destination path
// for diagnostics.
func (d *Disk) Put(key string, value []byte) error {
	if key == "" {
		return fmt.Errorf("%w: empty", ErrInvalidKey)
	}
	full := d.keyPath(key)
	dir := filepath.Dir(full)
	tmpPath := full + tempSuffix

	d.mu.Lock()
	defer d.mu.Unlock()

	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("cache: mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(tmpPath, value, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cache: write %s: %w", tmpPath, err)
	}
	if err := os.Rename(tmpPath, full); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cache: rename %s -> %s: %w", tmpPath, full, err)
	}
	return nil
}

// Has reports whether key is present without reading the value.
// Useful for tooling (eidos explain, --verify-cache) that needs to
// enumerate the cache state without paying the read cost.
func (d *Disk) Has(key string) bool {
	if key == "" {
		return false
	}
	_, err := os.Stat(d.keyPath(key))
	return err == nil
}

// keyPath returns the filesystem path entries for key are stored
// under: "<root>/<h[:2]>/<h>", where h is the hex SHA-256 of the
// key. The key itself never reaches the filesystem.
//
// Hashing rather than using the key verbatim is what makes the
// layout correct for the keys [NewKey] actually produces. Those
// begin with a literal "plugin" segment, so bucketing on the key's
// own first two bytes put every entry in a single "pl" directory —
// the split existed but sharded nothing. The digest distributes
// across 256 buckets regardless of what the caller composed.
//
// It also keeps the path portable. Keys carry ":" separators and
// run past 200 characters; ":" is not a legal filename character
// on NTFS, and long keys push a project-rooted path toward
// Windows' default MAX_PATH. Both path segments here are hex, and
// the filename is a fixed 64 characters.
//
// The digest is a filesystem-layout detail, not a content hash:
// callers never see it, and no cache semantics depend on it.
func (d *Disk) keyPath(key string) string {
	h := HashBytes([]byte(key))
	return filepath.Join(d.root, h[:2], h)
}
