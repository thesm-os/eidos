module go.thesmos.sh/eidos/reference

go 1.26.5

require (
	go.thesmos.sh/eidos v1.13.1
	go.thesmos.sh/eidos/backend/golang v0.13.1
	go.thesmos.sh/eidos/eidostest v0.13.1
)

replace (
	go.thesmos.sh/eidos => ../
	go.thesmos.sh/eidos/eidostest => ../eidostest
)
