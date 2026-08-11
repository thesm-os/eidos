module go.thesmos.sh/eidos/reference

go 1.26.5

require (
	go.thesmos.sh/eidos v1.14.0
	go.thesmos.sh/eidos/backend/golang v1.13.3
	go.thesmos.sh/eidos/eidostest v1.14.0
)

replace (
	go.thesmos.sh/eidos => ../
	go.thesmos.sh/eidos/eidostest => ../eidostest
)
