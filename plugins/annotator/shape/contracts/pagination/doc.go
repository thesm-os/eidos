// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package pagination recognises the paginated-reader contract —
// a reader whose protocol cursors via a named field. The
// directive carries a `cursor=` param naming the field, resolved
// against the type the reader answers and stamped qualified — so a
// consumer can select the member rather than only read its name,
// which is what a continuation claim needs to fetch the second page
// with the cursor the first one gave it.
//
// The recognised directive is:
//
//	//+gen:contract pagination role=reader cursor=Cursor
package pagination
