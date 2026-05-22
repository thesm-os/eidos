module go.thesmos.sh/eidos/frontend/golang

go 1.26.2

require (
	go.thesmos.sh/eidos v1.3.0
	go.thesmos.sh/eidos/eidostest v1.3.0
	golang.org/x/tools v0.45.0
)

require (
	golang.org/x/mod v0.36.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
)

replace (
	go.thesmos.sh/eidos => ../../
	go.thesmos.sh/eidos/eidostest => ../../eidostest
)
