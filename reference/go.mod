module go.thesmos.sh/eidos/reference

go 1.26.5

require (
	go.thesmos.sh/eidos v1.15.1
	go.thesmos.sh/eidos/eidostest v1.15.1
	go.thesmos.sh/eidos/lang/golang v1.15.1
)

replace (
	go.thesmos.sh/eidos => ..
	go.thesmos.sh/eidos/eidostest => ../eidostest
	go.thesmos.sh/eidos/lang/golang => ../lang/golang
)
