rungs:
	go run --tags debug ./examples/genserver/demoGenServer.go -trace.node

epmd:
	go build cmd/epmd/epmd.go

test:
	go vet
	go clean -testcache
	go test -v ./...

cover:
	go test -coverprofile=cover.out ./...
	go tool cover -html=cover.out -o coverage.html
	rm cover.out

bench:
	go test -bench=Node -run=X -benchmem

migrate:
	find . -type f -exec sed -i 's/halturin/ergo-services/g' {} +

