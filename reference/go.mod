module go.thesmos.sh/eidos/reference

go 1.26.5

require (
	go.thesmos.sh/eidos v1.12.0
	go.thesmos.sh/eidos/backend/golang v1.9.0
	go.thesmos.sh/eidos/eidostest v1.8.0
)

replace (
	go.thesmos.sh/eidos => ../
	go.thesmos.sh/eidos/eidostest => ../eidostest
)
