module go.thesmos.sh/eidos/backend/golang

go 1.26.5

require (
	go.thesmos.sh/eidos v1.6.0
	go.thesmos.sh/eidos/eidostest v1.6.0
	golang.org/x/tools v0.48.0
)

require (
	github.com/google/go-cmp v0.7.0 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
)

replace (
	go.thesmos.sh/eidos => ../../
	go.thesmos.sh/eidos/eidostest => ../../eidostest
)
