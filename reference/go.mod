module go.thesmos.sh/eidos/reference

go 1.26.5

require (
	go.thesmos.sh/eidos v1.6.0
	go.thesmos.sh/eidos/backend/golang v1.5.0
	go.thesmos.sh/eidos/eidostest v1.6.0
)

require (
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
)

replace (
	go.thesmos.sh/eidos => ../
	go.thesmos.sh/eidos/eidostest => ../eidostest
)
