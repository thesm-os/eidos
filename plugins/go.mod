module go.thesmos.sh/eidos/plugins

go 1.26.2

require (
	go.thesmos.sh/eidos v1.3.0
	go.thesmos.sh/eidos/eidostest v1.3.0
)

replace (
	go.thesmos.sh/eidos => ../
	go.thesmos.sh/eidos/eidostest => ../eidostest
)
