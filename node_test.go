
package ergonode

import (
	"testing"
	"net"
	"fmt"
)


func TestNode(t *testing.T) {
	opts := NodeOptions{
		ListenRangeBegin: 15001,
		ListenRangeEnd: 15001,
		EPMDPort: 14999,
	}

	node := CreateNode("node@localhost","cookies", opts)

	if conn, err := net.Dial("tcp", ":15001"); err != nil {
		fmt.Println("Connect to the node' listening port FAILED")
		t.Fatal(err)
	} else {
		defer conn.Close()
	}

	if conn, err := net.Dial("tcp", ":14999"); err != nil {
		fmt.Println("Connect to the node' listening EPMD port FAILED")
		t.Fatal(err)
	} else {
		defer conn.Close()
	}

	node.Stop()
}
