
package ergonode

import (
	"testing"
	"net"
	"fmt"
)


func TestNode(t *testing.T) {
	opts := NodeOptions{
		ListenRangeBegin: 25001,
		ListenRangeEnd: 25001,
		EPMDPort: 24999,
	}

	node := CreateNode("node@localhost","cookies", opts)

	if conn, err := net.Dial("tcp", ":25001"); err != nil {
		fmt.Println("Connect to the node' listening port FAILED")
		t.Fatal(err)
	} else {
		defer conn.Close()
	}

	if conn, err := net.Dial("tcp", ":24999"); err != nil {
		fmt.Println("Connect to the node' listening EPMD port FAILED")
		t.Fatal(err)
	} else {
		defer conn.Close()
	}

	node.Stop()
}
