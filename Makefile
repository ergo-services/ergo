all:
	go build examples/gonode.go

run:
	./gonode -cookie d3vc00k -epmd_port 12321 -erlang.node.trace -erlang.dist.trace

clean:
	(cd ../../../ && ${MAKE} $@)
