module go.thesmos.sh/eidos/eidostest

go 1.26.5

require go.thesmos.sh/eidos v1.9.0

replace (
	go.thesmos.sh/eidos => ../
	go.thesmos.sh/eidos/backend/golang => ../backend/golang
	go.thesmos.sh/eidos/frontend/golang => ../frontend/golang
)
