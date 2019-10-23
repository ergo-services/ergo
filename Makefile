all:
	go build examples/gonode.go

run:
	./gonode -cookie d3vc00k -listen 12321 -trace.node -trace.dist

epmd:
	go build cmd/epmd.go

clean:
	go clean
	$(RM) ./gonode

test:
	go vet
	go test ./...

cover:
	go test -coverprofile=cover.out ./...
	go tool cover -html=cover.out -o coverage.html
	rm cover.out
