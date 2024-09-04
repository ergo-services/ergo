package distributed

import (
	"testing"
	"time"

	"ergo.services/ergo"
	"ergo.services/ergo/gen"
)

// TODO test static route

func TestT0NodeBasic(t *testing.T) {
	options1 := gen.NodeOptions{}
	options1.Network.Cookie = "123"
	options1.Network.MaxMessageSize = 567
	options1.Log.DefaultLogger.Disable = true
	// options1.Log.Level = gen.LogLevelTrace
	node1, err := ergo.StartNode("distT0node1basic@localhost", options1)
	if err != nil {
		t.Fatal(err)
	}
	defer node1.Stop()

	if _, err := node1.Network().GetNode("unknown@node"); err != gen.ErrNoRoute {
		t.Fatal("must be gen.ErrNoRoute here")
	}

	options2 := gen.NodeOptions{}
	options2.Network.Cookie = "123"
	options2.Network.MaxMessageSize = 765
	options2.Log.DefaultLogger.Disable = true
	node2, err := ergo.StartNode("distT0node2basic@localhost", options2)
	if err != nil {
		t.Fatal(err)
	}
	defer node2.Stop()

	// make connection to the node2
	remote1, err := node1.Network().GetNode(node2.Name())
	if err != nil {
		t.Fatal(err)
	}

	// TODO rework it
	// race condition. should wait a bit until
	//      new connection got registered on the node2.
	time.Sleep(100 * time.Millisecond)

	// take the existing connection with node1
	remote2, err := node2.Network().Node(node1.Name())
	if err != nil {
		t.Fatal(err)
	}

	remote1Info := remote1.Info()
	remote2Info := remote2.Info()

	if remote1Info.Node != node2.Name() {
		t.Fatal("incorrect remote1 node name")
	}
	if remote2Info.Node != node1.Name() {
		t.Fatal("incorrect remote2 node name")
	}

	if remote1Info.Version != node2.Version() {
		t.Fatal("incorrect remote1 version")
	}
	if remote2Info.Version != node1.Version() {
		t.Fatal("incorrect remote2 version")
	}

	acceptors1, err := node1.Network().Acceptors()
	if err != nil {
		t.Fatal(err)
	}
	flags1 := acceptors1[0].NetworkFlags()
	if flags1 != remote2Info.NetworkFlags {
		t.Fatal("incorrect network flags on remote2")
	}

	acceptors2, err := node2.Network().Acceptors()
	if err != nil {
		t.Fatal(err)
	}
	flags2 := acceptors2[0].NetworkFlags()
	if flags2 != remote1Info.NetworkFlags {
		t.Fatal("incorrect network flags on remote1")
	}

	if node1.Network().MaxMessageSize() != options1.Network.MaxMessageSize {
		t.Fatal("incorrect MaxMessageSize on node1 (ignored option)")
	}
	if node2.Network().MaxMessageSize() != options2.Network.MaxMessageSize {
		t.Fatal("incorrect MaxMessageSize on node2 (ignored option)")
	}

	if node1.Network().MaxMessageSize() != remote2Info.MaxMessageSize {
		t.Fatal("incorrect MaxMessageSize (ignored on node2)")
	}
	if node2.Network().MaxMessageSize() != remote1Info.MaxMessageSize {
		t.Fatal("incorrect MaxMessageSize (ignored on node1)")
	}

	if acceptors2[0].Info().HandshakeVersion != remote1Info.HandshakeVersion {
		t.Fatal("handshake version mismatch")
	}

	if acceptors2[0].Info().ProtoVersion != remote1Info.ProtoVersion {
		t.Fatal("proto version mismatch")
	}
}
