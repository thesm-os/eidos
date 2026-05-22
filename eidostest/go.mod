module go.thesmos.sh/eidos/eidostest

go 1.26.2

require go.thesmos.sh/eidos v1.3.0

replace (
	go.thesmos.sh/eidos => ../
	go.thesmos.sh/eidos/backend/golang => ../backend/golang
	go.thesmos.sh/eidos/frontend/golang => ../frontend/golang
)
