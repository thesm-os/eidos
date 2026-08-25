// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package frontend converts TypeScript source into the
// language-agnostic [node] model.
//
// Language-specific facts ride on metadata keys in the `ts.*`
// namespace rather than on first-class node fields, keeping [node]
// and [emit] portable. The vocabulary is declared in
// `lang/typescript`, not here, because a plugin may not import a
// frontend — see that package for the catalogue and the typed
// accessors.
//
// # Parsing
//
// Parsing is tree-sitter, through the `tree-sitter-typescript`
// grammar. This is the only package in the module that links C, and
// the only one that cannot be built with `CGO_ENABLED=0`.
//
// Two grammars ship and the file extension picks between them:
// `<T>value` is a type assertion in `.ts` and the start of a JSX
// element in `.tsx`, so one grammar for both would mis-parse one of
// them.
//
// # What a syntax tree cannot answer
//
// tree-sitter is a parser, not a type checker. It resolves nothing
// across files, so three questions the Go frontend answers have no
// counterpart here:
//
//   - Type satisfaction. There is no `ts.isStringer`, because whether
//     a type satisfies an interface is not visible in syntax.
//   - Cross-file type identity. A type imported from `node_modules`
//     is a name and a module specifier. Within the parsed set the
//     converter resolves specifier plus name to a declaration;
//     outside it, it does not.
//   - Inferred types. `const x = compute()` carries no annotation, so
//     [node.Constant.Type] is nil.
//
// None of this obstructs generation, which reads declarations rather
// than inferring them. It does mean the `ts.*` vocabulary is
// structural where `go.*` is semantic.
//
// # Declaration mapping
//
// `interface` converts to [node.Interface] and `class` to
// [node.Struct] — different kinds because they are different things,
// one a contract and the other instantiable. Both carry fields, which
// is why [node.Interface] has a field list: most TypeScript
// interfaces declare no methods at all. See ADR-0008.
//
// Namespaces have no model kind and are flattened, with the dotted
// path recorded on `ts.namespace`. Overload signatures are not
// separate declarations — they collapse onto the implementation via
// `ts.overloads`, because the model keys a declaration by name and
// several of one name is a duplicate the store rejects.
package frontend
