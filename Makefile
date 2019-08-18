all:
	go build examples/gonode.go

run:
	./gonode -cookie d3vc00k -listen 12321 -trace.node -trace.dist

epmd:
	go build cmd/epmd.go

clean:
	go clean
	$(RM) ./gonode

test: test-r test-sv
test-sv:
	go test -tags supervisor --trace.node --trace.dist
test-r:
	go test -tags registrar --trace.node --trace.dist
