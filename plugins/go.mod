module go.thesmos.sh/eidos/plugins

go 1.26.5

require (
	go.thesmos.sh/eidos v1.3.3
	go.thesmos.sh/eidos/eidostest v1.3.2
)

replace (
	go.thesmos.sh/eidos => ../
	go.thesmos.sh/eidos/eidostest => ../eidostest
)
