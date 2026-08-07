module go.thesmos.sh/eidos/plugins

go 1.26.5

require (
	go.thesmos.sh/eidos v1.10.0
	go.thesmos.sh/eidos/eidostest v1.6.3
)

replace (
	go.thesmos.sh/eidos => ../
	go.thesmos.sh/eidos/eidostest => ../eidostest
)
