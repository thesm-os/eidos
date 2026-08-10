module go.thesmos.sh/eidos/bridge/protogo

go 1.26.5

require (
	go.thesmos.sh/eidos v1.13.1
	go.thesmos.sh/eidos/eidostest v1.13.1
	go.thesmos.sh/eidos/frontend/golang v1.13.1
	go.thesmos.sh/eidos/frontend/protobuf v1.13.1
)

require (
	github.com/bufbuild/protocompile v0.14.1 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (
	go.thesmos.sh/eidos => ../../
	go.thesmos.sh/eidos/eidostest => ../../eidostest
	go.thesmos.sh/eidos/frontend/golang => ../../frontend/golang
	go.thesmos.sh/eidos/frontend/protobuf => ../../frontend/protobuf
)
