// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import (
	"go.thesmos.sh/eidos/core/kind"
)

// Kind values for every concrete node type. Declared as
// [kind.Kind] so directive [Schema] AppliesTo lists reference
// them directly with full type safety:
//
//	directive.NewSchema("mock").On(node.KindInterface)
//
// The constants are the single source of truth for kind names —
// frontends populate [BaseNode.kind] (via the concrete type's Kind()
// method), the store indexes by these, and directive schemas
// constrain by them.
const (
	KindPackage     kind.Kind = "package"
	KindFile        kind.Kind = "file"
	KindImport      kind.Kind = "import"
	KindStruct      kind.Kind = "struct"
	KindInterface   kind.Kind = "interface"
	KindMethod      kind.Kind = "method"
	KindField       kind.Kind = "field"
	KindFunction    kind.Kind = "function"
	KindVariable    kind.Kind = "variable"
	KindConstant    kind.Kind = "constant"
	KindEnum        kind.Kind = "enum"
	KindEnumVariant kind.Kind = "enum_variant"
	KindAlias       kind.Kind = "alias"
	KindEmbed       kind.Kind = "embed"
	KindTypeRef     kind.Kind = "type_ref"
	KindParam       kind.Kind = "param"
	KindReturn      kind.Kind = "return"
	KindTypeParam   kind.Kind = "type_param"
)
