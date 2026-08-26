// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import "errors"

// ErrTemplateMissing reports a dispatch for an emit kind no template
// is registered for.
//
// The kind names itself in the wrapped message, so a plugin that
// shipped an emit kind without its template learns which one from the
// diagnostic rather than from a stack trace.
var ErrTemplateMissing = errors.New("backend: no template registered for kind")
