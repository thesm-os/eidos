module go.thesmos.sh/eidos/lang/protobuf

go 1.26.5

require (
	github.com/bufbuild/protocompile v0.14.1
	go.thesmos.sh/eidos v1.14.1
	go.thesmos.sh/eidos/eidostest v1.14.0
	google.golang.org/protobuf v1.36.11
)

require golang.org/x/sync v0.22.0 // indirect

replace (
	go.thesmos.sh/eidos => ../..
	go.thesmos.sh/eidos/eidostest => ../../eidostest
	go.thesmos.sh/eidos/lang/golang => ../golang
)
