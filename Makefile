all:
	go build examples/gonode.go

run:
	./gonode -cookie d3vc00k -listen 12321 -trace.node -trace.dist

epmd:
	go build cmd/epmd.go

clean:
	go clean
	$(RM) ./gonode

test: test-n test-r test-sv
test-n:
	go test -v -tags node --trace.node --trace.dist
test-sv:
	go test -v -tags supervisor --trace.node --trace.dist
test-r:
	go test -v -tags registrar --trace.node --trace.dist
