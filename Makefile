all:
	go build examples/gonode.go

run:
	./gonode -cookie d3vc00k -epmd_port 12321 -trace.node -trace.dist

clean:
	go clean
	$(RM) ./gonode
