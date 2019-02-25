all:
	go build examples/gonode.go

run:
	./gonode -cookie d3vc00k -listen 12321 -trace.node -trace.dist

epmd:
	go build cmd/epmd.go

clean:
	go clean
	$(RM) ./gonode
