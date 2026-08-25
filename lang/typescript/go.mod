module go.thesmos.sh/eidos/lang/typescript

go 1.26.5

require (
	github.com/tree-sitter/go-tree-sitter v0.25.0
	github.com/tree-sitter/tree-sitter-typescript v0.23.2
	go.thesmos.sh/eidos v1.15.1
	go.thesmos.sh/eidos/eidostest v1.15.1
)

require github.com/mattn/go-pointer v0.0.1 // indirect

replace (
	go.thesmos.sh/eidos => ../..
	go.thesmos.sh/eidos/eidostest => ../../eidostest
)
