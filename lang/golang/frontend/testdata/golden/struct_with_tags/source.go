// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package fixture exercises struct-tag stamping under go.tag.*.
package fixture

// Record carries serialisation-relevant tags on every field.
type Record struct {
	ID   string `json:"id"   db:"id_col"`
	Name string `json:"name" yaml:"display"`
}
