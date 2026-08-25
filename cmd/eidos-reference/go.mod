module go.thesmos.sh/eidos/cmd/eidos-reference

go 1.26.5

require (
	go.thesmos.sh/eidos v1.14.1
	go.thesmos.sh/eidos/bridge/protogo v1.13.3
	go.thesmos.sh/eidos/cli v1.13.3
	go.thesmos.sh/eidos/eidostest v1.14.0
	go.thesmos.sh/eidos/lang/golang v1.14.1
	go.thesmos.sh/eidos/lang/protobuf v1.14.1
	go.thesmos.sh/eidos/plugins v1.14.0
	go.thesmos.sh/eidos/reference v1.13.3
)

require (
	github.com/bufbuild/protocompile v0.14.1 // indirect
	github.com/goccy/go-yaml v1.19.2 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (
	go.thesmos.sh/eidos => ../..
	go.thesmos.sh/eidos/bridge/protogo => ../../bridge/protogo
	go.thesmos.sh/eidos/cli => ../../cli
	go.thesmos.sh/eidos/eidostest => ../../eidostest
	go.thesmos.sh/eidos/lang/golang => ../../lang/golang
	go.thesmos.sh/eidos/lang/protobuf => ../../lang/protobuf
	go.thesmos.sh/eidos/plugins => ../../plugins
	go.thesmos.sh/eidos/reference => ../../reference
)
