// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises iter.Seq / iter.Seq2 detection on
// function return types — the converter stamps go.isIterSeq /
// go.isIterSeq2 plus go.iterKeyType / go.iterValueType.
package fixture

import "iter"

// All yields every element of items. Drives the go.isIterSeq
// stamping path with a primitive element type.
func All(items []string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, item := range items {
			if !yield(item) {
				return
			}
		}
	}
}

// Pairs yields each (index, value) pair from items. Drives the
// go.isIterSeq2 + go.iterKeyType + go.iterValueType stamping path.
func Pairs(items []string) iter.Seq2[int, string] {
	return func(yield func(int, string) bool) {
		for i, item := range items {
			if !yield(i, item) {
				return
			}
		}
	}
}
