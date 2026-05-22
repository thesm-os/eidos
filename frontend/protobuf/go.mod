module go.thesmos.sh/eidos/frontend/protobuf

go 1.26.2

require (
	github.com/bufbuild/protocompile v0.14.1
	go.thesmos.sh/eidos v1.3.0
	go.thesmos.sh/eidos/eidostest v1.3.0
	google.golang.org/protobuf v1.34.2
)

require golang.org/x/sync v0.20.0 // indirect

replace (
	go.thesmos.sh/eidos => ../../
	go.thesmos.sh/eidos/eidostest => ../../eidostest
)
